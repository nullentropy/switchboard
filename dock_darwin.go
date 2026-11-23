package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void setDockIcon(const void *png, int len) {
	NSData *data = [NSData dataWithBytes:png length:len];
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			NSImage *img = [[NSImage alloc] initWithData:data];
			if (NSApp != nil && img != nil) [NSApp setApplicationIconImage:img];
		}
	});
}
*/
import "C"

import "unsafe"

func setDockIcon(png []byte) { C.setDockIcon(unsafe.Pointer(&png[0]), C.int(len(png))) }
