//go:build !darwin

package platform

func ReduceMotion() bool { return false }
