package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int shiftHeld(void) {
	@autoreleasepool {
		return ([NSEvent modifierFlags] & NSEventModifierFlagShift) ? 1 : 0;
	}
}
*/
import "C"

func shiftHeld() bool { return C.shiftHeld() != 0 }
