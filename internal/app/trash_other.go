//go:build !darwin

package app

import "errors"

// trashSupported says Delete File… can move files to the Trash here: only
// macOS has a reliable way (trash_darwin.go), so the menus leave the item
// out elsewhere and Remove from Library is what remains.
const trashSupported = false

var errNoTrash = errors.New("moving files to the Trash is not supported on this platform")

// trashFile is not available on this platform.
func trashFile(string) error { return errNoTrash }
