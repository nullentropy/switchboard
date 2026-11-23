package main

import "testing"

func TestLinuxLoadComesFromProc(t *testing.T) {
	s := parseProcLoad(8, "17.42 12.00 6.50 3/1024 4242\n",
		"MemTotal:       16000000 kB\nMemFree:          500000 kB\nMemAvailable:    1600000 kB\nSwapTotal:       2097152 kB\nSwapFree:        1048576 kB\n")
	if s.Load1 != 17.42 || s.Load5 != 12 || s.Load15 != 6.5 || s.Cores != 8 {
		t.Fatalf("load %+v", s)
	}
	if s.MemFree != 10 || s.Swap != "1024.00M" {
		t.Fatalf("memory %+v", s)
	}
	if !s.overloaded() {
		t.Fatalf("load 17 on 8 cores with 10%% free should be overloaded: %s", s)
	}
}
