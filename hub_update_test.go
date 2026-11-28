package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"switchboard/update"
)

type fakeRelease struct {
	tag   string
	files map[string][]byte
	hits  atomic.Int32
}

func (f *fakeRelease) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.URL.Path == "/repos/"+update.Repo+"/releases/latest" {
			fmt.Fprintf(w, `{"tag_name":%q}`, f.tag)
			return
		}
		prefix := "/" + update.Repo + "/releases/download/" + f.tag + "/"
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
func testUpdater(t *testing.T, srv *httptest.Server, version, goos string) update.Updater {
	return update.Updater{Version: version, GOOS: goos, GOARCH: "amd64", Dir: filepath.Join(t.TempDir(), "update"),
		API: srv.URL, DL: srv.URL, Client: srv.Client()}
}

func linuxRelease(t *testing.T, tag, bin string) *fakeRelease {
	files := map[string][]byte{"Switchboard-linux-amd64.tar.gz": linuxTarball(t, bin)}
	files["SHA256SUMS"] = sums(files)
	return &fakeRelease{tag: tag, files: files}
}

func TestRestartingToUpdateNamesTheSessionsMidTurn(t *testing.T) {
	d := &Agent{Meta: Meta{Title: "Dispatcher"}, State: Working}
	h := &Hub{Dispatcher: d, agents: []*Agent{
		{Meta: Meta{Title: "Ford"}, State: Working},
		{Meta: Meta{Title: "Zaphod"}, State: Ready},
		{Meta: Meta{Title: "Trillian"}, State: Starting},
		{Meta: Meta{Title: "Arthur"}, State: NeedsYou},
	}}
	got := h.midTurn()
	want := []string{"Dispatcher", "Ford", "Trillian"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCheckingForUpdatesWithAutomaticUpdatesOnDownloadsTheRelease(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	u := testUpdater(t, rel.serve(t), "0.1.0", "linux")
	h := &Hub{updater: &u}
	tag, ready, err := h.CheckForUpdates(context.Background())
	if err != nil || tag != "v0.2.0" || !ready {
		t.Fatalf("got %q, ready %v, %v; want v0.2.0 ready", tag, ready, err)
	}
	if got := h.UpdateReady(); got != "v0.2.0" {
		t.Fatalf("UpdateReady = %q; want v0.2.0", got)
	}
}

func TestCheckingForUpdatesWithAutomaticUpdatesOffOnlyReportsTheRelease(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	u := testUpdater(t, rel.serve(t), "0.1.0", "linux")
	h := &Hub{updater: &u, settings: Settings{UpdatesOff: true}}
	tag, ready, err := h.CheckForUpdates(context.Background())
	if err != nil || tag != "v0.2.0" || ready {
		t.Fatalf("got %q, ready %v, %v; want v0.2.0 not ready", tag, ready, err)
	}
	if n := rel.hits.Load(); n != 1 {
		t.Fatalf("made %d requests; want only the release lookup", n)
	}
	tag, err = h.DownloadUpdate(context.Background())
	if err != nil || tag != "v0.2.0" {
		t.Fatalf("download got %q, %v; want v0.2.0", tag, err)
	}
	if u.ReadyTag() != "v0.2.0" {
		t.Fatal("the downloaded release is not staged")
	}
}

func TestCheckingForUpdatesOnTheLatestReleaseIsUpToDate(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	for _, off := range []bool{false, true} {
		u := testUpdater(t, rel.serve(t), "0.2.0", "linux")
		h := &Hub{updater: &u, settings: Settings{UpdatesOff: off}}
		tag, ready, err := h.CheckForUpdates(context.Background())
		if err != nil || tag != "" || ready {
			t.Errorf("updates off %v: got %q, ready %v, %v; want up to date", off, tag, ready, err)
		}
	}
}

func TestABuildThatCannotUpdateSaysSo(t *testing.T) {
	h := &Hub{}
	if _, _, err := h.CheckForUpdates(context.Background()); err != errNoSelfUpdate {
		t.Fatalf("check got %v; want %v", err, errNoSelfUpdate)
	}
	if _, err := h.DownloadUpdate(context.Background()); err != errNoSelfUpdate {
		t.Fatalf("download got %v; want %v", err, errNoSelfUpdate)
	}
}

func TestTheUpdateToolOnTheLatestReleaseSaysUpToDate(t *testing.T) {
	rel := linuxRelease(t, "v0.2.0", "new")
	u := testUpdater(t, rel.serve(t), "0.2.0", "linux")
	h := &Hub{updater: &u}
	got, err := (&mcpServer{hub: h}).call("check_for_updates", nil)
	if err != nil || got != "Switchboard 0.2.0 is up to date." {
		t.Fatalf("got %q, %v; want up to date", got, err)
	}
	if h.UpdateReady() != "" || u.ReadyTag() != "" {
		t.Fatal("an up to date check staged a release")
	}
}

func TestTheUpdateToolDownloadsANewerReleaseAndDoesNotRestart(t *testing.T) {
	rel := linuxRelease(t, "v0.3.0", "new")
	u := testUpdater(t, rel.serve(t), "0.2.0", "linux")
	restarted := false
	h := &Hub{updater: &u, settings: Settings{UpdatesOff: true}, restart: func() error { restarted = true; return nil }}
	got, err := (&mcpServer{hub: h}).call("check_for_updates", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"0.2.0", "0.3.0", "ready", "restart Switchboard themselves"} {
		if !strings.Contains(got, want) {
			t.Errorf("result %q does not mention %q", got, want)
		}
	}
	if u.ReadyTag() != "v0.3.0" {
		t.Fatal("the newer release is not staged")
	}
	if h.UpdateReady() != "v0.3.0" {
		t.Fatalf("UpdateReady = %q; want v0.3.0 shown with automatic updates off", h.UpdateReady())
	}
	if restarted {
		t.Fatal("the tool restarted Switchboard")
	}
}

func TestTheUpdateToolOnADevBuildSaysSo(t *testing.T) {
	_, err := (&mcpServer{hub: &Hub{}}).call("check_for_updates", nil)
	if err == nil || !strings.Contains(err.Error(), "development build") {
		t.Fatalf("got %v; want a development build message", err)
	}
}
