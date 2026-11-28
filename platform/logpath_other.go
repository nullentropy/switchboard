//go:build !darwin

package platform

import (
	"os"
	"path/filepath"
)

func logPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "switchboard", "switchboard.log")
}
