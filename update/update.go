package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const Repo = "nullentropy/switchboard"

type Updater struct {
	Version, GOOS, GOARCH string
	Dir                   string
	API, DL               string
	Client                *http.Client
}

func New(version, dir string) Updater {
	return Updater{Version: version, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Dir: dir,
		API: "https://api.github.com", DL: "https://github.com",
		Client: &http.Client{Timeout: 10 * time.Minute}}
}

type staged struct {
	Tag     string `json:"tag"`
	Payload string `json:"payload"`
}

func assetFor(goos, goarch string) (string, error) {
	switch {
	case goos == "darwin":
		return "Switchboard-macos-universal.zip", nil
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return "Switchboard-linux-" + goarch + ".tar.gz", nil
	case goos == "windows" && goarch == "amd64":
		return "Switchboard-windows-amd64.zip", nil
	}
	return "", fmt.Errorf("no release file for %s/%s", goos, goarch)
}

func ParseTag(s string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func NewerTag(cur, next string) bool {
	a, oka := ParseTag(cur)
	b, okb := ParseTag(next)
	if !oka || !okb {
		return false
	}
	for i := range 3 {
		if a[i] != b[i] {
			return b[i] > a[i]
		}
	}
	return false
}

func (u Updater) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "switchboard-updater")
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

func (u Updater) LatestTag(ctx context.Context) (string, error) {
	resp, err := u.get(ctx, u.API+"/repos/"+Repo+"/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("read the latest release: %w", err)
	}
	return r.Tag, nil
}

func (u Updater) download(ctx context.Context, tag, name string) error {
	resp, err := u.get(ctx, u.DL+"/"+Repo+"/releases/download/"+tag+"/"+name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(filepath.Join(u.Dir, name))
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (u Updater) CheckAndStage(ctx context.Context) (string, error) {
	if _, ok := ParseTag(u.Version); !ok {
		return "", nil
	}
	asset, err := assetFor(u.GOOS, u.GOARCH)
	if err != nil {
		return "", err
	}
	tag, err := u.LatestTag(ctx)
	if err != nil {
		return "", err
	}
	if !NewerTag(u.Version, tag) {
		return "", nil
	}
	if s, err := u.readStaged(); err == nil && s.Tag == tag {
		if _, err := os.Stat(s.Payload); err == nil {
			return tag, nil
		}
	}
	if err := os.RemoveAll(u.Dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return "", err
	}
	for _, name := range []string{asset, "SHA256SUMS"} {
		if err := u.download(ctx, tag, name); err != nil {
			os.RemoveAll(u.Dir)
			return "", err
		}
	}
	payload, err := u.unpack(ctx, asset)
	if err != nil {
		os.RemoveAll(u.Dir)
		return "", err
	}
	return tag, u.writeStaged(staged{Tag: tag, Payload: payload})
}

func (u Updater) unpack(ctx context.Context, asset string) (string, error) {
	if err := verifySum(u.Dir, asset); err != nil {
		return "", err
	}
	src := filepath.Join(u.Dir, asset)
	switch u.GOOS {
	case "darwin":
		// ditto keeps the bundle's symlinks and extended attributes, which a
		// Go zip reader drops.
		if err := exec.CommandContext(ctx, "ditto", "-x", "-k", src, u.Dir).Run(); err != nil {
			return "", fmt.Errorf("unpack %s: %w", asset, err)
		}
		return filepath.Join(u.Dir, "Switchboard.app"), nil
	case "windows":
		return filepath.Join(u.Dir, "Switchboard.exe"), unzipOne(src, "Switchboard.exe", filepath.Join(u.Dir, "Switchboard.exe"))
	default:
		return filepath.Join(u.Dir, "switchboard"), untarOne(src, "bin/switchboard", filepath.Join(u.Dir, "switchboard"))
	}
}

func verifySum(dir, asset string) error {
	sums, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			want = f[0]
		}
	}
	if want == "" {
		return fmt.Errorf("the release has no checksum for %s", asset)
	}
	f, err := os.Open(filepath.Join(dir, asset))
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fmt.Errorf("%s does not match its checksum", asset)
	}
	return nil
}

func writeExec(dst string, r io.Reader) error {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func untarOne(src, name, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s has no %s", filepath.Base(src), name)
		}
		if err != nil {
			return err
		}
		if strings.TrimPrefix(h.Name, "./") == name && h.Typeflag == tar.TypeReg {
			return writeExec(dst, tr)
		}
	}
}

func unzipOne(src, name, dst string) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			r, err := f.Open()
			if err != nil {
				return err
			}
			defer r.Close()
			return writeExec(dst, r)
		}
	}
	return fmt.Errorf("%s has no %s", filepath.Base(src), name)
}

func (u Updater) stagedPath() string { return filepath.Join(u.Dir, "staged.json") }

func (u Updater) readStaged() (staged, error) {
	var s staged
	b, err := os.ReadFile(u.stagedPath())
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

func (u Updater) writeStaged(s staged) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(u.stagedPath(), b, 0o600)
}

func (u Updater) ReadyTag() string {
	s, err := u.readStaged()
	if err != nil || !NewerTag(u.Version, s.Tag) {
		return ""
	}
	return s.Tag
}

func InstallRoot() (root, exe string, err error) {
	if exe, err = os.Executable(); err != nil {
		return "", "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", "", err
	}
	if runtime.GOOS == "darwin" {
		if root := filepath.Dir(filepath.Dir(filepath.Dir(exe))); strings.HasSuffix(root, ".app") {
			return root, exe, nil
		}
	}
	return exe, exe, nil
}

func (u Updater) InstallStaged(root string) (bool, error) {
	s, err := u.readStaged()
	if err != nil {
		return false, nil
	}
	if !NewerTag(u.Version, s.Tag) {
		return false, os.RemoveAll(u.Dir)
	}
	if strings.HasSuffix(s.Payload, ".app") != strings.HasSuffix(root, ".app") {
		return false, fmt.Errorf("%s is not an installed release; reinstall with install.sh", root)
	}
	if err := swapInstall(root, s.Payload); err != nil {
		return false, err
	}
	os.RemoveAll(u.Dir)
	return true, nil
}

func (u Updater) AtLaunch(off bool, root string) (bool, error) {
	os.RemoveAll(root + ".old")
	if off {
		return false, nil
	}
	return u.InstallStaged(root)
}

func swapInstall(root, src string) error {
	old := root + ".old"
	os.RemoveAll(old)
	if err := os.Rename(root, old); err != nil {
		return fmt.Errorf("cannot replace %s: %w", root, err)
	}
	if err := moveInto(src, root); err != nil {
		os.RemoveAll(root)
		if back := os.Rename(old, root); back != nil {
			return fmt.Errorf("update failed (%w) and %s could not be put back: %w", err, root, back)
		}
		return err
	}
	// On Windows the running exe cannot be deleted; AtLaunch removes it next time.
	os.RemoveAll(old)
	return nil
}

func moveInto(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("ditto", src, dst).Run()
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeExec(dst, in)
}
