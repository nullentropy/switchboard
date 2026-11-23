package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		if _, err := git(dir, []string{"GIT_AUTHOR_NAME=Arthur Dent", "GIT_AUTHOR_EMAIL=arthur@example.example",
			"GIT_COMMITTER_NAME=Arthur Dent", "GIT_COMMITTER_EMAIL=arthur@example.example"}, args...); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	os.MkdirAll(filepath.Join(dir, "magrathea"), 0o755)
	os.WriteFile(filepath.Join(dir, "magrathea", "a.txt"), []byte("one"), 0o644)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644)
	run("add", "-A")
	run("commit", "-q", "-m", "Don't Panic")
	return dir
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func TestRestoringACheckpointPutsTheFilesBackAndLeavesTheBranchAndIndexAlone(t *testing.T) {
	dir := testRepo(t)
	os.WriteFile(filepath.Join(dir, "magrathea", "a.txt"), []byte("staged"), 0o644)
	git(dir, nil, "add", "magrathea/a.txt")
	os.WriteFile(filepath.Join(dir, "magrathea", "a.txt"), []byte("two"), 0o644)
	os.WriteFile(filepath.Join(dir, "towel.txt"), []byte("mine"), 0o644)
	statusBefore, _ := git(dir, nil, "status", "--porcelain")

	sha, err := checkpoint(filepath.Join(dir, "magrathea"), checkpointRef("s1", 0))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "magrathea", "a.txt"), []byte("agent"), 0o644)
	os.WriteFile(filepath.Join(dir, "magrathea", "new.go"), []byte("package magrathea"), 0o644)
	os.Remove(filepath.Join(dir, "towel.txt"))
	os.WriteFile(filepath.Join(dir, "build.log"), []byte("ignored"), 0o644)

	if err := restoreCheckpoint(dir, sha); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "magrathea", "a.txt")); got != "two" {
		t.Errorf("a.txt is %q", got)
	}
	if got := read(t, filepath.Join(dir, "towel.txt")); got != "mine" {
		t.Errorf("towel.txt is %q", got)
	}
	if got := read(t, filepath.Join(dir, "magrathea", "new.go")); got != "<missing>" {
		t.Errorf("new.go is still there: %q", got)
	}
	if got := read(t, filepath.Join(dir, "build.log")); got != "ignored" {
		t.Errorf("an ignored file was touched: %q", got)
	}
	if after, _ := git(dir, nil, "status", "--porcelain"); after != statusBefore {
		t.Errorf("status went from %q to %q", statusBefore, after)
	}
	if b, _ := git(dir, nil, "branch", "--show-current"); b != "main" {
		t.Errorf("branch is %q", b)
	}
	dropCheckpoints(dir, "s1")
	if refs, _ := git(dir, nil, "for-each-ref", "refs/switchboard/"); refs != "" {
		t.Errorf("refs left: %q", refs)
	}
}

func TestAWorktreeKeepsTheSubfolderAndIsRemovedWithItsBranch(t *testing.T) {
	dir := testRepo(t)
	home := t.TempDir()
	cwd, branch, err := addWorktree(filepath.Join(dir, "magrathea"), home, "find-the-question")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cwd) != "magrathea" || read(t, filepath.Join(cwd, "a.txt")) != "one" {
		t.Fatalf("cwd %s", cwd)
	}
	if root := repoRoot(cwd); root != repoRoot(dir) {
		t.Fatalf("repo of the worktree is %q, want %q", root, repoRoot(dir))
	}
	m := Meta{Cwd: cwd, Repo: repoRoot(dir), Worktree: filepath.Dir(cwd), WorkBranch: branch}
	if left, _ := worktreeLeftovers(m); left != "" {
		t.Fatalf("a fresh worktree has leftovers: %s", left)
	}
	os.WriteFile(filepath.Join(cwd, "a.txt"), []byte("changed"), 0o644)
	_, cerr := git(cwd, []string{"GIT_AUTHOR_NAME=Arthur Dent", "GIT_AUTHOR_EMAIL=arthur@example.example",
		"GIT_COMMITTER_NAME=Arthur Dent", "GIT_COMMITTER_EMAIL=arthur@example.example"}, "commit", "-qam", "Mostly harmless")
	if cerr != nil {
		t.Fatal(cerr)
	}
	os.WriteFile(filepath.Join(cwd, "b.txt"), []byte("draft"), 0o644)
	if left, _ := worktreeLeftovers(m); !strings.Contains(left, "1 uncommitted") || !strings.Contains(left, "1 commits") {
		t.Fatalf("leftovers %q", left)
	}
	if err := removeWorktree(m); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.Worktree); !os.IsNotExist(err) {
		t.Errorf("worktree still on disk")
	}
	if b, _ := git(dir, nil, "branch", "--list", branch); b != "" {
		t.Errorf("branch %s still there", branch)
	}
}
