//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func alive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil && strings.HasSuffix(string(b), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err == nil {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s was never written", path)
	return 0
}

func TestQuittingMidTurnEndsTheAgentAndTheCommandsItStartedPromptly(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ntrap '' TERM\nsleep 600 &\necho $! > " + dir + "/tool\necho $$ > " + dir + "/agent\ncat > /dev/null\nwait\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	h := newHub(map[string]string{"claude": bin}, "")
	a, err := h.New(Meta{Cwd: dir, Mode: "default"}, false)
	if err != nil {
		t.Fatal(err)
	}
	go a.Send("compute the question to the ultimate answer")
	agent, tool := readPid(t, filepath.Join(dir, "agent")), readPid(t, filepath.Join(dir, "tool"))
	t.Cleanup(func() {
		_ = syscall.Kill(agent, syscall.SIGKILL)
		_ = syscall.Kill(tool, syscall.SIGKILL)
	})

	start := time.Now()
	h.StopAll()
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("StopAll took %s", took)
	}
	if alive(agent) {
		t.Fatalf("the agent process %d is still running after StopAll returned", agent)
	}
	deadline := time.Now().Add(time.Second)
	for alive(tool) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if alive(tool) {
		t.Fatalf("the agent's command %d is still running after quit", tool)
	}
}
