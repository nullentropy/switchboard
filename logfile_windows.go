package main

import (
	"log"
	"os"

	"golang.org/x/sys/windows"
)

func logToFile() string {
	p := logPath()
	if p == "" {
		return ""
	}
	f := openLog(p)
	if f == nil {
		return ""
	}
	windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
	windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	os.Stdout, os.Stderr = f, f
	log.SetOutput(f)
	return p
}
