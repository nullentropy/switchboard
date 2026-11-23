package main

import "github.com/nullentropy/caution/native"

func prepareApp() {
	if native.InBundle() {
		logToFile()
		usePathFromLoginShell("/bin/zsh")
	}
}
