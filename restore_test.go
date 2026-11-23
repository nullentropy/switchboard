package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setState(a *Agent, st State) {
	a.mu.Lock()
	a.State = st
	a.mu.Unlock()
	a.changed()
}

func byTitle(t *testing.T, h *Hub, title string) *Agent {
	t.Helper()
	for _, a := range h.Agents() {
		if a.Snapshot().Meta.Title == title {
			return a
		}
	}
	t.Fatalf("no session %q after restart", title)
	return nil
}

func TestARestartShowsEachSessionsLastStateAndOnlyAnExplicitStopAsStopped(t *testing.T) {
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	self, _ := os.Executable()
	bins := map[string]string{"claude": self}
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(bins, path)
	m := &mcpServer{hub: h}

	arthur, _ := h.New(Meta{Title: "Arthur Dent", Cwd: t.TempDir(), Mode: "default"}, false)
	go arthur.Send("make a cup of tea")
	p := waitFor(t, arthur, "waiting on the user", func(s Snapshot) bool { return s.State == NeedsYou }).Pending[0]
	arthur.Answer(p.ReqID, true, false, nil)
	waitFor(t, arthur, "turn end", func(s Snapshot) bool { return s.State == Ready })

	trillian, _ := h.New(Meta{Title: "Trillian", Cwd: t.TempDir(), Mode: "default"}, false)
	go trillian.Send("work out the improbability")
	waitFor(t, trillian, "waiting on the user", func(s Snapshot) bool { return s.State == NeedsYou })

	ford, _ := h.New(Meta{Title: "Ford Prefect", Cwd: t.TempDir(), Mode: "default"}, false)
	setState(ford, Working)
	marvin, _ := h.New(Meta{Title: "Marvin", Cwd: t.TempDir(), Mode: "default"}, false)
	setState(marvin, Failed)
	zaphod, _ := h.New(Meta{Title: "Zaphod Beeblebrox", Cwd: t.TempDir(), Mode: "default"}, false)
	setState(zaphod, Ready)
	if _, err := m.call("stop_session", map[string]any{"session": "Zaphod Beeblebrox"}); err != nil {
		t.Fatal(err)
	}
	if s := zaphod.Snapshot().State; s != Stopped {
		t.Fatalf("a session the dispatcher stopped shows %s, want Stopped", s)
	}

	h.StopAll()
	waitFor(t, arthur, "process gone", func(Snapshot) bool { return !arthur.running() })
	waitFor(t, trillian, "process gone", func(Snapshot) bool { return !trillian.running() })
	h.save()

	h2 := newHub(bins, path)
	for title, want := range map[string]State{
		"Arthur Dent": Ready, "Trillian": Ready, "Ford Prefect": Interrupted,
		"Marvin": Failed, "Zaphod Beeblebrox": Stopped,
	} {
		if got := byTitle(t, h2, title).Snapshot().State; got != want {
			t.Errorf("%s after restart shows %s, want %s", title, got, want)
		}
	}

	out, err := (&mcpServer{hub: h2}).call("list_sessions", nil)
	if err != nil || !strings.Contains(out, `"state": "Interrupted"`) {
		t.Fatalf("list_sessions after restart: %v %s", err, out)
	}
}

func TestASessionSavedByAnOlderBuildComesBackReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	old := `{"settings": {"theme": "Gruvbox"}, "sessions": [{"id": "42424242-0000-4000-8000-000000000042", "title": "Slartibartfast", "cwd": "` + t.TempDir() + `", "mode": "default", "launched": false}]}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHub(map[string]string{}, path)
	if s := byTitle(t, h, "Slartibartfast").Snapshot().State; s != Ready {
		t.Fatalf("an older build's session shows %s, want Ready", s)
	}
	if got := h.takeInterrupted(); len(got) != 0 {
		t.Fatalf("an older build's session counted as interrupted: %d", len(got))
	}
}

func TestResumingAfterARestartContinuesEachInterruptedSessionAndAsksOnce(t *testing.T) {
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	self, _ := os.Executable()
	bins := map[string]string{"claude": self}
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(bins, path)
	for _, title := range []string{"Ford Prefect", "Deep Thought"} {
		a, _ := h.New(Meta{Title: title, Cwd: t.TempDir(), Mode: "default"}, false)
		setState(a, Working)
	}
	calm, _ := h.New(Meta{Title: "Arthur Dent", Cwd: t.TempDir(), Mode: "default"}, false)
	setState(calm, Ready)
	h.StopAll()
	h.save()

	h2 := newHub(bins, path)
	defer h2.StopAll()
	list := h2.takeInterrupted()
	var names []string
	for _, a := range list {
		names = append(names, a.Snapshot().Meta.Title)
	}
	if strings.Join(names, ",") != "Ford Prefect,Deep Thought" {
		t.Fatalf("interrupted sessions %v, want Ford Prefect and Deep Thought", names)
	}
	if again := h2.takeInterrupted(); len(again) != 0 {
		t.Fatalf("asked a second time about %d sessions", len(again))
	}

	h2.Resume(list)
	for _, a := range list {
		s := waitFor(t, a, "resumed", func(s Snapshot) bool { return s.State == NeedsYou })
		var sent string
		for _, e := range s.Entries {
			if e.Kind == EntryUser {
				sent = e.Text
			}
		}
		if !strings.Contains(sent, "Switchboard restarted") || !strings.Contains(sent, "carry on where you left off") {
			t.Fatalf("%s was sent %q", s.Meta.Title, sent)
		}
	}
	if s := byTitle(t, h2, "Arthur Dent").Snapshot(); s.State != Ready || len(s.Entries) != 0 {
		t.Fatalf("a session that was not interrupted was resumed: %s %+v", s.State, s.Entries)
	}
}

func TestTheDispatcherCanMessageAnInterruptedSession(t *testing.T) {
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	self, _ := os.Executable()
	h := newHub(map[string]string{"claude": self}, "")
	defer h.StopAll()
	a := wallSession(t, h, Meta{Title: "Eddie", Cwd: t.TempDir(), Mode: "default"})
	a.State = Interrupted
	if _, err := (&mcpServer{hub: h}).call("send_message", map[string]any{"session": "Eddie", "text": "carry on, shipboard computer"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, a, "resumed", func(s Snapshot) bool { return s.State == NeedsYou })
}
