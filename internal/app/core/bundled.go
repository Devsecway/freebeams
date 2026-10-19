package core

import (
	"strings"

	"freebeams/internal/bibles/bundled"
)

// BundledState is the status of the bundled translations' installation
// (internal/bibles/bundled), reported as /state.bundled.
type BundledState struct {
	Running   bool     `json:"running"`
	Done      bool     `json:"done"` // the library has the current bundle version
	Installed []string `json:"installed"`
	Skipped   []string `json:"skipped"`
	Version   int      `json:"version"`
	Error     string   `json:"error,omitempty"`
}

// BundledSummary is the Scriptures footer text after the bundled
// translations were installed ("" when nothing was added, so the footer is
// left alone).
func BundledSummary(r bundled.Report) string {
	if len(r.Installed) == 0 {
		return ""
	}
	return "Installed " + strings.Join(r.Installed, ", ")
}
