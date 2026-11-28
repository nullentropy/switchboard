package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
)

type fakeRelease struct {
	tag   string
	files map[string][]byte
	hits  atomic.Int32
}

func (f *fakeRelease) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.URL.Path == "/repos/"+Repo+"/releases/latest" {
			fmt.Fprintf(w, `{"tag_name":%q}`, f.tag)
			return
		}
		prefix := "/" + Repo + "/releases/download/" + f.tag + "/"
		if len(r.URL.Path) > len(prefix) && r.URL.Path[:len(prefix)] == prefix {
			if b, ok := f.files[r.URL.Path[len(prefix):]]; ok {
				w.Write(b)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sums(files map[string][]byte) []byte {
	var b bytes.Buffer
	for name, data := range files {
		h := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(h[:]), name)
	}
	return b.Bytes()
}

func linuxTarball(t *testing.T, bin string) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"share/applications/switchboard.desktop": "[Desktop Entry]", "bin/switchboard": bin} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}

func windowsZip(t *testing.T, exe string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.Create("Switchboard.exe")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(exe))
	zw.Close()
	return b.Bytes()
}

func testUpdater(t *testing.T, srv *httptest.Server, version, goos string) Updater {
	return Updater{Version: version, GOOS: goos, GOARCH: "amd64", Dir: filepath.Join(t.TempDir(), "update"),
		API: srv.URL, DL: srv.URL, Client: srv.Client()}
}

func linuxRelease(t *testing.T, tag, bin string) *fakeRelease {
	files := map[string][]byte{"Switchboard-linux-amd64.tar.gz": linuxTarball(t, bin)}
	files["SHA256SUMS"] = sums(files)
	return &fakeRelease{tag: tag, files: files}
}

func TestADevBuildNeverAsksForAnUpdate(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	u := testUpdater(t, rel.serve(t), "dev", "linux")
	tag, err := u.CheckAndStage(context.Background())
	if err != nil || tag != "" {
		t.Fatalf("got %q, %v; want nothing staged", tag, err)
	}
	if n := rel.hits.Load(); n != 0 {
		t.Fatalf("a dev build made %d requests", n)
	}
}

func TestANewerReleaseIsDownloadedCheckedAndStaged(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	u := testUpdater(t, rel.serve(t), "0.1.0", "linux")
	tag, err := u.CheckAndStage(context.Background())
	if err != nil || tag != "v0.2.0" {
		t.Fatalf("got %q, %v; want v0.2.0 staged", tag, err)
	}
	s, err := u.readStaged()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.Payload); string(b) != "new" {
		t.Fatalf("staged payload holds %q, want the release's binary", b)
	}
}

func TestTheSameOrAnOlderReleaseStagesNothing(t *testing.T) {
	for _, tag := range []string{"v0.2.0", "v0.1.9", "garbage"} {
		rel := linuxRelease(t, tag, "new")
		u := testUpdater(t, rel.serve(t), "0.2.0", "linux")
		got, err := u.CheckAndStage(context.Background())
		if err != nil || got != "" {
			t.Errorf("latest %s: got %q, %v; want nothing staged", tag, got, err)
		}
	}
}

func TestADownloadThatFailsItsChecksumIsNotStaged(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	rel.files["Switchboard-linux-amd64.tar.gz"] = linuxTarball(t, "tampered")
	u := testUpdater(t, rel.serve(t), "0.1.0", "linux")
	if _, err := u.CheckAndStage(context.Background()); err == nil {
		t.Fatal("a checksum mismatch was accepted")
	}
	if _, err := u.readStaged(); err == nil {
		t.Fatal("a release that failed its checksum was staged")
	}
}

func TestWindowsStagesTheExeFromItsZip(t *testing.T) {
	files := map[string][]byte{"Switchboard-windows-amd64.zip": windowsZip(t, "new exe")}
	files["SHA256SUMS"] = sums(files)
	rel := &fakeRelease{tag: "v0.2.0", files: files}
	u := testUpdater(t, rel.serve(t), "0.1.0", "windows")
	if _, err := u.CheckAndStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, _ := u.readStaged()
	if b, _ := os.ReadFile(s.Payload); string(b) != "new exe" || filepath.Base(s.Payload) != "Switchboard.exe" {
		t.Fatalf("staged %s holding %q", s.Payload, b)
	}
}

