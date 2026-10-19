//go:build darwin

package app

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

// fw_trash moves the file at path to the Trash through NSFileManager (what
// the Finder does; no Automation permission, unlike telling the Finder with
// osascript). It returns 0 on success; otherwise 1 with the error
// description copied into err (n bytes).
static int fw_trash(const char *path, char *err, int n) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		NSError *e = nil;
		if ([[NSFileManager defaultManager] trashItemAtURL:url resultingItemURL:nil error:&e]) {
			return 0;
		}
		const char *msg = e != nil ? [[e localizedDescription] UTF8String] : "unknown error";
		strncpy(err, msg, n - 1);
		err[n - 1] = 0;
		return 1;
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"unsafe"
)

// trashSupported says Delete File… can move files to the Trash here.
const trashSupported = true

// errNoTrash is never returned on macOS (see trash_other.go).
var errNoTrash = errors.New("moving files to the Trash is not supported on this platform")

// trashFile moves path to the Trash. A file that is already gone is
// reported as os.ErrNotExist (wrapped).
func trashFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	const n = 512
	buf := (*C.char)(C.malloc(n))
	defer C.free(unsafe.Pointer(buf))
	if C.fw_trash(cpath, buf, n) != 0 {
		return fmt.Errorf("move %q to the Trash: %s", path, C.GoString(buf))
	}
	return nil
}
