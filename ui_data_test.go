package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"switchboard/platform"
)

func uiFixture(t *testing.T) (*Hub, time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = time.Now })
	uiLoadRead = func() platform.Load { return platform.Load{Load1: 7.5, Load5: 4, Cores: 10, MemFree: 42} }
	uiLoadAt = time.Time{}
	t.Cleanup(func() { uiLoadRead = platform.ReadLoad; uiLoadAt = time.Time{} })
	return newHub(map[string]string{}, filepath.Join(t.TempDir(), "state.json")), now
}

func uiSet(a *Agent, st State, cost float64) *Agent {
	a.State, a.Cost = st, cost
	return a
}

func uiIDs(list any) []string {
	var out []string
	for _, r := range list.([]any) {
		out = append(out, r.(map[string]any)["id"].(string))
	}
	return out
}

func TestUIDataSessionRowHasTheListSessionsFieldsAndNoTranscript(t *testing.T) {
	h, now := uiFixture(t)
	arthur := wallSession(t, h, Meta{Title: "Arthur", Provider: "codex", Cwd: "/galaxy/earth", Goal: "Find the Question"})
	arthur.State = NeedsYou
	arthur.Cost = 4.2
	arthur.Branch = "towel"
	arthur.Activity = "Reading the Guide"
	arthur.TurnFrom = now.Add(-75 * time.Second)
	arthur.Pending = []*Pending{{Tool: "Bash", Summary: "make tea"}}
	arthur.queue = []Queued{{ID: 1, Text: "Don't panic"}, {ID: 2, Text: "Bring a towel"}}
	arthur.Entries = []Entry{
		{Kind: EntryUser, Text: "What is the answer?"},
		{Kind: EntryAssistant, Text: "Forty-two.\nYou won't like it."},
		{Kind: EntryTool, Tool: "Bash", Text: "deep thought"},
	}

	d := uiData(h, uiViewState{Selected: arthur})
	got := d["sessions"].([]any)[0]
	want := map[string]any{
		"id": arthur.Meta.ID, "title": "Arthur", "agent": "codex", "goal": "Find the Question", "goalDone": false,
		"folder": "/galaxy/earth", "branch": "towel", "state": "Needs you", "activity": "Reading the Guide",
		"cost": 4.2, "waiting": "Bash make tea", "queued": 2.0, "lastReply": "Forty-two. …",
		"elapsed": 75.0, "selected": true, "key": 1.0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("session row\n got %#v\nwant %#v", got, want)
	}
}

func TestUIDataElapsedCountsOnlyWhileATurnIsLive(t *testing.T) {
	h, now := uiFixture(t)
	for _, st := range []State{Starting, Working, NeedsYou, Ready, Background, Failed} {
		a := uiSet(wallSession(t, h, Meta{Title: st.String(), Cwd: "/heart-of-gold"}), st, 0)
		a.TurnFrom = now.Add(-3 * time.Minute)
	}
	uiSet(wallSession(t, h, Meta{Title: "Marvin", Cwd: "/heart-of-gold"}), Working, 0)
	got := map[string]any{}
	for _, r := range uiData(h, uiViewState{})["sessions"].([]any) {
		row := r.(map[string]any)
		got[row["title"].(string)] = row["elapsed"]
	}
	want := map[string]any{"Starting": 180.0, "Working": 180.0, "Needs you": 180.0, "Ready": 0.0, "Background": 0.0, "Failed": 0.0, "Marvin": 0.0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("elapsed by session: %v", got)
	}
}

