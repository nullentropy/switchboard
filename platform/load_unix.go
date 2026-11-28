//go:build !windows

package platform

func ReadProcs() []Proc {
	out, err := Command("ps", "-A", "-o", "pid=,ppid=,%cpu=,rss=,comm=").Output()
	if err != nil {
		return nil
	}
	return parseProcs(string(out))
}
