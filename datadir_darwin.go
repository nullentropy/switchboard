package main

import "os"

func dataDir() (string, error) {
	return os.UserConfigDir()
}
