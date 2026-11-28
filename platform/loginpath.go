package platform

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

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

func usePathFromLoginShell(fallback string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = fallback
	}
	if p := loginPath(shell); p != "" {
		os.Setenv("PATH", p)
	}
}
