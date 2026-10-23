package app

import "freebeams/internal/app/core"

// Native window chrome follows the interface colour: the app draws its own
// UI from ui.DarkPalette / LightPalette, but the title bars, the native
// menus, the Open/Save panels and alerts are drawn by the OS, which would
// otherwise follow the system's Light / Dark setting. renderNativeAppearance
// projects settings.InterfaceColor onto them every frame (a no-op unless it
// changed, or on Windows unless a window appeared): NSApp.appearance on
// macOS (appearance_darwin.go), DWMWA_USE_IMMERSIVE_DARK_MODE on every
// window on Windows (appearance_windows.go), nothing elsewhere.

// renderNativeAppearance applies the interface colour to the OS chrome. It
// writes only a.look, the projection's cache.
func (a *App) renderNativeAppearance() {
	a.look.apply(a.settings.InterfaceColor != core.ColorLight)
}

// nativeAppearance is /state.nativeAppearance (core.State).
func (a *App) nativeAppearance() string { return a.look.state() }
