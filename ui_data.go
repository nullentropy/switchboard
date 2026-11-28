package main

import (
	"sort"
	"strings"
	"sync"
	"time"

	"switchboard/platform"
)

type uiViewState struct {
	Selected *Agent
	Goal     string
	Project  string
	Columns  int
	Width    float64
}

var uiGroups = []string{"need", "work", "bg", "wait", "fail", "int", "done", "idle"}

const uiLoadTTL = 5 * time.Second

var (
	uiLoadRead = platform.ReadLoad
	uiLoadMu   sync.Mutex
	uiLoadAt   time.Time
	uiLoadLast platform.Load
)

func uiLoadSample(now time.Time) platform.Load {
	uiLoadMu.Lock()
	defer uiLoadMu.Unlock()
	if uiLoadAt.IsZero() || now.Sub(uiLoadAt) >= uiLoadTTL || now.Before(uiLoadAt) {
		uiLoadLast, uiLoadAt = uiLoadRead(), now
	}
	return uiLoadLast
}

type uiSession struct {
	a    *Agent
	snap Snapshot
	row  map[string]any
}

func uiData(h *Hub, vs uiViewState) map[string]any {
	now := clock()
	var all []*uiSession
	for _, a := range h.Agents() {
		all = append(all, &uiSession{a: a, snap: a.Snapshot()})
	}

	var shown []*uiSession
	for _, g := range uiGroups {
		for _, s := range all {
			m := s.snap.Meta
			if groupOf(s.snap.State) == g && (vs.Goal == "" || m.Goal == vs.Goal) &&
				(m.Goal == vs.Goal || !h.GoalDone(m.Goal)) &&
				(vs.Project == "" || projectOf(m) == vs.Project) {
				shown = append(shown, s)
			}
		}
	}
	keys := map[*uiSession]int{}
	for i, s := range shown {
		if i < 9 {
			keys[s] = i + 1
		}
	}
	for _, s := range all {
		s.row = uiSessionRow(h, s.snap, keys[s], s.a == vs.Selected, now)
	}

	sessions := []any{}
	groups := map[string]any{}
	for _, g := range uiGroups {
		groups[g] = []any{}
	}
	for _, s := range shown {
		sessions = append(sessions, s.row)
		g := groupOf(s.snap.State)
		groups[g] = append(groups[g].([]any), s.row)
	}

	byGoal := map[string][]any{}
	var names []string
	count := map[string]int{}
	spent := 0.0
	for _, s := range all {
		count[groupOf(s.snap.State)]++
		spent += s.snap.Cost
		if g := s.snap.Meta.Goal; g != "" {
			if _, ok := byGoal[g]; !ok {
				names = append(names, g)
			}
			byGoal[g] = append(byGoal[g], s.row)
		}
	}
	sort.Strings(names)
	goals := []any{}
	for _, n := range names {
		goals = append(goals, map[string]any{"name": n, "done": h.GoalDone(n), "sessions": byGoal[n]})
	}

	load := uiLoadSample(now)
	version := strings.TrimPrefix(h.UpdateReady(), "v")
	return map[string]any{
		"sessions": sessions,
		"groups":   groups,
		"goals":    goals,
		"totals": map[string]any{
			"need": float64(count["need"]), "work": float64(count["work"]), "done": float64(count["done"]), "spent": spent,
		},
		"load": map[string]any{
			"overloaded": load.Overloaded(), "load": load.Load1, "cores": float64(load.Cores), "freeMemory": float64(load.MemFree),
		},
		"limits": uiLimits(h.Limits(), now),
		"update": map[string]any{"ready": version != "", "version": version},
		"view": map[string]any{
			"columns": float64(vs.Columns), "width": vs.Width, "goal": vs.Goal, "project": vs.Project,
		},
	}
}

func uiSessionRow(h *Hub, s Snapshot, key int, selected bool, now time.Time) map[string]any {
	waiting := ""
	if len(s.Pending) > 0 {
		waiting = strings.TrimSpace(s.Pending[0].Tool + " " + s.Pending[0].Summary)
	}
	since := 0.0
	if (s.State == Working || s.State == Starting || s.State == NeedsYou) && !s.TurnFrom.IsZero() {
		since = float64(int(now.Sub(s.TurnFrom).Seconds()))
	}
	return map[string]any{
		"id":        s.Meta.ID,
		"title":     s.Meta.Title,
		"agent":     providerOf(s.Meta),
		"goal":      s.Meta.Goal,
		"goalDone":  h.GoalDone(s.Meta.Goal),
		"folder":    s.Meta.Cwd,
		"branch":    s.Branch,
		"state":     s.State.String(),
		"activity":  s.Activity,
		"cost":      s.Cost,
		"waiting":   waiting,
		"queued":    float64(len(s.Queued)),
		"lastReply": oneLine(lastReply(s.Entries)),
		"elapsed":   since,
		"selected":  selected,
		"key":       float64(key),
	}
}

func uiResets(at, now time.Time) string {
	switch {
	case at.IsZero():
		return ""
	case at.Sub(now) < 24*time.Hour:
		return at.Format("15:04")
	}
	return at.Format("Mon 15:04")
}

func uiLimits(all map[string]Limits, now time.Time) []any {
	out := []any{}
	for _, p := range providers {
		l, ok := all[p]
		if !ok || (!l.HasFive && !l.HasWeek) {
			continue
		}
		out = append(out, map[string]any{
			"provider":   p,
			"name":       providerName(p),
			"five":       l.Five,
			"week":       l.Week,
			"hasFive":    l.HasFive,
			"hasWeek":    l.HasWeek,
			"fiveResets": uiResets(l.FiveResets, now),
			"weekResets": uiResets(l.WeekResets, now),
		})
	}
	return out
}
