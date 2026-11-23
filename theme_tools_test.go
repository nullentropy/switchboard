package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheDispatcherSavesACustomThemeThatSurvivesARestart(t *testing.T) {
	for name, toks := range themes {
		for k := range tokenRoles {
			if toks[k] == "" {
				t.Fatalf("built-in %s has no %s", name, k)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(map[string]string{}, path)
	m := &mcpServer{hub: h}

	if _, err := m.call("save_theme", map[string]any{"name": "Paper", "colors": map[string]any{}}); err == nil {
		t.Fatal("overwrote a built-in theme")
	}
	_, err := m.call("save_theme", map[string]any{"name": "Magrathea", "colors": map[string]any{"bg": "#ZZZ", "glow": "#ffffff"}})
	if err == nil || !strings.Contains(err.Error(), "unknown token glow") || !strings.Contains(err.Error(), "bg:") {
		t.Fatalf("bad colours accepted: %v", err)
	}
	if _, err := m.call("save_theme", map[string]any{"name": "Magrathea", "base": "Paper", "colors": map[string]any{"bg": "#002B36"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	set := newHub(map[string]string{}, path).Settings()
	got := themeTokens(set, set.Theme)
	if set.Theme != "Magrathea" || got["bg"] != "#002b36" || got["ink"] != themes["Paper"]["ink"] {
		t.Fatalf("after restart theme %s bg %s ink %s", set.Theme, got["bg"], got["ink"])
	}
}
