package platform

import (
	"os"
	"os/exec"
)

var exitProcess = os.Exit

func Relaunch(exe string) error {
	if err := exec.Command(exe, os.Args[1:]...).Start(); err != nil {
		return err
	}
	exitProcess(0)
	return nil
}
