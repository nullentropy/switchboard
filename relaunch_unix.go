//go:build !windows

package main

import (
	"os"
	"syscall"
)

func relaunch(exe string) error {
	return syscall.Exec(exe, append([]string{exe}, os.Args[1:]...), os.Environ())
}
