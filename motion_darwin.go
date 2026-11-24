package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int reduceMotion(void) {
	@autoreleasepool {
		return [[NSWorkspace sharedWorkspace] accessibilityDisplayShouldReduceMotion] ? 1 : 0;
	}
}
*/
import "C"

func osReduceMotion() bool { return C.reduceMotion() != 0 }
