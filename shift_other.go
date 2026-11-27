//go:build !darwin && !windows

package main

func shiftHeld() bool { return false }
