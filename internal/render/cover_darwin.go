//go:build darwin

package render

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// fw_cover_screen makes the borderless NSWindow cover its whole screen,
// including the menu bar and status items ("Displays have separate Spaces",
// the macOS default, gives every display a menu bar that otherwise sits over
// the top of the output and slides back in when the mouse touches the edge).
// It returns 0 on success, 1 when the handle is not a window.
static int fw_cover_screen(uintptr_t handle) {
	id obj = (__bridge id)(void *)handle;
	if (obj == nil || ![obj isKindOfClass:[NSWindow class]]) {
		return 1;
	}
	NSWindow *win = (NSWindow *)obj;
	// Above the menu bar (NSMainMenuWindowLevel) and its status items
	// (NSStatusWindowLevel), below popup menus and the screen saver.
	[win setLevel:NSStatusWindowLevel + 1];
	// Godot marks a borderless window that covers its screen hidesOnDeactivate
	// (_update_window_style), which empties the projector whenever the
	// operator switches to another app. A projector stays up regardless of
	// which app is active, and survives Hide (Cmd+H) too.
	[win setHidesOnDeactivate:NO];
	[win setCanHide:NO];
	// Stay on this display through Space switches and never become a
	// fullscreen Space of its own (no Spaces animation, no menu bar reveal).
	[win setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces |
	                           NSWindowCollectionBehaviorStationary |
	                           NSWindowCollectionBehaviorFullScreenAuxiliary |
	                           NSWindowCollectionBehaviorIgnoresCycle];
	[win setHasShadow:NO];
	NSScreen *screen = [win screen];
	if (screen != nil) {
		[win setFrame:[screen frame] display:YES];
	}
	return 0;
}

// fw_window_state reports what the OS says about the NSWindow: AppKit's
// visibility, its level, whether macOS hides it while the application is
// inactive, whether the app is active, whether the window is on the active
// Space and unoccluded, and whether the window server lists it on screen (what
// a screenshot would show). It returns 0 on success, 1 when the handle is not
// a window.
static int fw_window_state(uintptr_t handle, int *visible, long *level, int *hides,
                           int *active, int *onSpace, int *shown, int *onScreen) {
	id obj = (__bridge id)(void *)handle;
	if (obj == nil || ![obj isKindOfClass:[NSWindow class]]) {
		return 1;
	}
	NSWindow *win = (NSWindow *)obj;
	*visible = [win isVisible] ? 1 : 0;
	*level = (long)[win level];
	*hides = [win hidesOnDeactivate] ? 1 : 0;
	*active = [NSApp isActive] ? 1 : 0;
	*onSpace = [win isOnActiveSpace] ? 1 : 0;
	*shown = ([win occlusionState] & NSWindowOcclusionStateVisible) ? 1 : 0;
	*onScreen = 0;
	CGWindowID num = (CGWindowID)[win windowNumber];
	CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly, kCGNullWindowID);
	if (list != NULL) {
		for (CFIndex i = 0; i < CFArrayGetCount(list); i++) {
			CFDictionaryRef info = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
			CFNumberRef n = (CFNumberRef)CFDictionaryGetValue(info, kCGWindowNumber);
			int64_t id = 0;
			if (n != NULL && CFNumberGetValue(n, kCFNumberSInt64Type, &id) && (CGWindowID)id == num) {
				*onScreen = 1;
				break;
			}
		}
		CFRelease(list);
	}
	return 0;
}
*/
import "C"

// coverSupported reports that coverScreen can raise a window above the menu
// bar on this platform.
const coverSupported = true

// coverScreen raises the native window above the menu bar and stretches it
// over its whole screen. handle is DisplayServer's WindowHandle (an NSWindow).
func coverScreen(handle uintptr) bool {
	return handle != 0 && C.fw_cover_screen(C.uintptr_t(handle)) == 0
}

// coverState reads the native window's visibility, level and
// hides-on-deactivate flag (false when handle is not an NSWindow).
func coverState(handle uintptr) (NativeWindowState, bool) {
	if handle == 0 {
		return NativeWindowState{}, false
	}
	var (
		visible, hides, active, onSpace, shown, onScreen C.int
		level                                            C.long
	)
	if C.fw_window_state(C.uintptr_t(handle), &visible, &level, &hides, &active, &onSpace, &shown, &onScreen) != 0 {
		return NativeWindowState{}, false
	}
	return NativeWindowState{
		Visible: visible != 0, Level: int(level), HidesOnDeactivate: hides != 0,
		AppActive: active != 0, OnActiveSpace: onSpace != 0, Shown: shown != 0, OnScreen: onScreen != 0,
	}, true
}
