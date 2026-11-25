package main

import (
	"os"
	"os/exec"
)

func relaunch(exe string) error {
	if err := exec.Command(exe, os.Args[1:]...).Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
