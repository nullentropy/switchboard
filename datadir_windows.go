package main

import (
	"errors"
	"os"
)

func dataDir() (string, error) {
	if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
		return dir, nil
	}
	return "", errors.New("LOCALAPPDATA is not set")
}