func TestEachPlatformUpdatesFromItsOwnReleaseFile(t *testing.T) {
	cases := []struct{ GOOS, GOARCH, want string }{
		{"darwin", "arm64", "Switchboard-macos-universal.zip"},
		{"darwin", "amd64", "Switchboard-macos-universal.zip"},
		{"linux", "amd64", "Switchboard-linux-amd64.tar.gz"},
		{"linux", "arm64", "Switchboard-linux-arm64.tar.gz"},
		{"windows", "amd64", "Switchboard-windows-amd64.zip"},
	}
	for _, c := range cases {
		if got, err := assetFor(c.GOOS, c.GOARCH); err != nil || got != c.want {
			t.Errorf("%s/%s: got %q, %v; want %s", c.GOOS, c.GOARCH, got, err, c.want)
		}
	}
	for _, p := range [][2]string{{"windows", "arm64"}, {"linux", "386"}, {"freebsd", "amd64"}} {
		if _, err := assetFor(p[0], p[1]); err == nil {
			t.Errorf("%s/%s has no release file but got one", p[0], p[1])
		}
	}
}

func stageLinux(t *testing.T, version, tag string) (Updater, string) {
	rel := linuxRelease(t, tag, "new")
	u := testUpdater(t, rel.serve(t), "0.1.0", "linux")
	if _, err := u.CheckAndStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	u.Version = version
	root := filepath.Join(t.TempDir(), "switchboard")
	os.WriteFile(root, []byte("old"), 0o755)
	return u, root
}

func TestInstallingAStagedReleaseReplacesTheInstallAndClearsTheStage(t *testing.T) {
	u, root := stageLinux(t, "0.1.0", "v0.2.0")
	ok, err := u.InstallStaged(root)
	if err != nil || !ok {
		t.Fatalf("got %v, %v; want installed", ok, err)
	}
	if b, _ := os.ReadFile(root); string(b) != "new" {
		t.Fatalf("install holds %q, want the new release", b)
	}
	if _, err := os.Stat(u.Dir); !os.IsNotExist(err) {
		t.Fatal("the stage was left behind")
	}
}

func TestAStagedReleaseThatIsNotNewerIsDeletedNotInstalled(t *testing.T) {
	u, root := stageLinux(t, "0.2.0", "v0.2.0")
	ok, err := u.InstallStaged(root)
	if err != nil || ok {
		t.Fatalf("got %v, %v; want nothing installed", ok, err)
	}
	if b, _ := os.ReadFile(root); string(b) != "old" {
		t.Fatal("the install was replaced by a release that is not newer")
	}
	if _, err := os.Stat(u.Dir); !os.IsNotExist(err) {
		t.Fatal("the superseded stage was kept")
	}
}

func TestWithUpdatesTurnedOffAStagedReleaseIsNotInstalledAtLaunch(t *testing.T) {
	u, root := stageLinux(t, "0.1.0", "v0.2.0")
	ok, err := u.AtLaunch(true, root)
	if err != nil || ok {
		t.Fatalf("got %v, %v; want nothing installed", ok, err)
	}
	if b, _ := os.ReadFile(root); string(b) != "old" {
		t.Fatal("the install was replaced with updates turned off")
	}
	if ok, err := u.AtLaunch(false, root); err != nil || !ok {
		t.Fatalf("with updates on: got %v, %v; want installed", ok, err)
	}
}

func TestAFailedSwapPutsTheOldInstallBack(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "switchboard")
	os.WriteFile(root, []byte("old"), 0o755)
	if err := swapInstall(root, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("swapping in a missing file succeeded")
	}
	if b, _ := os.ReadFile(root); string(b) != "old" {
		t.Fatalf("install holds %q after a failed swap, want the old one back", b)
	}
}

func fakeApp(t *testing.T, dir, body string) string {
	app := filepath.Join(dir, "Switchboard.app")
	os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755)
	os.WriteFile(filepath.Join(app, "Contents", "MacOS", "switchboard"), []byte(body), 0o755)
	return app
}

func TestMacOSStagesTheAppBundleAndSwapsItWhole(t *testing.T) {
	if _, err := exec.LookPath("ditto"); err != nil {
		t.Skip("needs ditto")
	}
	src := fakeApp(t, t.TempDir(), "new")
	zipPath := filepath.Join(t.TempDir(), "app.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", src, zipPath).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	z, _ := os.ReadFile(zipPath)
	files := map[string][]byte{"Switchboard-macos-universal.zip": z}
	files["SHA256SUMS"] = sums(files)
	rel := &fakeRelease{tag: "v0.2.0", files: files}
	u := testUpdater(t, rel.serve(t), "0.1.0", "darwin")
	if _, err := u.CheckAndStage(context.Background()); err != nil {
		t.Fatal(err)
	}
	root := fakeApp(t, t.TempDir(), "old")
	os.WriteFile(filepath.Join(root, "Contents", "stale"), nil, 0o644)
	if ok, err := u.InstallStaged(root); err != nil || !ok {
		t.Fatalf("got %v, %v; want installed", ok, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "Contents", "MacOS", "switchboard")); string(b) != "new" {
		t.Fatalf("bundle executable holds %q, want the new release", b)
	}
	if _, err := os.Stat(filepath.Join(root, "Contents", "stale")); err == nil {
		t.Fatal("a file from the old bundle survived the swap")
	}
}
