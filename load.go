package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type procRow struct {
	pid, ppid int
	cpu       float64
	rss       int64
	comm      string
}

func parseProcs(out string) []procRow {
	var rows []procRow
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		cpu, err3 := strconv.ParseFloat(f[2], 64)
		rss, err4 := strconv.ParseInt(f[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		rows = append(rows, procRow{pid, ppid, cpu, rss, filepath.Base(strings.Trim(strings.Join(f[4:], " "), "()"))})
	}
	return rows
}

func readProcs() []procRow {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,%cpu=,rss=,comm=").Output()
	if err != nil {
		return nil
	}
	return parseProcs(string(out))
}

type usage struct {
	CPU float64
	RSS int64
	Top string
}

func treeUsage(procs []procRow, root int) usage {
	kids := map[int][]procRow{}
	var self *procRow
	for i, p := range procs {
		kids[p.ppid] = append(kids[p.ppid], p)
		if p.pid == root {
			self = &procs[i]
		}
	}
	var u usage
	if self == nil {
		return u
	}
	var topCPU float64 = -1
	var walk func(p procRow, depth int)
	walk = func(p procRow, depth int) {
		u.CPU += p.cpu
		u.RSS += p.rss
		if depth > 0 && p.cpu > topCPU {
			topCPU, u.Top = p.cpu, p.comm
		}
		for _, k := range kids[p.pid] {
			walk(k, depth+1)
		}
	}
	walk(*self, 0)
	return u
}

type loadSample struct {
	Load1, Load5, Load15 float64
	Cores                int
	MemFree              int
	Pressure             int
	Swap                 string
}

func (s loadSample) overloaded() bool {
	return s.Load1 > 2*float64(s.Cores) || s.Pressure >= 4 || (s.MemFree > 0 && s.MemFree < 12)
}

func (s loadSample) calm() bool {
	return s.Load1 <= 1.25*float64(s.Cores) && s.Pressure < 4 && (s.MemFree == 0 || s.MemFree >= 20)
}

func (s loadSample) String() string {
	t := fmt.Sprintf("load %.0f now, %.0f over 5 minutes, on %d cores", s.Load1, s.Load5, s.Cores)
	if s.MemFree > 0 {
		t += fmt.Sprintf(", %d%% of memory free", s.MemFree)
	}
	switch {
	case s.Pressure >= 4:
		t += ", memory pressure critical"
	case s.Pressure == 2:
		t += ", memory pressure warning"
	}
	if s.Swap != "" {
		t += ", " + s.Swap + " of swap used"
	}
	return t
}

func sysctl(name string) string {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readLoad() loadSample {
	s := loadSample{Cores: runtime.NumCPU()}
	f := strings.Fields(strings.Trim(sysctl("vm.loadavg"), "{} "))
	if len(f) >= 3 {
		s.Load1, _ = strconv.ParseFloat(f[0], 64)
		s.Load5, _ = strconv.ParseFloat(f[1], 64)
		s.Load15, _ = strconv.ParseFloat(f[2], 64)
	}
	s.MemFree, _ = strconv.Atoi(sysctl("kern.memorystatus_level"))
	s.Pressure, _ = strconv.Atoi(sysctl("kern.memorystatus_vm_pressure_level"))
	if sw := strings.Fields(sysctl("vm.swapusage")); len(sw) >= 6 && sw[3] == "used" {
		s.Swap = strings.TrimPrefix(sw[5], "=")
	}
	return s
}

type loadWatch struct {
	over, calm int
	alerted    bool
}

func (w *loadWatch) step(s loadSample) string {
	switch {
	case s.overloaded():
		w.over, w.calm = w.over+1, 0
	case s.calm():
		w.over, w.calm = 0, w.calm+1
	default:
		w.over, w.calm = 0, 0
	}
	if !w.alerted && w.over >= 2 {
		w.alerted = true
		return "The laptop is overloaded: " + s.String() + "."
	}
	if w.alerted && w.calm >= 2 {
		w.alerted = false
		return "The laptop has recovered: " + s.String() + "."
	}
	return ""
}

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
	u usage
}

func (h *Hub) sessionLoads() []sessionLoad {
	procs := readProcs()
	var out []sessionLoad
	for _, a := range h.Agents() {
		if pid := a.pid(); pid != 0 {
			if u := treeUsage(procs, pid); u.RSS > 0 {
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
	return t
}

func (h *Hub) loadReport() string {
	s := readLoad()
	var b strings.Builder
	fmt.Fprintf(&b, "The laptop: %s.\n", s)
	switch {
	case s.overloaded():
		b.WriteString("It is overloaded. Do not start sessions until it recovers.\n")
	case !s.calm():
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
	w := &loadWatch{}
	for range time.Tick(15 * time.Second) {
		msg := w.step(readLoad())
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
