package platform

import "golang.org/x/sys/windows"

func PrepareApp() {
	if h, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE); err == nil && h != 0 && h != windows.InvalidHandle {
		return
	}
	logToFile()
}