func TestUIDataListsSessionsInBoardOrderWithGroupsAndKeys(t *testing.T) {
	h, _ := uiFixture(t)
	zaphod := uiSet(wallSession(t, h, Meta{Title: "Zaphod", Cwd: "/heart-of-gold"}), Ready, 0)
	ford := uiSet(wallSession(t, h, Meta{Title: "Ford", Cwd: "/heart-of-gold"}), Working, 0)
	trillian := uiSet(wallSession(t, h, Meta{Title: "Trillian", Cwd: "/heart-of-gold"}), NeedsYou, 0)
	marvin := uiSet(wallSession(t, h, Meta{Title: "Marvin", Cwd: "/heart-of-gold"}), Stopped, 0)
	eddie := uiSet(wallSession(t, h, Meta{Title: "Eddie", Cwd: "/heart-of-gold"}), Background, 0)
	slarti := uiSet(wallSession(t, h, Meta{Title: "Slartibartfast", Cwd: "/heart-of-gold"}), Failed, 0)
	fenchurch := uiSet(wallSession(t, h, Meta{Title: "Fenchurch", Cwd: "/heart-of-gold"}), Interrupted, 0)
	agrajag := uiSet(wallSession(t, h, Meta{Title: "Agrajag", Cwd: "/heart-of-gold"}), Waiting, 0)
	starting := uiSet(wallSession(t, h, Meta{Title: "Deep Thought", Cwd: "/heart-of-gold"}), Starting, 0)
	var extra []*Agent
	for i := range 2 {
		extra = append(extra, uiSet(wallSession(t, h, Meta{Title: fmt.Sprintf("Vogon %d", i), Cwd: "/heart-of-gold"}), Stopped, 0))
	}

	d := uiData(h, uiViewState{Selected: ford})
	wantOrder := []string{trillian.Meta.ID, ford.Meta.ID, starting.Meta.ID, eddie.Meta.ID, agrajag.Meta.ID, slarti.Meta.ID,
		fenchurch.Meta.ID, zaphod.Meta.ID, marvin.Meta.ID, extra[0].Meta.ID, extra[1].Meta.ID}
	if got := uiIDs(d["sessions"]); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("board order %v, want %v", got, wantOrder)
	}
	for i, r := range d["sessions"].([]any) {
		row := r.(map[string]any)
		wantKey := float64(i + 1)
		if i >= 9 {
			wantKey = 0
		}
		if row["key"] != wantKey || row["selected"] != (row["id"] == ford.Meta.ID) {
			t.Fatalf("%s: key %v selected %v", row["title"], row["key"], row["selected"])
		}
	}
	groups := d["groups"].(map[string]any)
	wantGroups := map[string][]string{
		"need": {trillian.Meta.ID}, "work": {ford.Meta.ID, starting.Meta.ID}, "bg": {eddie.Meta.ID},
		"wait": {agrajag.Meta.ID}, "fail": {slarti.Meta.ID}, "int": {fenchurch.Meta.ID}, "done": {zaphod.Meta.ID},
		"idle": {marvin.Meta.ID, extra[0].Meta.ID, extra[1].Meta.ID},
	}
	if len(groups) != len(wantGroups) {
		t.Fatalf("groups: %v", groups)
	}
	for g, want := range wantGroups {
		if got := uiIDs(groups[g]); !reflect.DeepEqual(got, want) {
			t.Fatalf("groups.%s = %v, want %v", g, got, want)
		}
	}
}

func TestUIDataEmptyWallHasEmptyListsAndZeroTotals(t *testing.T) {
	h, _ := uiFixture(t)
	d := uiData(h, uiViewState{})
	if len(d["sessions"].([]any)) != 0 || len(d["goals"].([]any)) != 0 || len(d["limits"].([]any)) != 0 {
		t.Fatalf("empty wall: %v", d)
	}
	for _, g := range uiGroups {
		if l, ok := d["groups"].(map[string]any)[g].([]any); !ok || len(l) != 0 {
			t.Fatalf("groups.%s = %#v", g, d["groups"].(map[string]any)[g])
		}
	}
	want := map[string]any{"need": 0.0, "work": 0.0, "done": 0.0, "spent": 0.0}
	if !reflect.DeepEqual(d["totals"], want) {
		t.Fatalf("totals %v", d["totals"])
	}
}

func TestUIDataFiltersMatchTheBoardWhileGoalsAndTotalsCountEverySession(t *testing.T) {
	h, _ := uiFixture(t)
	arthur := uiSet(wallSession(t, h, Meta{Title: "Arthur", Cwd: "/galaxy/earth", Goal: "Find the Question"}), Working, 1.25)
	ford := uiSet(wallSession(t, h, Meta{Title: "Ford", Cwd: "/galaxy/betelgeuse", Goal: "Find the Question"}), NeedsYou, 2)
	zaphod := uiSet(wallSession(t, h, Meta{Title: "Zaphod", Cwd: "/galaxy/betelgeuse", Goal: "Steal the Heart of Gold"}), Ready, 0.5)
	marvin := uiSet(wallSession(t, h, Meta{Title: "Marvin", Cwd: "/galaxy/earth"}), Ready, 0.25)
	h.doneGoals["Steal the Heart of Gold"] = true

	d := uiData(h, uiViewState{})
	if got, want := uiIDs(d["sessions"]), []string{ford.Meta.ID, arthur.Meta.ID, marvin.Meta.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unfiltered board %v, want %v without the done goal's session", got, want)
	}
	if got, want := uiIDs(uiData(h, uiViewState{Goal: "Find the Question"})["sessions"]), []string{ford.Meta.ID, arthur.Meta.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("goal filter %v, want %v", got, want)
	}
	if got, want := uiIDs(uiData(h, uiViewState{Goal: "Steal the Heart of Gold"})["sessions"]), []string{zaphod.Meta.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("filtering to a done goal %v, want %v", got, want)
	}
	if got, want := uiIDs(uiData(h, uiViewState{Project: "earth"})["sessions"]), []string{arthur.Meta.ID, marvin.Meta.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("project filter %v, want %v", got, want)
	}

	if want := map[string]any{"need": 1.0, "work": 1.0, "done": 2.0, "spent": 4.0}; !reflect.DeepEqual(d["totals"], want) {
		t.Fatalf("totals %v, want %v", d["totals"], want)
	}
	goals := d["goals"].([]any)
	if len(goals) != 2 {
		t.Fatalf("goals %v", goals)
	}
	for i, want := range []struct {
		name string
		done bool
		ids  []string
	}{{"Find the Question", false, []string{arthur.Meta.ID, ford.Meta.ID}}, {"Steal the Heart of Gold", true, []string{zaphod.Meta.ID}}} {
		g := goals[i].(map[string]any)
		if len(g) != 3 || g["name"] != want.name || g["done"] != want.done || !reflect.DeepEqual(uiIDs(g["sessions"]), want.ids) {
			t.Fatalf("goal %d = %v, want %+v", i, g, want)
		}
	}
	zrow := goals[1].(map[string]any)["sessions"].([]any)[0].(map[string]any)
	if zrow["goalDone"] != true || zrow["key"] != 0.0 {
		t.Fatalf("hidden done-goal session row %v", zrow)
	}
}

