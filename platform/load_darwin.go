package platform

import (
	"runtime"
	"strconv"
	"strings"
)

func sysctl(name string) string {
	out, err := Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ReadLoad() Load {
	s := Load{Cores: runtime.NumCPU()}
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
