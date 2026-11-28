package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"switchboard/platform"
)

func (a *Agent) pid() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.p == nil || a.p.cmd.Process == nil {
		return 0
	}
	return a.p.cmd.Process.Pid
}

type sessionLoad struct {
	a *Agent
	u platform.Usage
}

func (h *Hub) sessionLoads() []sessionLoad {
	procs := platform.ReadProcs()
	var out []sessionLoad
	for _, a := range h.Agents() {
		if pid := a.pid(); pid != 0 {
			if u := platform.TreeUsage(procs, pid); u.RSS > 0 {
				out = append(out, sessionLoad{a, u})
			}
		}
	}
	slices.SortFunc(out, func(x, y sessionLoad) int { return int(y.u.CPU - x.u.CPU) })
	return out
}

func (l sessionLoad) String() string {
	s := l.a.Snapshot()
	t := fmt.Sprintf("%s (%s, %s): %.0f%% CPU, %.1f GB", s.Meta.Title, s.Meta.ID[:8], s.State, l.u.CPU, float64(l.u.RSS)/(1<<20))
	if l.u.Top != "" {
		t += ", busiest process " + l.u.Top
	}
	if c := workCounts(background(s.Subs)); c != "" {
		t += ", running " + c + " in the background"
	}
	return t
}

func (h *Hub) loadReport() string {
	s := platform.ReadLoad()
	var b strings.Builder
	fmt.Fprintf(&b, "The machine: %s.\n", s)
	switch {
	case s.Overloaded():
		b.WriteString("It is overloaded. Do not start sessions until it recovers.\n")
	case !s.Calm():
		b.WriteString("It is busy. Start at most one session that builds or tests.\n")
	default:
		b.WriteString("It has room.\n")
	}
	ls := h.sessionLoads()
	if len(ls) == 0 {
		b.WriteString("No session has a running process.")
		return b.String()
	}
	b.WriteString("Sessions with a running process, busiest first:\n")
	for _, l := range ls {
		b.WriteString("- " + l.String() + "\n")
	}
	return strings.TrimSpace(b.String())
}

func (h *Hub) watchLoad() {
	w := &platform.LoadWatch{}
	for range time.Tick(15 * time.Second) {
		msg := w.Step(platform.ReadLoad())
		if msg == "" || !h.anyWorking() {
			continue
		}
		var heavy []string
		for i, l := range h.sessionLoads() {
			if i == 3 {
				break
			}
			heavy = append(heavy, l.String())
		}
		if len(heavy) > 0 {
			msg += " Busiest sessions: " + strings.Join(heavy, "; ") + "."
		}
		h.mu.Lock()
		h.reports = append(h.reports, msg)
		h.mu.Unlock()
		h.flushReports()
	}
}

func (h *Hub) anyWorking() bool {
	for _, a := range h.Agents() {
		if st := a.Snapshot().State; a != h.Dispatcher && (st == Working || st == Starting) {
			return true
		}
	}
	return false
}
