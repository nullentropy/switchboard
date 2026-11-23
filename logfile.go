package main

import (
	"os"
	"path/filepath"
)

func openLog(p string) *os.File {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil
	}
	if st, err := os.Stat(p); err == nil && st.Size() > 8<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil
	}
	return f
}
