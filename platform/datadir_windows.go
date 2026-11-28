package platform

import (
	"errors"
	"os"
)

func DataDir() (string, error) {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return dir, nil
	}
	return "", errors.New("LOCALAPPDATA is not set")
}
