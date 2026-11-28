package platform

import "github.com/nullentropy/caution/native"

func PrepareApp() {
	if native.InBundle() {
		logToFile()
		usePathFromLoginShell("/bin/zsh")
	}
}
