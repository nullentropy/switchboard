//go:build !windows

package main

func readProcs() []procRow {
	out, err := command("ps", "-A", "-o", "pid=,ppid=,%cpu=,rss=,comm=").Output()
	if err != nil {
		return nil
	}
	return parseProcs(string(out))
}
