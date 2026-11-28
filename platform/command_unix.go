//go:build !windows

package platform

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func Command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

func StartProcess(cmd *exec.Cmd, low bool) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	if low {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
	}
	return nil
}

func TerminateTree(p *os.Process) {
	_ = syscall.Kill(-p.Pid, syscall.SIGTERM)
}

func KillTree(p *os.Process) {
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
}
