package platform

import (
	"errors"
	"os"
	"path/filepath"
)

func DataDir() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", errors.New("no home directory")
	}
	return filepath.Join(home, ".local", "share"), nil
}
