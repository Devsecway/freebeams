//go:build windows

package app

import (
	"log"
	"syscall"
	"unsafe"

	"graphics.gd/classdb/DisplayServer"
)

// On Windows the title bar of every top-level window is dark or light per
// window: DwmSetWindowAttribute(DWMWA_USE_IMMERSIVE_DARK_MODE). Godot sets
// it from the system's setting when it creates a window and again when the
// system theme changes (WM_SETTINGCHANGE), so the app re-applies its own
// colour to every window it has not seen yet, and to all of them every
// reapplyFrames frames. Plain syscall (no cgo), so it cross-compiles.

var (
	dwmapi                 = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttrib = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	dwmwaUseImmersiveDarkMode       = 20 // Windows 10 20H1 and later
	dwmwaUseImmersiveDarkModeBefore = 19 // Windows 10 1809 – 1909
	reapplyFrames                   = 120
)

// nativeLook is what the windows were last given (main thread only).
type nativeLook struct {
	set, dark bool
	windows   map[int32]bool // window ids given the current colour
	frames    int
	failed    bool // DwmSetWindowAttribute is unavailable (logged once)
}

// apply gives every window the colour: all of them when it changed or
// every reapplyFrames frames, else only windows that appeared since.
func (l *nativeLook) apply(dark bool) {
	l.frames++
	all := !l.set || l.dark != dark || l.frames%reapplyFrames == 0
	if all {
		l.set, l.dark = true, dark
		l.windows = map[int32]bool{}
	}
	for _, id := range DisplayServer.GetWindowList() {
		if l.windows[id] {
			continue
		}
		l.windows[id] = true
		hwnd := uintptr(DisplayServer.WindowGetNativeHandle(DisplayServer.WindowHandle, DisplayServer.Window(id)))
		if hwnd == 0 {
			delete(l.windows, id) // not created yet: next frame
			continue
		}
		if err := setDarkTitleBar(hwnd, dark); err != nil && !l.failed {
			l.failed = true
			log.Printf("[freebeams] native appearance: %v", err)
		}
	}
}

// state is "dark" or "light" once applied.
func (l *nativeLook) state() string {
	switch {
	case !l.set:
		return ""
	case l.dark:
		return "dark"
	}
	return "light"
}

// setDarkTitleBar sets DWMWA_USE_IMMERSIVE_DARK_MODE on a window (the
// attribute's older number before Windows 10 20H1).
func setDarkTitleBar(hwnd uintptr, dark bool) error {
	if err := procDwmSetWindowAttrib.Find(); err != nil {
		return err
	}
	var v int32
	if dark {
		v = 1
	}
	for _, attr := range []uintptr{dwmwaUseImmersiveDarkMode, dwmwaUseImmersiveDarkModeBefore} {
		hr, _, _ := procDwmSetWindowAttrib.Call(hwnd, attr, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
		if hr == 0 { // S_OK
			return nil
		}
	}
	return syscall.Errno(0x80070057) // E_INVALIDARG: neither attribute is known
}
