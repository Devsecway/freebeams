package render

// Holding a monitor's file open (docs/MEDIA.md "Black, Clear and Logo").
//
// A layer releases its source when it is hidden, so a Black longer than the
// crossfade used to close the live player and the clip came back from 0.
// The app now holds the file its playback state is about (Hold), whatever
// the layers do: the player stays open under Black and Logo, keeps its
// position and its sound, and the layer that shows the clip again shares it.
//
// This file only adds to the shared player layer (videosource.go).

// RoleResource is the resource preview's own player (its mini transport):
// a clip is previewed there without touching the Preview or Live players.
const RoleResource VideoRole = RoleAudio + 1

// holds is the source the app holds open per role.
var holds = map[VideoRole]*MediaSource{}

// Hold keeps (role, path) open until the next Hold for that role with
// another path ("" releases it). It is idempotent, so the app calls it from
// its render pass. Nothing is held where no media player plays the file
// (the Theora fallback).
func Hold(role VideoRole, path string) {
	h := holds[role]
	if h != nil && h.refs > 0 && h.path == path {
		return
	}
	delete(holds, role)
	if path != "" && NativeVideoSupported() {
		holds[role] = acquireSource(role, path)
	}
	if h != nil && h.refs > 0 {
		h.release()
	}
}

// Source returns the open source for (role, path), or nil: the file a
// monitor's playback state is about, even while another one (a Logo video)
// is the role's Current.
func Source(role VideoRole, path string) *MediaSource {
	if path == "" {
		return nil
	}
	return sources[sourceKey{role, path}]
}

// releaseHolds forgets the holds (CloseSources closes their sources).
func releaseHolds() { clear(holds) }
