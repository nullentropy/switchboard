package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wallSession(t *testing.T, h *Hub, m Meta) *Agent {
	t.Helper()
	m.ID = newID()
	a := newAgent(h, m)
	h.mu.Lock()
	h.agents = append(h.agents, a)
	h.mu.Unlock()
	return a
}

func onWall(h *Hub, a *Agent) bool {
	for _, x := range h.Agents() {
		if x == a {
			return true
		}
	}
	return false
}

func TestTheDispatcherRemovesAStoppedSessionAndRefusesOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(map[string]string{}, path)
	m := &mcpServer{hub: h}

	plain := wallSession(t, h, Meta{Title: "Marvin", Cwd: t.TempDir()})
	busy := wallSession(t, h, Meta{Title: "Deep Thought", Cwd: t.TempDir()})
	busy.State = Working
	h.save()

	if _, err := m.call("remove_session", map[string]any{"session": "Agrajag"}); err == nil || !strings.Contains(err.Error(), "no session matches") {
		t.Fatalf("unknown name: %v", err)
	}
	if _, err := m.call("remove_session", map[string]any{"session": "Deep Thought"}); err == nil || !strings.Contains(err.Error(), "Working") {
		t.Fatalf("working session: %v", err)
	}
	if !onWall(h, busy) {
		t.Fatal("a working session was removed")
	}
	if _, err := m.call("remove_session", map[string]any{"session": "Marvin"}); err != nil {
		t.Fatal(err)
	}
	if onWall(h, plain) {
		t.Fatal("the stopped session is still on the wall")
	}
	if b := read(t, path); strings.Contains(b, plain.Meta.ID) || !strings.Contains(b, busy.Meta.ID) {
		t.Fatalf("saved state after remove: %s", b)
	}
}

func TestRemovingAWorktreeSessionDropsACleanWorktreeAndRefusesOneWithWork(t *testing.T) {
	dir := testRepo(t)
	home := t.TempDir()
	h := newHub(map[string]string{}, filepath.Join(t.TempDir(), "state.json"))
	m := &mcpServer{hub: h}
	session := func(title string) (*Agent, Meta) {
		cwd, branch, err := addWorktree(dir, home, title)
		if err != nil {
			t.Fatal(err)
		}
		a := wallSession(t, h, Meta{Title: title, Cwd: cwd, Repo: repoRoot(dir), Worktree: cwd, WorkBranch: branch})
		if _, err := checkpoint(cwd, checkpointRef(a.Meta.ID, 0)); err != nil {
			t.Fatal(err)
		}
		return a, a.Meta
	}
	branchThere := func(b string) bool { out, _ := git(dir, nil, "branch", "--list", b); return out != "" }
	refsOf := func(id string) string {
		out, _ := git(dir, nil, "for-each-ref", "refs/switchboard/checkpoints/"+id+"/")
		return out
	}

	clean, cm := session("heart-of-gold")
	dirty, dm := session("vogon-poetry")
	os.WriteFile(filepath.Join(dm.Cwd, "towel.txt"), []byte("mine"), 0o644)

	_, err := m.call("remove_session", map[string]any{"session": "vogon-poetry"})
	if err == nil || !strings.Contains(err.Error(), "1 uncommitted changes") {
		t.Fatalf("dirty worktree: %v", err)
	}
	if !onWall(h, dirty) || !branchThere(dm.WorkBranch) || refsOf(dm.ID) == "" || read(t, filepath.Join(dm.Cwd, "towel.txt")) != "mine" {
		t.Fatal("refusing a dirty worktree changed something")
	}

	if _, err := m.call("remove_session", map[string]any{"session": "heart-of-gold"}); err != nil {
		t.Fatal(err)
	}
	if onWall(h, clean) {
		t.Fatal("the clean session is still on the wall")
	}
	if _, err := os.Stat(cm.Cwd); !os.IsNotExist(err) || branchThere(cm.WorkBranch) {
		t.Fatal("the clean worktree or its branch is still there")
	}
	if refsOf(cm.ID) != "" {
		t.Fatal("the checkpoint refs are still there")
	}
}
