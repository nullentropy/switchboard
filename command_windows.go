package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

func noWindow(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

func command(name string, args ...string) *exec.Cmd {
	return noWindow(exec.Command(name, args...))
}

func commandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return noWindow(exec.CommandContext(ctx, name, args...))
}

func startProcess(cmd *exec.Cmd, low bool) error {
	if low {
		if cmd.SysProcAttr == nil {
			noWindow(cmd)
		}
		cmd.SysProcAttr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
	}
	return cmd.Start()
}

func terminateTree(p *os.Process) {}

func killTree(p *os.Process) {
	if command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run() != nil {
		_ = p.Kill()
	}
}
