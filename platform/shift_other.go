//go:build !darwin && !windows

package platform

func ShiftHeld() bool { return false }
