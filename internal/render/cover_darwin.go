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
