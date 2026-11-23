package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var providers = []string{"claude", "codex", "gemini"}

func providerName(p string) string {
	switch p {
	case "codex":
		return "Codex"
	case "gemini":
		return "Gemini"
	}
	return "Claude"
}

func providerOf(m Meta) string {
	if m.Provider == "" {
		return "claude"
	}
	return m.Provider
}

func driverFor(p string) driver {
	switch p {
	case "codex":
		return &codexDriver{}
	case "gemini":
		return &geminiDriver{}
	}
	return claudeDriver{}
}

func (h *Hub) bin(provider string) string {
	if provider == "" {
		provider = "claude"
	}
	if b := h.bins[provider]; b != "" {
		return b
	}
	b, err := exec.LookPath(provider)
	if err != nil {
		return provider
	}
	return b
}

func checkBin(h *Hub, provider string) error {
	b := h.bin(provider)
	if _, err := exec.LookPath(b); err != nil {
		return fmt.Errorf("%s is not installed: %s is not on PATH", providerName(provider), provider)
	}
	return nil
}

const pathMark = "__switchboard_path__"

func loginPath(shell string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-ilc", `printf '`+pathMark+`%s`+pathMark+`' "$PATH"`).Output()
	if err != nil {
		return ""
	}
	parts := strings.Split(string(out), pathMark)
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-2]
}

func usePathFromLoginShell() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	if p := loginPath(shell); p != "" {
		os.Setenv("PATH", p)
	}
}
