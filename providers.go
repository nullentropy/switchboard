package main

import (
	"fmt"
	"os/exec"
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
