package platform

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

func Command(name string, args ...string) *exec.Cmd {
	return noWindow(exec.Command(name, args...))
}

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return noWindow(exec.CommandContext(ctx, name, args...))
}

func StartProcess(cmd *exec.Cmd, low bool) error {
	if low {
		if cmd.SysProcAttr == nil {
			noWindow(cmd)
		}
		cmd.SysProcAttr.CreationFlags |= windows.BELOW_NORMAL_PRIORITY_CLASS
	}
	return cmd.Start()
}

func TerminateTree(p *os.Process) {}

func KillTree(p *os.Process) {
	if Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run() != nil {
		_ = p.Kill()
	}
}
