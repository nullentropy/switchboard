package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeDispatcherHub(t *testing.T) (*Hub, string) {
	t.Helper()
	t.Setenv("SWITCHBOARD_FAKE", "claude")
	launches := filepath.Join(t.TempDir(), "launches")
	t.Setenv("SWITCHBOARD_FAKE_ARGS", launches)
	self, _ := os.Executable()
	h := newHub(map[string]string{"claude": self}, filepath.Join(t.TempDir(), "state.json"))
	h.Dispatcher.Meta.Cwd = t.TempDir()
	t.Cleanup(h.StopAll)
	return h, launches
}

func launchLines(t *testing.T, path string) []string {
	t.Helper()
	b, _ := os.ReadFile(path)
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func flagValue(line, flag string) string {
	f := strings.Fields(line)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == flag {
			return f[i+1]
		}
	}
	return ""
}

func dispatcherTurn(t *testing.T, d *Agent, text string) {
	t.Helper()
	go d.Send(text)
	p := waitFor(t, d, "dispatcher asks", func(s Snapshot) bool { return s.State == NeedsYou }).Pending[0]
	d.Answer(p.ReqID, true, false, nil)
	waitFor(t, d, "dispatcher turn end", func(s Snapshot) bool { return s.State == Ready })
}

func TestTheDispatcherModelSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(nil, path)
	h.SetDispatcherModel("heart-of-gold")
	if got := newHub(nil, path).Dispatcher.Snapshot().Meta.Model; got != "heart-of-gold" {
		t.Fatalf("after a restart the dispatcher model is %q, want heart-of-gold", got)
	}
	h.SetDispatcherModel("")
	if got := newHub(nil, path).Dispatcher.Snapshot().Meta.Model; got != "" {
		t.Fatalf("after choosing Default and restarting the dispatcher model is %q, want the CLI default", got)
	}
}

func TestSettingsOffersDefaultAndTheClaudeModelsForTheDispatcher(t *testing.T) {
	h := newHub(nil, "")
	h.claudeModels = []string{"deep-thought", "heart-of-gold"}
	h.settings.Provider = "codex"

	names, sel := h.dispatcherModelChoices()
	if want := []string{"Default", "deep-thought", "heart-of-gold"}; !reflect.DeepEqual(names, want) || sel != 0 {
		t.Fatalf("choices %v selected %d, want %v selected 0", names, sel, want)
	}

	h.SetDispatcherModel("heart-of-gold")
	if _, sel := h.dispatcherModelChoices(); sel != 2 {
		t.Fatalf("selected %d, want heart-of-gold at 2", sel)
	}

	h.SetDispatcherModel("infinite-improbability")
	names, sel = h.dispatcherModelChoices()
	if names[sel] != "infinite-improbability" {
		t.Fatalf("a saved model the CLI no longer lists is not shown: %v selected %d", names, sel)
	}
}

func TestTheDispatcherUsesANewModelFromItsNextTurnAndKeepsItsConversation(t *testing.T) {
	h, launches := fakeDispatcherHub(t)
	d := h.Dispatcher
	dispatcherTurn(t, d, "what is the answer")
	if l := launchLines(t, launches); len(l) != 1 || flagValue(l[0], "--model") != "" {
		t.Fatalf("first launch %v, want one launch on the CLI default model", l)
	}

	h.SetDispatcherModel("deep-thought")
	dispatcherTurn(t, d, "what is the question")

	l := launchLines(t, launches)
	if len(l) != 2 {
		t.Fatalf("launches %v, want the dispatcher relaunched once", l)
	}
	if got := flagValue(l[1], "--model"); got != "deep-thought" {
		t.Fatalf("relaunch model %q, want deep-thought: %s", got, l[1])
	}
	if got := flagValue(l[1], "--resume"); got != d.Snapshot().Meta.ID {
		t.Fatalf("relaunch resumes %q, want the dispatcher's conversation %s", got, d.Snapshot().Meta.ID)
	}
}

func TestChangingTheDispatcherModelMidTurnWaitsForTheTurnToEnd(t *testing.T) {
	h, launches := fakeDispatcherHub(t)
	d := h.Dispatcher
	go d.Send("build the hyperspace bypass")
	p := waitFor(t, d, "dispatcher asks", func(s Snapshot) bool { return s.State == NeedsYou }).Pending[0]

	h.SetDispatcherModel("heart-of-gold")
	if !d.running() || d.Snapshot().State != NeedsYou {
		t.Fatalf("the dispatcher was stopped mid-turn: state %s", d.Snapshot().State)
	}
	d.Answer(p.ReqID, true, false, nil)
	s := waitFor(t, d, "dispatcher turn end", func(s Snapshot) bool { return s.State == Ready })
	if last := s.Entries[len(s.Entries)-1]; last.Text != "decision allow" {
		t.Fatalf("the turn did not finish on its first process: %+v", s.Entries)
	}
	if l := launchLines(t, launches); len(l) != 1 {
		t.Fatalf("launches %v, want no relaunch during the turn", l)
	}

	dispatcherTurn(t, d, "and now the Vogons")
	l := launchLines(t, launches)
	if len(l) != 2 || flagValue(l[1], "--model") != "heart-of-gold" || flagValue(l[1], "--resume") != d.Snapshot().Meta.ID {
		t.Fatalf("launches %v, want a resumed relaunch on heart-of-gold", l)
	}
}