func TestUIDataLoadLimitsUpdateAndView(t *testing.T) {
	h, now := uiFixture(t)
	h.limits = map[string]Limits{
		"codex":  {Week: 12, HasWeek: true, WeekResets: now.Add(72 * time.Hour)},
		"claude": {Five: 64, Week: 30, HasFive: true, HasWeek: true, FiveResets: now.Add(2 * time.Hour)},
		"gemini": {},
	}
	h.updateTag = "v4.2.0"

	d := uiData(h, uiViewState{Columns: 3, Width: 1420, Goal: "Find the Question", Project: "heart-of-gold"})
	if want := map[string]any{"overloaded": false, "load": 7.5, "cores": 10.0, "freeMemory": 42.0}; !reflect.DeepEqual(d["load"], want) {
		t.Fatalf("load %v, want %v", d["load"], want)
	}
	wantLimits := []any{
		map[string]any{"provider": "claude", "name": "Claude", "five": 64.0, "week": 30.0, "hasFive": true, "hasWeek": true, "fiveResets": "14:00", "weekResets": ""},
		map[string]any{"provider": "codex", "name": "Codex", "five": 0.0, "week": 12.0, "hasFive": false, "hasWeek": true, "fiveResets": "", "weekResets": "Sun 12:00"},
	}
	if !reflect.DeepEqual(d["limits"], wantLimits) {
		t.Fatalf("limits\n got %v\nwant %v", d["limits"], wantLimits)
	}
	if want := map[string]any{"ready": true, "version": "4.2.0"}; !reflect.DeepEqual(d["update"], want) {
		t.Fatalf("update %v", d["update"])
	}
	if want := map[string]any{"columns": 3.0, "width": 1420.0, "goal": "Find the Question", "project": "heart-of-gold"}; !reflect.DeepEqual(d["view"], want) {
		t.Fatalf("view %v", d["view"])
	}
	want := []string{"goals", "groups", "limits", "load", "sessions", "totals", "update", "view"}
	var keys []string
	for k := range d {
		keys = append(keys, k)
	}
	if len(keys) != len(want) {
		t.Fatalf("top-level fields %v, want %v", keys, want)
	}

	h.updateTag = ""
	if want := map[string]any{"ready": false, "version": ""}; !reflect.DeepEqual(uiData(h, uiViewState{})["update"], want) {
		t.Fatalf("no update: %v", uiData(h, uiViewState{})["update"])
	}
}

func TestUIDataReadsTheMachineLoadAtMostOnceEveryFiveSeconds(t *testing.T) {
	h, now := uiFixture(t)
	reads := 0
	uiLoadRead = func() platform.Load { reads++; return platform.Load{Load1: float64(reads), Cores: 8, MemFree: 50} }
	for _, step := range []time.Duration{0, time.Second, 4 * time.Second} {
		clock = func() time.Time { return now.Add(step) }
		uiData(h, uiViewState{})
	}
	if reads != 1 {
		t.Fatalf("read the load %d times within five seconds", reads)
	}
	clock = func() time.Time { return now.Add(5 * time.Second) }
	if got := uiData(h, uiViewState{})["load"].(map[string]any)["load"]; reads != 2 || got != 2.0 {
		t.Fatalf("after five seconds: %d reads, load %v", reads, got)
	}
	uiLoadRead = func() platform.Load { return platform.Load{Load1: 30, Cores: 8, MemFree: 5} }
	clock = func() time.Time { return now.Add(time.Minute) }
	if got := uiData(h, uiViewState{})["load"].(map[string]any)["overloaded"]; got != true {
		t.Fatalf("overloaded = %v", got)
	}
}
