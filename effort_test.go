package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeAgentHub(t *testing.T, provider string) (*Hub, string) {
	t.Helper()
	t.Setenv("SWITCHBOARD_FAKE", provider)
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("SWITCHBOARD_FAKE_ARGS", record)
	self, _ := os.Executable()
	return newHub(map[string]string{"claude": self, "codex": self, "gemini": self}, ""), record
}

func reported(effort string) func(Snapshot) bool {
	return func(s Snapshot) bool { return s.ReportedEffort == effort && s.effort() == effort }
}

func TestClaudeStartsWithTheEffortFlagShowsWhatItReportsAndChangesItLive(t *testing.T) {
	h, record := fakeAgentHub(t, "claude")
	a, _ := h.New(Meta{Title: "Deep Thought", Cwd: t.TempDir(), Mode: "default", Effort: "xhigh"}, false)
	defer a.Stop()
	go a.Send("compute the answer to life, the universe and everything")

	waitFor(t, a, "claude reporting xhigh", reported("xhigh"))
	if args := read(t, record); !strings.Contains(args, "--effort xhigh") {
		t.Fatalf("claude was started with %q, want --effort xhigh", args)
	}

	if err := a.SetEffort("max"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, a, "claude reporting max after the change", reported("max"))
	if strings.Count(read(t, record), "\n") != 1 {
		t.Fatal("changing the effort restarted claude")
	}
}

func TestClaudeShowsItsOwnDefaultEffortWhenSwitchboardAskedForNone(t *testing.T) {
	h, record := fakeAgentHub(t, "claude")
	a, _ := h.New(Meta{Title: "Marvin", Cwd: t.TempDir(), Mode: "default"}, false)
	defer a.Stop()
	if s := a.Snapshot(); s.effort() != "" {
		t.Fatalf("effort %q before claude said anything, want none", s.effort())
	}
	go a.Send("cheer up")

	waitFor(t, a, "claude reporting its default", reported("medium"))
	if strings.Contains(read(t, record), "--effort") {
		t.Fatal("claude got an --effort flag nobody asked for")
	}
}

func TestCodexGetsTheEffortAsConfigThenPerTurn(t *testing.T) {
	h, record := fakeAgentHub(t, "codex")
	a, _ := h.New(Meta{Provider: "codex", Title: "Heart of Gold", Cwd: t.TempDir(), Mode: "default", Effort: "high"}, false)
	defer a.Stop()
	go a.Send("engage the Infinite Improbability Drive")

	s := waitFor(t, a, "waiting on the user", func(s Snapshot) bool { return s.State == NeedsYou })
	if s.ReportedEffort != "high" {
		t.Fatalf("reported effort %q, want high", s.ReportedEffort)
	}
	a.Answer(s.Pending[0].ReqID, true, false, nil)
	waitFor(t, a, "turn end", func(s Snapshot) bool { return s.State == Ready })

	if err := a.SetEffort("minimal"); err != nil {
		t.Fatal(err)
	}
	go a.Send("now make the tea")
	waitFor(t, a, "codex taking minimal for the next turn", func(s Snapshot) bool { return s.State == NeedsYou && s.ReportedEffort == "minimal" })

	want := "thread/start model_reasoning_effort=high\nturn/start effort=high\nturn/start effort=minimal\n"
	if got := read(t, record); got != want {
		t.Fatalf("codex received\n%s\nwant\n%s", got, want)
	}
}

func TestStartSessionTakesOnlyTheEffortsEachAgentSupports(t *testing.T) {
	h, _ := fakeAgentHub(t, "claude")
	m := &mcpServer{hub: h}
	codexModelsMu.Lock()
	saved := codexModels
	codexModels = []codexModel{
		{Model: "heart-of-gold", Efforts: []string{"low", "high"}, Default: true},
		{Model: "bistromath", Efforts: []string{"minimal", "medium"}},
	}
	codexModelsMu.Unlock()
	defer func() { codexModelsMu.Lock(); codexModels = saved; codexModelsMu.Unlock() }()

	start := func(args map[string]any) error {
		args["folder"], args["prompt"] = t.TempDir(), "Don't panic"
		_, err := m.call("start_session", args)
		return err
	}
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"claude refuses ultra", map[string]any{"effort": "ultra"}, `Claude does not accept effort "ultra"; use one of low, medium, high, xhigh, max`},
		{"codex default model refuses medium", map[string]any{"agent": "codex", "effort": "medium"}, `Codex does not accept effort "medium"; use one of low, high`},
		{"codex named model refuses high", map[string]any{"agent": "codex", "model": "bistromath", "effort": "high"}, `Codex model bistromath does not accept effort "high"; use one of minimal, medium`},
		{"codex unlisted model refuses towel", map[string]any{"agent": "codex", "model": "vogon", "effort": "towel"}, "use one of none, minimal, low, medium, high, xhigh, max, ultra"},
		{"gemini refuses any effort", map[string]any{"agent": "gemini", "effort": "high"}, "Gemini has no reasoning effort setting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := start(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
			if len(h.Agents()) != 0 {
				t.Fatal("a refused effort still started a session")
			}
		})
	}

	if err := start(map[string]any{"title": "Zaphod", "effort": "HIGH"}); err != nil {
		t.Fatal(err)
	}
	a, err := m.find("Zaphod")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	if e := a.Snapshot().Meta.Effort; e != "high" {
		t.Fatalf("saved effort %q, want high", e)
	}
	if err := a.SetEffort("ultra"); err == nil || !strings.Contains(err.Error(), "low, medium, high, xhigh, max") {
		t.Fatalf("changing a claude session to ultra: %v", err)
	}
}
