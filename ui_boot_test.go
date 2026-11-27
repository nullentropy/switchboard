package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const testUI = `{"name":"Compact","slots":{"header":{"type":"label","text":"hi"}}}`

func uiHub(t *testing.T) (string, *Hub) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(map[string]string{}, path)
	h.UpdateSettings(func(s *Settings) {
		s.Interfaces = map[string]json.RawMessage{"Compact": json.RawMessage(testUI)}
		s.Interface = "Compact"
	})
	return path, h
}

func TestInterfacesSaveAndLoad(t *testing.T) {
	path, _ := uiHub(t)
	h := newHub(map[string]string{}, path)
	s := h.Settings()
	if s.Interface != "Compact" || len(s.Interfaces) != 1 {
		t.Fatalf("loaded interface %q, %d saved", s.Interface, len(s.Interfaces))
	}
	name, raw := h.ActiveUI()
	var got, want any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(testUI), &want)
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if name != "Compact" || string(gb) != string(wb) {
		t.Fatalf("active %q %s, want Compact %s", name, raw, testUI)
	}
}

func TestDefaultIsEmptyInterface(t *testing.T) {
	path, h := uiHub(t)
	h.UpdateSettings(func(s *Settings) { s.Interface = "" })
	h = newHub(map[string]string{}, path)
	if name, raw := h.ActiveUI(); name != "" || raw != nil {
		t.Fatalf("Default should be on, got %q", name)
	}
	if _, ok := h.Settings().Interfaces["Compact"]; !ok {
		t.Fatal("switching to Default dropped the saved interface")
	}
}

func TestSkipIgnoresSavedInterfaceForTheRun(t *testing.T) {
	path, h := uiHub(t)
	h.bootUI(false)
	h = newHub(map[string]string{}, path)
	h.bootUI(true)
	if name, _ := h.ActiveUI(); name != "" {
		t.Fatalf("skipped run shows %q", name)
	}
	h = newHub(map[string]string{}, path)
	if h.Settings().Interface != "Compact" {
		t.Fatalf("skip changed the saved choice to %q", h.Settings().Interface)
	}
	if h.uiStarts != 0 {
		t.Fatalf("skipped run left the crash count at %d", h.uiStarts)
	}
	h.bootUI(true)
	h.UseUI(func(s *Settings) {
		s.Interfaces["Other"] = json.RawMessage(testUI)
		s.Interface = "Other"
	})
	if name, _ := h.ActiveUI(); name != "Other" {
		t.Fatalf("choosing an interface during a skipped run shows %q", name)
	}
}

func bootAgain(path string, skip bool) *Hub {
	h := newHub(map[string]string{}, path)
	h.bootUI(skip)
	return h
}

func TestTwoShortStartupsSwitchToDefault(t *testing.T) {
	path, _ := uiHub(t)
	bootAgain(path, false)
	h := bootAgain(path, false)
	if name, _ := h.ActiveUI(); name != "Compact" || len(h.reports) != 0 {
		t.Fatalf("second startup: active %q, reports %v", name, h.reports)
	}
	h = bootAgain(path, false)
	if name, _ := h.ActiveUI(); name != "" {
		t.Fatalf("after two short startups %q is still on", name)
	}
	if len(h.reports) != 1 || !strings.Contains(h.reports[0], `"Compact"`) || !strings.Contains(h.reports[0], "Default") {
		t.Fatalf("dispatcher report %v", h.reports)
	}
	h = newHub(map[string]string{}, path)
	s := h.Settings()
	if s.Interface != "" || h.uiStarts != 0 {
		t.Fatalf("saved interface %q, count %d", s.Interface, h.uiStarts)
	}
	if _, ok := s.Interfaces["Compact"]; !ok {
		t.Fatal("switching back to Default deleted the interface")
	}
}

func TestLongRunResetsCrashCount(t *testing.T) {
	path, _ := uiHub(t)
	bootAgain(path, false).uiStarted()
	bootAgain(path, false)
	h := bootAgain(path, false)
	if name, _ := h.ActiveUI(); name != "Compact" {
		t.Fatalf("a run past the grace period should reset the count, got %q", name)
	}
	h = bootAgain(path, false)
	if name, _ := h.ActiveUI(); name != "" {
		t.Fatalf("two short startups after the reset left %q on", name)
	}
}

func TestRunWithoutInterfaceBreaksTheRow(t *testing.T) {
	path, _ := uiHub(t)
	bootAgain(path, false)
	bootAgain(path, true)
	bootAgain(path, false)
	h := bootAgain(path, false)
	if name, _ := h.ActiveUI(); name != "Compact" {
		t.Fatalf("a skipped run between short startups should reset the count, got %q", name)
	}
}

func TestRepickingTheSameInterfaceEndsTheSkip(t *testing.T) {
	_, h := uiHub(t)
	h.bootUI(true)
	h.UpdateSettings(func(s *Settings) { s.Theme = "Gruvbox" })
	if name, _ := h.ActiveUI(); name != "" {
		t.Fatalf("a settings change unrelated to the interface ended the skip: %q", name)
	}
	h.UseUI(func(s *Settings) { s.Interface = "Compact" })
	if name, _ := h.ActiveUI(); name != "Compact" {
		t.Fatalf("re-picking the saved interface during a skipped run shows %q", name)
	}
}
