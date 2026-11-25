package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemovingAGoalTakesItsSessionsOffAndForgetsItsDoneState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	h := newHub(map[string]string{}, path)
	arthur := wallSession(t, h, Meta{Title: "Arthur", Cwd: t.TempDir(), Goal: "Find the Question", Parent: "dispatcher"})
	ford := wallSession(t, h, Meta{Title: "Ford", Cwd: t.TempDir(), Goal: "Find the Question"})
	zaphod := wallSession(t, h, Meta{Title: "Zaphod", Cwd: t.TempDir(), Goal: "Steal the Heart of Gold"})
	h.SetGoalDone("Find the Question", true, false)
	h.Dispatcher.State = Working

	if n := h.RemoveGoal("Find the Question", true, true); n != 2 {
		t.Fatalf("removed %d sessions", n)
	}
	if onWall(h, arthur) || onWall(h, ford) || !onWall(h, zaphod) {
		t.Fatal("removing the goal left its sessions or took another goal's")
	}
	if h.GoalDone("Find the Question") {
		t.Fatal("the removed goal is still done")
	}
	if b := read(t, path); strings.Contains(b, "Find the Question") || !strings.Contains(b, zaphod.Meta.ID) {
		t.Fatalf("saved state after removing the goal: %s", b)
	}
	if len(h.reports) != 1 || !strings.Contains(h.reports[0], "Find the Question: the user removed the goal") {
		t.Fatalf("dispatcher reports: %q", h.reports)
	}
	if h.RemoveGoal("", true, true) != 0 || !onWall(h, zaphod) {
		t.Fatal("an empty goal name removed something")
	}
}

func TestRemovingAGoalDropsCleanWorktreesAndKeepsOrDeletesOnesWithWork(t *testing.T) {
	dir := testRepo(t)
	home := t.TempDir()
	h := newHub(map[string]string{}, filepath.Join(t.TempDir(), "state.json"))
	session := func(title, goal string) Meta {
		cwd, branch, err := addWorktree(dir, home, title)
		if err != nil {
			t.Fatal(err)
		}
		return wallSession(t, h, Meta{Title: title, Cwd: cwd, Repo: repoRoot(dir), Worktree: cwd, WorkBranch: branch, Goal: goal}).Meta
	}
	gone := func(m Meta) bool {
		_, err := os.Stat(m.Cwd)
		out, _ := git(dir, nil, "branch", "--list", m.WorkBranch)
		return os.IsNotExist(err) && out == ""
	}

	clean := session("heart-of-gold", "Improbability")
	dirty := session("vogon-poetry", "Improbability")
	os.WriteFile(filepath.Join(dirty.Cwd, "towel.txt"), []byte("mine"), 0o644)
	if left := h.goalLeftovers("Improbability"); len(left) != 1 || !strings.Contains(left[0], "vogon-poetry") {
		t.Fatalf("leftovers: %q", left)
	}
	h.RemoveGoal("Improbability", true, true)
	if len(h.Agents()) != 0 {
		t.Fatal("sessions are still on the wall")
	}
	if !gone(clean) || gone(dirty) || read(t, filepath.Join(dirty.Cwd, "towel.txt")) != "mine" {
		t.Fatal("keep worktrees did not drop the clean one and keep the one with work")
	}

	thrown := session("babel-fish", "Pan Galactic")
	os.WriteFile(filepath.Join(thrown.Cwd, "towel.txt"), []byte("mine"), 0o644)
	h.RemoveGoal("Pan Galactic", false, true)
	if !gone(thrown) {
		t.Fatal("delete worktrees kept the worktree with work")
	}
}

func TestTheDispatcherRemovesAGoalOrRefusesAllOfIt(t *testing.T) {
	dir := testRepo(t)
	h := newHub(map[string]string{}, filepath.Join(t.TempDir(), "state.json"))
	m := &mcpServer{hub: h}
	cwd, branch, err := addWorktree(dir, t.TempDir(), "slartibartfast")
	if err != nil {
		t.Fatal(err)
	}
	fjords := wallSession(t, h, Meta{Title: "Slartibartfast", Cwd: cwd, Repo: repoRoot(dir), Worktree: cwd, WorkBranch: branch, Goal: "Build Earth"})
	trillian := wallSession(t, h, Meta{Title: "Trillian", Cwd: t.TempDir(), Goal: "Build Earth"})
	trillian.State = Working
	os.WriteFile(filepath.Join(cwd, "fjords.txt"), []byte("Norway"), 0o644)

	if _, err := m.call("remove_goal", map[string]any{"goal": "Magrathea"}); err == nil || !strings.Contains(err.Error(), "no session has the goal") {
		t.Fatalf("unknown goal: %v", err)
	}
	if _, err := m.call("remove_goal", map[string]any{"goal": ""}); err == nil {
		t.Fatal("an empty goal was accepted")
	}
	_, err = m.call("remove_goal", map[string]any{"goal": "Build Earth"})
	if err == nil || !strings.Contains(err.Error(), "Slartibartfast's worktree") || !strings.Contains(err.Error(), "1 uncommitted changes") {
		t.Fatalf("goal with work: %v", err)
	}
	if !onWall(h, fjords) || !onWall(h, trillian) || trillian.Snapshot().State != Working {
		t.Fatal("refusing the goal changed something")
	}

	os.Remove(filepath.Join(cwd, "fjords.txt"))
	out, err := m.call("remove_goal", map[string]any{"goal": "Build Earth"})
	if err != nil || out != "Removed Build Earth and its 2 sessions." {
		t.Fatalf("remove_goal: %q %v", out, err)
	}
	if len(h.Agents()) != 0 {
		t.Fatal("the goal's sessions are still on the wall")
	}
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Fatal("the clean worktree is still there")
	}
	if len(h.reports) != 0 {
		t.Fatalf("the dispatcher was told about its own removal: %q", h.reports)
	}
}
