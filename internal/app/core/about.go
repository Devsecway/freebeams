package core

// Copyright is the copyright line of the app (LICENSE, the About box, the
// installers and the macOS bundle).
const Copyright = "Copyright © 2026 Devsecway"

// AboutText is the Help ▸ About message: version is the release version
// (config/version), build the Go module version of a VCS build ("" when not
// known).
func AboutText(version, build string) string {
	v := "FreeBeams " + version
	if build != "" && build != version && build != "v"+version {
		v += " (" + build + ")"
	}
	// The message dialog is a fixed 480 × 210 window: four short lines fit.
	return v + "\nFree worship presentation software, built with Go and Godot.\n\n" +
		"MIT License. " + Copyright + ".\n" +
		"Godot, FFmpeg and the other parts it includes are listed in THIRD-PARTY-NOTICES.txt beside the app."
}
