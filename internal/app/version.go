package app

import (
	"fmt"
	"strings"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/ProjectSettings"
)

// Version is the release version: config/version in graphics/project.godot,
// the single source of truth that bin/version, the exports (bundle and file
// versions) and the packaging scripts also read. It is "dev" when the
// setting is missing.
func Version() string {
	v := strings.TrimSpace(fmt.Sprint(ProjectSettings.GetSetting("application/config/version", "")))
	if v == "" || v == "<nil>" {
		return "dev"
	}
	return v
}

// versionBanner is the startup log line, e.g. "FreeBeams 0.1.0 (Godot
// 4.7.2-stable (official))".
func versionBanner() string {
	return fmt.Sprintf("FreeBeams %s (Godot %s)", Version(), Engine.GetVersionInfo().String)
}
