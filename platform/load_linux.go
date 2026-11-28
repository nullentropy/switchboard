package platform

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func ReadLoad() Load {
	loadavg, _ := os.ReadFile("/proc/loadavg")
	meminfo, _ := os.ReadFile("/proc/meminfo")
	return parseProcLoad(runtime.NumCPU(), string(loadavg), string(meminfo))
}

func parseProcLoad(cores int, loadavg, meminfo string) Load {
	s := Load{Cores: cores}
	if f := strings.Fields(loadavg); len(f) >= 3 {
		s.Load1, _ = strconv.ParseFloat(f[0], 64)
		s.Load5, _ = strconv.ParseFloat(f[1], 64)
		s.Load15, _ = strconv.ParseFloat(f[2], 64)
	}
	kb := map[string]int64{}
	for _, line := range strings.Split(meminfo, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if f := strings.Fields(rest); len(f) > 0 {
			kb[name], _ = strconv.ParseInt(f[0], 10, 64)
		}
	}
	if total := kb["MemTotal"]; total > 0 {
		s.MemFree = int(kb["MemAvailable"] * 100 / total)
	}
	if kb["SwapTotal"] > 0 {
		s.Swap = fmt.Sprintf("%.2fM", float64(kb["SwapTotal"]-kb["SwapFree"])/1024)
	}
	return s
}
