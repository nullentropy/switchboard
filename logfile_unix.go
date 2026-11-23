//go:build !windows

package main

import "golang.org/x/sys/unix"

func logToFile() string {
	p := logPath()
	if p == "" {
		return ""
	}
	f := openLog(p)
	if f == nil {
		return ""
	}
	unix.Dup2(int(f.Fd()), 1)
	unix.Dup2(int(f.Fd()), 2)
	return p
}
