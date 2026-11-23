package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func prepareApp() {
	if _, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS); err == nil {
		return
	}
	logToFile()
	usePathFromLoginShell("/bin/sh")
}
