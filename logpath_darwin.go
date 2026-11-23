package main

import (
	"os"
	"path/filepath"
)

func logPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Logs", "Switchboard.log")
}
