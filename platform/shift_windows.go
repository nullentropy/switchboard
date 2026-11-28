package platform

import "golang.org/x/sys/windows"

var procGetAsyncKeyState = windows.NewLazySystemDLL("user32.dll").NewProc("GetAsyncKeyState")

func ShiftHeld() bool {
	const vkShift = 0x10
	r, _, _ := procGetAsyncKeyState.Call(vkShift)
	return r&0x8000 != 0
}
