package render

import "graphics.gd/classdb/OS"

// Low-processor mode keeps the operator UI cheap at idle but throttles
// redraws, so it is switched off while anything animates. The switch is
// refcounted: concurrent transitions (preview monitor + live monitor + output)
// and other animators (video, Phase 2) share it without fighting.

var (
	animating     int
	savedLowProcs bool
)

// AcquireAnimating marks one animation as running. While at least one is
// running, low-processor mode is off. Pair every call with ReleaseAnimating.
// Main thread only.
func AcquireAnimating() {
	if animating == 0 {
		savedLowProcs = OS.LowProcessorUsageMode()
		OS.SetLowProcessorUsageMode(false)
	}
	animating++
}

// ReleaseAnimating ends one animation started with AcquireAnimating. When the
// last one ends, low-processor mode is restored to its previous setting.
func ReleaseAnimating() {
	if animating == 0 {
		return
	}
	animating--
	if animating == 0 {
		OS.SetLowProcessorUsageMode(savedLowProcs)
	}
}

// Animating reports how many animations hold low-processor mode off.
func Animating() int { return animating }
