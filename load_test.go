package main

import (
	"strings"
	"testing"
)

func TestTheDispatcherHearsOnceWhenTheLaptopIsOverloadedAndAgainWhenItRecovers(t *testing.T) {
	procs := parseProcs("  1     0   0.0   1000 /sbin/launchd\n" +
		"100     1   2.0  200000 /Users/arthur/.local/bin/claude\n" +
		"110   100 300.5 3000000 /usr/local/go/pkg/tool/darwin_arm64/compile\n" +
		"111   110  50.0  100000 /usr/local/go/pkg/tool/darwin_arm64/link\n" +
		"200     1  90.0  500000 /Applications/Chrome.app/Contents/MacOS/Chrome\n")
	u := treeUsage(procs, 100)
	if u.CPU != 352.5 || u.RSS != 3300000 || u.Top != "compile" {
		t.Fatalf("usage %+v", u)
	}

	busy := loadSample{Load1: 40, Cores: 12, MemFree: 30}
	tight := loadSample{Load1: 4, Cores: 12, MemFree: 9}
	calm := loadSample{Load1: 6, Cores: 12, MemFree: 40, Pressure: 2}
	critical := loadSample{Load1: 6, Cores: 12, MemFree: 40, Pressure: 4}
	w := &loadWatch{}
	steps := []struct {
		s    loadSample
		want string
	}{
		{busy, ""},
		{busy, "overloaded"},
		{busy, ""},
		{tight, ""},
		{calm, ""},
		{calm, "recovered"},
		{calm, ""},
		{tight, ""},
		{critical, "overloaded"},
	}
	for i, st := range steps {
		got := w.step(st.s)
		if (st.want == "") != (got == "") || !strings.Contains(got, st.want) {
			t.Fatalf("step %d: got %q, want %q", i, got, st.want)
		}
	}
}
