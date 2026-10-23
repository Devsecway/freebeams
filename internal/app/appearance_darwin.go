//go:build darwin

package app

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// fw_set_appearance gives the whole app the Dark Aqua or the Aqua
// appearance: every window's title bar, the menus, NSOpenPanel / NSSavePanel
// and NSAlert follow NSApp.appearance unless a window sets its own.
static void fw_set_appearance(int dark) {
	NSAppearanceName name = dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua;
	[NSApp setAppearance:[NSAppearance appearanceNamed:name]];
}

// fw_effective_appearance writes the name of NSApp's effective appearance
// into buf, followed by " (window: <name>)" for the first window whose own
// effective appearance differs (a window that overrides it).
static void fw_effective_appearance(char *buf, int len) {
	NSString *app = [[NSApp effectiveAppearance] name];
	NSString *out = app ?: @"";
	for (NSWindow *w in [NSApp windows]) {
		NSString *n = [[w effectiveAppearance] name];
		if (n != nil && app != nil && ![n isEqualToString:app]) {
			out = [NSString stringWithFormat:@"%@ (window: %@)", app, n];
			break;
		}
	}
	[out getCString:buf maxLength:len encoding:NSUTF8StringEncoding];
}
*/
import "C"

import (
	"log"
	"unsafe"
)

// nativeLook is the appearance last given to NSApp (main thread only).
type nativeLook struct {
	set, dark bool
}

// apply sets NSApp.appearance when the colour changed.
func (l *nativeLook) apply(dark bool) {
	if l.set && l.dark == dark {
		return
	}
	l.set, l.dark = true, dark
	flag := C.int(0)
	if dark {
		flag = 1
	}
	C.fw_set_appearance(flag)
	log.Printf("[freebeams] native appearance: %s", l.state())
}

// state names the effective appearance (read back from AppKit).
func (l *nativeLook) state() string {
	if !l.set {
		return ""
	}
	var buf [256]C.char
	C.fw_effective_appearance(&buf[0], C.int(len(buf)))
	return C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
}
