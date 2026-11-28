package platform

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type Proc struct {
	pid, ppid int
	cpu       float64
	rss       int64
	comm      string
}

func parseProcs(out string) []Proc {
	var rows []Proc
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
		rows = append(rows, Proc{pid, ppid, cpu, rss, filepath.Base(strings.Trim(strings.Join(f[4:], " "), "()"))})
	}
	return rows
}

type Usage struct {
	CPU float64
	RSS int64
	Top string
}

func TreeUsage(procs []Proc, root int) Usage {
	kids := map[int][]Proc{}
	var self *Proc
	for i, p := range procs {
		kids[p.ppid] = append(kids[p.ppid], p)
		if p.pid == root {
			self = &procs[i]
		}
	}
	var u Usage
	if self == nil {
		return u
	}
	var topCPU float64 = -1
	var walk func(p Proc, depth int)
	walk = func(p Proc, depth int) {
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

type Load struct {
	Load1, Load5, Load15 float64
	Cores                int
	MemFree              int
	Pressure             int
	Swap                 string
	ByCPU                bool
	CPUBusy              float64
}

func (s Load) cpuOverloaded() bool {
	if s.ByCPU {
		return s.CPUBusy >= 95
	}
	return s.Load1 > 2*float64(s.Cores)
}

func (s Load) cpuCalm() bool {
	if s.ByCPU {
		return s.CPUBusy < 80
	}
	return s.Load1 <= 1.25*float64(s.Cores)
}

func (s Load) Overloaded() bool {
	return s.cpuOverloaded() || s.Pressure >= 4 || (s.MemFree > 0 && s.MemFree < 12)
}

func (s Load) Calm() bool {
	return s.cpuCalm() && s.Pressure < 4 && (s.MemFree == 0 || s.MemFree >= 20)
}

func (s Load) String() string {
	t := fmt.Sprintf("load %.0f now, %.0f over 5 minutes, on %d cores", s.Load1, s.Load5, s.Cores)
	if s.ByCPU {
		t = fmt.Sprintf("CPU %.0f%% busy on %d cores", s.CPUBusy, s.Cores)
	}
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

type LoadWatch struct {
	over, calm int
	alerted    bool
}

func (w *LoadWatch) Step(s Load) string {
	switch {
	case s.Overloaded():
		w.over, w.calm = w.over+1, 0
	case s.Calm():
		w.over, w.calm = 0, w.calm+1
	default:
		w.over, w.calm = 0, 0
	}
	if !w.alerted && w.over >= 2 {
		w.alerted = true
		return "The machine is overloaded: " + s.String() + "."
	}
	if w.alerted && w.calm >= 2 {
		w.alerted = false
		return "The machine has recovered: " + s.String() + "."
	}
	return ""
}
