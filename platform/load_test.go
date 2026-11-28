package platform

import (
	"strings"
	"testing"
)

func TestTheDispatcherHearsOnceWhenTheMachineIsOverloadedAndAgainWhenItRecovers(t *testing.T) {
	procs := parseProcs("  1     0   0.0   1000 /sbin/launchd\n" +
		"100     1   2.0  200000 /Users/arthur/.local/bin/claude\n" +
		"110   100 300.5 3000000 /usr/local/go/pkg/tool/darwin_arm64/compile\n" +
		"111   110  50.0  100000 /usr/local/go/pkg/tool/darwin_arm64/link\n" +
		"200     1  90.0  500000 /Applications/Chrome.app/Contents/MacOS/Chrome\n")
	u := TreeUsage(procs, 100)
	if u.CPU != 352.5 || u.RSS != 3300000 || u.Top != "compile" {
		t.Fatalf("usage %+v", u)
	}

	busy := Load{Load1: 40, Cores: 12, MemFree: 30}
	tight := Load{Load1: 4, Cores: 12, MemFree: 9}
	calm := Load{Load1: 6, Cores: 12, MemFree: 40, Pressure: 2}
	critical := Load{Load1: 6, Cores: 12, MemFree: 40, Pressure: 4}
	w := &LoadWatch{}
	steps := []struct {
		s    Load
		want string
	}{
		{busy, ""},
		{busy, "The machine is overloaded"},
		{busy, ""},
		{tight, ""},
		{calm, ""},
		{calm, "The machine has recovered"},
		{calm, ""},
		{tight, ""},
		{critical, "The machine is overloaded"},
	}
	for i, st := range steps {
		got := w.Step(st.s)
		if (st.want == "") != (got == "") || !strings.Contains(got, st.want) {
			t.Fatalf("step %d: got %q, want %q", i, got, st.want)
		}
	}
}

func TestWindowsJudgesOverloadByHowBusyTheCPUIs(t *testing.T) {
	busy := Load{Cores: 16, MemFree: 40, ByCPU: true, CPUBusy: 97}
	middling := Load{Cores: 16, MemFree: 40, ByCPU: true, CPUBusy: 85}
	calm := Load{Cores: 16, MemFree: 40, ByCPU: true, CPUBusy: 42}
	if !busy.Overloaded() || middling.Overloaded() || calm.Overloaded() {
		t.Fatalf("overloaded: busy %v, middling %v, calm %v", busy.Overloaded(), middling.Overloaded(), calm.Overloaded())
	}
	if busy.Calm() || middling.Calm() || !calm.Calm() {
		t.Fatalf("calm: busy %v, middling %v, calm %v", busy.Calm(), middling.Calm(), calm.Calm())
	}
	if got := busy.String(); !strings.HasPrefix(got, "CPU 97% busy on 16 cores") {
		t.Fatalf("report %q", got)
	}
}
