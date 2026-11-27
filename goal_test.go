package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	caution "github.com/nullentropy/caution"
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

func TestADoneGoalLeavesTheBoardEvenWhileTheBoardShowsOnlyThatGoal(t *testing.T) {
	h := newHub(map[string]string{}, filepath.Join(t.TempDir(), "state.json"))
	m := &mcpServer{hub: h}
	arthur := wallSession(t, h, Meta{Title: "Arthur", Cwd: t.TempDir(), Goal: "Find the Question"})
	zaphod := wallSession(t, h, Meta{Title: "Zaphod", Cwd: t.TempDir(), Goal: "Steal the Heart of Gold"})
	v := &View{hub: h, tiles: map[*Agent]*tile{}, body: caution.Panel()}
	v.buildBoard()
	v.addTile(arthur)
	v.addTile(zaphod)

	v.filterGoal("Find the Question")
	if _, err := m.call("set_goal_done", map[string]any{"goal": "Find the Question", "done": true}); err != nil {
		t.Fatal(err)
	}
	v.layoutBoard()
	if v.goal != "" || len(v.shown) != 1 || v.shown[0] != zaphod {
		t.Fatalf("after the dispatcher marked the shown goal done: filter %q, %d on the board", v.goal, len(v.shown))
	}

	v.showDone = true
	v.filterGoal("Find the Question")
	if len(v.shown) != 1 || v.shown[0] != arthur {
		t.Fatal("clicking the done goal did not show its sessions")
	}
	v.showDone = false
	v.layoutBoard()
	if v.goal != "" || len(v.shown) != 1 || v.shown[0] != zaphod {
		t.Fatalf("after hiding the done goals: filter %q, %d on the board", v.goal, len(v.shown))
	}
}

func TestTheBoardRebuildsOnlyWhenAGoalSessionChangesWhatItShows(t *testing.T) {
	h := newHub(map[string]string{}, "")
	trillian := wallSession(t, h, Meta{Title: "Trillian", Cwd: t.TempDir(), Goal: "Find the Question"})
	trillian.State = NeedsYou
	trillian.Pending = []*Pending{{ReqID: "babel", Tool: "Bash"}}
	v := &View{hub: h, tiles: map[*Agent]*tile{}, body: caution.Panel()}
	v.buildBoard()
	v.addTile(trillian)
	v.layoutBoard()

	rebuilds := 0
	badge := v.tiles[trillian].badge.Children()[0]
	seen := func() {
		if b := v.tiles[trillian].badge.Children()[0]; b != badge {
			badge = b
			rebuilds++
		}
	}
	for i := range 50 {
		trillian.mu.Lock()
		trillian.Activity = fmt.Sprintf("search agent step %d", i)
		trillian.mu.Unlock()
		v.updateTile(trillian, trillian.Snapshot())
		seen()
	}
	if rebuilds != 0 {
		t.Fatalf("the board rebuilt %d times for 50 activity updates", rebuilds)
	}

	trillian.mu.Lock()
	trillian.State, trillian.Pending = Working, nil
	trillian.mu.Unlock()
	v.updateTile(trillian, trillian.Snapshot())
	seen()
	h.Rename(trillian, "Trillian McMillan")
	v.updateTile(trillian, trillian.Snapshot())
	seen()
	if rebuilds != 2 {
		t.Fatalf("the board rebuilt %d times for a state change and a rename", rebuilds)
	}
}

var hitchhikerGoals = []string{
	"Find the Question", "Steal the Heart of Gold", "Build Earth", "Rescue the Mice", "Calm Marvin",
	"Translate Babel Fish", "Escape the Vogons", "Visit Milliways", "Restore Magrathea", "Dodge the Bistromath",
	"Find a Towel", "Read Vogon Poetry",
}

func TestOpenGoalsFitOnTwoRowsAt1440Wide(t *testing.T) {
	for _, n := range []int{1, 5, 12} {
		h := newHub(map[string]string{}, "")
		v := &View{hub: h, tiles: map[*Agent]*tile{}, body: caution.Panel()}
		v.buildBoard()
		for i, g := range hitchhikerGoals[:n] {
			a := wallSession(t, h, Meta{Title: fmt.Sprintf("Arthur %d", i), Cwd: t.TempDir(), Goal: g})
			if i == 0 {
				a.State = NeedsYou
			}
			v.addTile(a)
			v.addTile(wallSession(t, h, Meta{Title: fmt.Sprintf("Ford %d", i), Cwd: t.TempDir(), Goal: g}))
		}
		v.layoutBoard()
		rows, chips := v.goals.Children(), 0
		for _, r := range rows {
			chips += len(r.Children())
		}
		if chips != n {
			t.Fatalf("%d goals drew %d chips", n, chips)
		}
		if want := map[int]int{1: 1, 5: 1, 12: 2}[n]; len(rows) > want {
			t.Fatalf("%d goals took %d rows", n, len(rows))
		}
	}
}

func TestTheDoneGoalsChipShowsDoneGoalsAsChipsAfterTheOpenOnes(t *testing.T) {
	h := newHub(map[string]string{}, filepath.Join(t.TempDir(), "state.json"))
	v := &View{hub: h, tiles: map[*Agent]*tile{}, body: caution.Panel()}
	v.buildBoard()
	for _, g := range hitchhikerGoals[:3] {
		v.addTile(wallSession(t, h, Meta{Title: g, Cwd: t.TempDir(), Goal: g}))
	}
	h.SetGoalDone("Build Earth", true, false)
	h.SetGoalDone("Find the Question", true, false)
	count := func() int {
		v.layoutBoard()
		n := 0
		for _, r := range v.goals.Children() {
			n += len(r.Children())
		}
		return n
	}
	if n := count(); n != 2 {
		t.Fatalf("with done goals hidden: %d chips, want the open goal and the toggle", n)
	}
	v.showDone = true
	if n := count(); n != 4 {
		t.Fatalf("with done goals shown: %d chips", n)
	}
}

func TestChipsWrapWhenTheNextOneDoesNotFit(t *testing.T) {
	got := wrapChips([]float64{100, 100, 100, 250, 40}, 320, 8)
	want := [][2]int{{0, 3}, {3, 5}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	if got := wrapChips([]float64{500}, 320, 8); fmt.Sprint(got) != "[[0 1]]" {
		t.Fatalf("an oversized chip: %v", got)
	}
}
