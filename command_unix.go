//go:build !windows

package main

import (
	"context"
	"os/exec"
	"syscall"
)

func command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func commandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func startProcess(cmd *exec.Cmd, low bool) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	if low {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
	}
	return nil
}
