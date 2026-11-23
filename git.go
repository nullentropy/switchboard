package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func git(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

func repoRoot(dir string) string {
	common, err := git(dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return ""
	}
	if filepath.Base(common) == ".git" {
		return filepath.Dir(common)
	}
	top, _ := git(dir, nil, "rev-parse", "--show-toplevel")
	return top
}

var slugBad = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugBad.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "-")
	}
	if s == "" {
		s = "session"
	}
	return s
}

func worktreeHome() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "switchboard", "worktrees")
}

func addWorktree(dir, home, name string) (path, branch string, err error) {
	top, err := git(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	rel, _ := git(dir, nil, "rev-parse", "--show-prefix")
	root := repoRoot(dir)
	path = filepath.Join(home, filepath.Base(root)+"-"+name)
	branch = "switchboard/" + name
	if _, err := git(top, nil, "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return "", "", err
	}
	if rel != "" {
		return filepath.Join(path, rel), branch, nil
	}
	return path, branch, nil
}

func worktreeTop(m Meta) string {
	if m.Worktree == "" {
		return ""
	}
	top, err := git(m.Cwd, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return m.Worktree
	}
	return top
}

func worktreeLeftovers(m Meta) (string, error) {
	top := worktreeTop(m)
	if top == "" {
		return "", nil
	}
	if _, err := os.Stat(top); err != nil {
		return "", nil
	}
	var parts []string
	if st, err := git(top, nil, "status", "--porcelain"); err == nil && st != "" {
		parts = append(parts, fmt.Sprintf("%d uncommitted changes", len(strings.Split(st, "\n"))))
	}
	base, err := git(m.Repo, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if n, err := git(top, nil, "rev-list", "--count", base+"..HEAD"); err == nil && n != "0" {
		parts = append(parts, n+" commits not merged into "+filepath.Base(m.Repo))
	}
	return strings.Join(parts, " and "), nil
}

func removeWorktree(m Meta) error {
	top := worktreeTop(m)
	if top == "" {
		return nil
	}
	if _, err := git(m.Repo, nil, "worktree", "remove", "--force", top); err != nil {
		return err
	}
	if m.WorkBranch != "" {
		_, _ = git(m.Repo, nil, "branch", "-D", m.WorkBranch)
	}
	return nil
}

func checkpointRef(id string, n int) string {
	return fmt.Sprintf("refs/switchboard/checkpoints/%s/%d", id, n)
}

func snapshotTree(dir string) (string, error) {
	f, err := os.CreateTemp("", "switchboard-index-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	seeded := false
	if idx, err := git(dir, nil, "rev-parse", "--path-format=absolute", "--git-path", "index"); err == nil {
		if src, err := os.Open(idx); err == nil {
			_, err = io.Copy(f, src)
			src.Close()
			seeded = err == nil
		}
	}
	f.Close()
	if !seeded {
		os.Remove(f.Name())
	}
	env := []string{"GIT_INDEX_FILE=" + f.Name()}
	if _, err := git(dir, env, "add", "-A", "."); err != nil {
		return "", err
	}
	return git(dir, env, "write-tree")
}

func checkpoint(dir, ref string) (string, error) {
	top, err := git(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	tree, err := snapshotTree(top)
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", tree, "-m", "switchboard checkpoint"}
	if head, err := git(top, nil, "rev-parse", "--verify", "-q", "HEAD"); err == nil && head != "" {
		args = append(args, "-p", head)
	}
	sha, err := git(top, []string{"GIT_AUTHOR_NAME=switchboard", "GIT_AUTHOR_EMAIL=switchboard@localhost",
		"GIT_COMMITTER_NAME=switchboard", "GIT_COMMITTER_EMAIL=switchboard@localhost"}, args...)
	if err != nil {
		return "", err
	}
	if _, err := git(top, nil, "update-ref", ref, sha); err != nil {
		return "", err
	}
	return sha, nil
}

func restoreCheckpoint(dir, sha string) error {
	top, err := git(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	now, err := snapshotTree(top)
	if err != nil {
		return err
	}
	added, err := git(top, nil, "diff", "--name-only", "--no-renames", "--diff-filter=A", sha+"^{tree}", now)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "switchboard-index-*")
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(f.Name())
	defer os.Remove(f.Name())
	env := []string{"GIT_INDEX_FILE=" + f.Name()}
	if _, err := git(top, env, "read-tree", sha+"^{tree}"); err != nil {
		return err
	}
	if _, err := git(top, env, "checkout-index", "-a", "-f"); err != nil {
		return err
	}
	for _, p := range strings.Split(added, "\n") {
		if p != "" {
			os.Remove(filepath.Join(top, p))
		}
	}
	return nil
}

func dropCheckpoints(dir, id string) {
	refs, err := git(dir, nil, "for-each-ref", "--format=%(refname)", "refs/switchboard/checkpoints/"+id+"/")
	if err != nil {
		return
	}
	for _, r := range strings.Split(refs, "\n") {
		if r != "" {
			_, _ = git(dir, nil, "update-ref", "-d", r)
		}
	}
}

func projectFiles(dir string) []string {
	out, err := git(dir, nil, "ls-files", "-co", "--exclude-standard")
	if err == nil {
		if out == "" {
			return nil
		}
		return strings.Split(out, "\n")
	}
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || len(files) >= 5000 {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() && p != dir && (strings.HasPrefix(name, ".") || name == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, rel)
		}
		return nil
	})
	return files
}
