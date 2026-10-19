//go:build darwin

package app

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// fw_set_app_name gives the application menu (the bold item next to the
// Apple menu) and its "About …", "Hide …" and "Quit …" entries the app's
// name. Under `gd run` the process is Godot's own bundle, so macOS labels
// them "Godot"; a release bundle is named right already. The menu bar reads
// the title when the main menu is installed, so it is re-installed.
static void fw_set_app_name(const char *name) {
	NSString *n = [NSString stringWithUTF8String:name];
	NSString *former = [[NSProcessInfo processInfo] processName];
	[[NSProcessInfo processInfo] setProcessName:n];
	NSMenu *main = [NSApp mainMenu];
	if (main == nil || [main numberOfItems] == 0) {
		return;
	}
	NSMenu *appMenu = [[main itemAtIndex:0] submenu];
	if (appMenu == nil) {
		return;
	}
	[appMenu setTitle:n];
	for (NSMenuItem *item in [appMenu itemArray]) {
		NSString *t = [item title];
		if (former.length > 0 && [t containsString:former]) {
			[item setTitle:[t stringByReplacingOccurrencesOfString:former withString:n]];
		} else if ([t containsString:@"Godot"]) {
			[item setTitle:[t stringByReplacingOccurrencesOfString:@"Godot" withString:n]];
		}
	}
	[NSApp setMainMenu:nil];
	[NSApp setMainMenu:main];
}

// fw_app_menu_title returns the application menu's title (for logging).
static const char *fw_app_menu_title(void) {
	NSMenu *main = [NSApp mainMenu];
	if (main == nil || [main numberOfItems] == 0) {
		return "";
	}
	NSMenu *appMenu = [[main itemAtIndex:0] submenu];
	return appMenu == nil ? "" : [[appMenu title] UTF8String];
}
*/
import "C"

import (
	"log"
	"unsafe"
)

// setAppName names the application menu after the app (see fw_set_app_name).
func setAppName(name string) {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	C.fw_set_app_name(cs)
	log.Printf("[freebeams] application menu: %s", C.GoString(C.fw_app_menu_title()))
}
