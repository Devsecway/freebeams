# Media playback

How videos and audio files in the Media tab play, per platform, and what the operator gets.

## What plays where

| Platform | Videos | Audio | Needs |
|---|---|---|---|
| macOS | every format AVFoundation decodes (H.264 / HEVC in `.mp4` `.mov` `.m4v`, and `.webm` / `.ogv` only where the OS has a decoder) | `.mp3` `.m4a` `.aac` `.wav` `.aiff` `.aif` `.flac` (`.ogg` only with an OS decoder) | nothing: the source file plays as it is (`core.MediaCaps.Native`, `/state.media.native`) |
| Windows, Linux | Ogg Theora (`.ogv`) through Godot's `VideoStreamPlayer`; other containers after an ffmpeg + libtheora transcode (`internal/media`) | listed, not played yet | ffmpeg with `libtheora` for the transcode |

ffmpeg / ffprobe are still used everywhere, when present, for metadata (`Describe`: size and duration) and poster frames
(`Thumbnail`); without them videos show a plain tile and play all the same on macOS. The native path never
transcodes (`media.Processor.Native`, set by the app from `render.NativeVideoSupported()`), so the "Needs libtheora"
badge and the "videos cannot play" footer note are gone on macOS.

Audio goes to the system's default output device at the player's volume (the transport's volume slider; it is not
routed through Godot's audio bus). A video with a sound track plays its sound too.

## macOS: AVFoundation (`internal/render/avplayer_darwin.go`)

One `AVPlayer` + `AVPlayerItem` + `AVPlayerItemVideoOutput` (BGRA) per open file, wrapped in an Objective-C object in the
cgo preamble (like `cover_darwin.go`), linked against AVFoundation, CoreMedia, CoreVideo, Accelerate and QuartzCore.
The Go side (`nativePlayer` interface) calls it on the main thread only:

- `open` → `play` / `pause` / `seek` / `setVolume` / `setLoop` / `close`;
- `status` (loading / ready / failed), `position`, `duration`, `tracks` (has video / has audio, presentation size),
  `playing` (rate ≠ 0), `ended` (a non-looping end, reported once);
- `copyFrame`: `itemTimeForHostTime:` → `hasNewPixelBufferForItemTime:` → `copyPixelBufferForItemTime:` → lock →
  `vImagePermuteChannels_ARGB8888` BGRA→RGBA straight into a Go `[]byte` (row padding handled through `rowBytes`) →
  unlock, release. A frame larger than the buffer is kept for the next call with a larger buffer.

Looping is gapless: `actionAtItemEnd = None` and the `AVPlayerItemDidPlayToEndTime` observer (main queue, which Godot's
`NSApp` run loop pumps between frames) seeks to zero without touching the rate when the loop flag is on; otherwise it
pauses and sets the ended flag.

`suppressesPlayerRendering = YES`: frames go to our texture only; `automaticallyWaitsToMinimizeStalling = NO`: local
files start at once.

## Shared sources (`internal/render/videosource.go`)

A `MediaSource` is keyed by `(role, path)` and refcounted:

- `RoleLive`: the Live Output monitor **and** the projector window (`render.Output` gives its view `RoleLive`), so one
  decoder feeds both and the sound plays once;
- `RolePreview`: the Preview Output monitor, its own decoder, paused at 0 by default;
- `RoleNone` (thumbnails, the resource preview): no player, the poster still only.

A source owns the native player, one RGBA8 `Image` and the `ImageTexture` every attached layer draws
(`ImageTexture.Update` when the size is unchanged, `SetImage` otherwise). Views with a role keep `Process` on while they
hold a source and call `pumpSources()` once per engine frame (`Engine.GetProcessFrames()` guard): status transitions,
the tracks once ready, pending play / seek, a new frame if there is one. `AcquireAnimating` is held while a source
plays (low-processor mode off), released when it pauses, ends, fails or closes.

Layers (`layer.native`, a `TextureRect` between the poster and the text, `StretchKeepAspectCovered`) acquire their
source when they have a path, a role and are visible in the tree, and release it when hidden: the outgoing side of a
finished crossfade keeps its last frame on its (now hidden) texture rect and drops the player. A quick Black and back
(inside the 500 ms fade) therefore keeps the decoder; a longer one re-opens the file from 0.

`render.Current(role)` is the source most recently opened for a role (what the monitor shows now).
`render.CloseSources()` runs from `ReleaseCaches()` at shutdown.

## Playback state (`internal/app/playback.go`)

State-machine rule: what the operator wants of each monitor is app state, `core.Playback{Key, Playing, Loop, Volume,
SeekSeq, SeekTo}` per monitor. Events mutate it:

- a new video / audio item arriving on a monitor (`core.FrameMedia` on the live / preview frame; Black and Logo count
  as "no media", Clear keeps the background playing) gets `core.PlaybackDefaults`: live autoplays, preview waits
  paused at 0, the item's `Background.Loop` is the default (videos loop, audio does not), the volume carries over;
- the transport bar's callbacks and the control actions;
- the player reaching the end with looping off (`MediaSource.TakeEnded`, polled every frame in `Process`).

`renderPlayback` projects the state onto `render.Current(role)` idempotently (play if wanted and not playing, …; a
seek is applied once per `SeekSeq`, memoised in the view cache) and reads the player back into `core.PlaybackState`
for the transport bar and `/state.playback.{live,preview}`: `playing, position, duration, loop, volume, hasVideo,
hasAudio, title, native, media, failed`. While a source is open the transport is re-projected every frame even when
nothing else is dirty.

## Transport bar (`internal/ui/transport.go`)

Shown under the Preview Output and Live Output monitors only while the monitor's slide is a video or an audio file
(UI-SPEC §3: the output monitor's video controls): a seek slider with "0:00 / 0:30", a round play / pause button, skip
to start / end, a loop toggle, the volume icon and slider. Dumb view: `SetState` (no-signal setters + `flushCalls`),
callbacks `OnPlayPause`, `OnSeek(frac)`, `OnVolume(v)`, `OnLoop(on)`, `OnSkip(dir)`. Drags seek on release; a click on
the track or a keyboard step seeks at once. Icons: `play`, `pause`, `skip_back`, `skip_fwd`, `loop`, `volume`,
`knob` (the sliders' grabber), `audio` (the library's Audio row); the `HSlider` styling is `buildSliderTheme`.

## Control actions

`play`, `pause`, `toggle-play`, `seek {"seconds"}`, `volume {"value"}`, `loop {"on"}`, each with `"target": "live"`
(default) or `"preview"`. `search-media {"kind": "audio"}` lists the Audio collection; `/state.media` has `audio`
(count) and `native`.

## Audio playlist (background audio)

An audio item in the schedule plays only while it is the Live item. Churches also need music that keeps playing
while other items go live: walk-in music under the announcements loop, quiet music under a prayer slide. That is
the **audio playlist** under the Schedule panel.

**EasyWorship.** EW 7/8 has no playlist. Its audio files are schedule items with Media Usage (Auto / Background /
Foreground) and Repeating on the Audio Properties form (`Form_VideoProperties`, `Frame_InspectorMedia`), and the
library's Audio collection (`ResourceAudioAdd` / `ResourceAudioEdit`). The decoded forms have no Playlist,
Soundtrack or background-music bar. So the feature is our own, kept minimal and in the existing UI's terms: a
panel in the Schedule column (it belongs to the service, like the schedule), the Media tab's Audio collection as
its source, and the monitors' transport vocabulary (round play / pause, skip, loop, volume icons).

**Behaviour (decisions).**

- A playlist is an ordered list of audio files from the media library, saved **with the schedule**
  (`models.Schedule.Playlist`, the `playlist` key of the `.fbs` file: `tracks [{id, mediaId, path, title, seconds}]`,
  `repeat`, `shuffle`, `fadeOut`). It is left out of the file while it is empty with default settings
  (`omitzero` + `Playlist.IsZero`), so schedules without music are written exactly as before; schedules from before
  the playlist (and `.fws` files) open with an empty one. The format version stays 1: older FreeBeams builds ignore
  the key. Tracks keep their own path and title, so a schedule still plays a file that left the library; a track
  whose file is gone is shown dimmed and skipped. Not in the library itself: one playlist per service is what
  churches plan, and a library of playlists can come later from the same model.
- It plays **independently of the Live slide**: going live with anything (a song, a video, another audio item),
  Black, Clear and **Logo** never touch it. Logo is deliberately included: walk-in music under the logo screen
  before the service is the main use. A Live audio or video item plays on top of it (the operator pauses the
  playlist if they want).
- Play starts the cued track (or resumes); at the end of a track the next one starts. **Repeat** cycles off → all →
  one (the transport's button; `loop` icon, `repeat_one` for one): off stops after the last track with the first
  one cued, all wraps around, one plays the track again. The operator's Next / Prev ignore repeat one. **Prev**
  restarts a track that has played for more than 3 s (`core.PrevRestart`), as music players do.
- **Shuffle** plays every track once per pass in a random order (no track twice in a row across passes); turning
  it on keeps the current track first, a track added mid-pass is put among the ones still to come, and a picked
  track (double-click) takes the current place so the pass stays complete.
- **Stop** fades out over the playlist's fade-out (⚙ ▸ Fade Out on Stop ▸ Off / 2 / 5 / 10 s; any 0–30 s through
  the control API), then closes the player; a second Stop during the fade stops at once, Play cancels it. Stop keeps
  the track cued (it starts from 0).
- A missing file (checked with `os.Stat` whenever the player moves) and a file the player fails on
  (`TrackFailed`, logged once) are skipped; when nothing can play, Play says so.
- The volume is the operator's, not the schedule's (not saved, not dirty). Editing the list, repeat, shuffle and
  the fade-out marks the schedule dirty. New / Open schedule stop the music and cue the new playlist's first
  track. Removing the playing track plays the one now at its place.

**Pieces.**

- `internal/models/playlist.go`: `Playlist`, `PlaylistTrack`, `RepeatMode`; Insert / Remove / Move / Normalize,
  `Schedule.EditPlaylist` (marks dirty). `playlist_test.go`.
- `internal/app/core/audio.go`: `AudioPlayer`, the pure state machine (status, current track by ID, volume,
  seek requests, fade-out, the shuffle order of the pass, failed files) with injected `Exists` and `Rand`;
  `State` builds `/state.audio`. `audio_test.go` covers the order, the end of the list, repeat all / one,
  missing and failing files, shuffle without repeats, Prev / Next while stopped or paused, the fade, and edits.
- `internal/render/audiosource.go`: `RoleAudio`, an audio-only `MediaSource` role (`OpenAudio` / `CloseAudio`),
  and `PumpMedia`. It is additive: the `nativePlayer` interface and `videosource.go` are unchanged, so the playlist
  plays through whatever backend plays the monitors (AVFoundation on macOS; the ffmpeg-pipe player once it lands
  for Windows / Linux). The source has no layer: the app opens and closes it in render and pumps it every frame,
  since no SlideView may hold a source while only music plays.
- `internal/app/audio.go`: events (panel callbacks, control actions, `audioEvents` turning the player's end /
  failure into state, the fade's end), `renderAudio` (opens the file the state wants, applies seek before play,
  volume × fade gain, play / pause; then the panel), `/state.audio`, the phone's music bar.
- `internal/ui/playlist.go`: `ui.AudioPlaylist`, a dumb view under the schedule list: header (+ ▾ Add Selected
  Audio / Add Audio Files…, ⚙ ▾ Fade Out on Stop ▸ / Remove Track / Clear Playlist), the track list (▶ / ❚❚ on the
  current track, missing files dimmed; double-click plays; right-click Play Track / Move Up / Move Down / Remove),
  and the transport: title, `0:12 / 3:00`, seek slider, ⏮ ⏯ ⏭ ⏹, repeat, shuffle, volume. The Media tab's tile menu
  has **Add to Audio Playlist** (audio tiles only). Icons `stop`, `shuffle`, `repeat_one` (`uicore/icons.go`).

**Automation.** `/state.audio`: `status (stopped|playing|paused), playing, track, title, next, order, position,
duration, volume, gain, fading, repeat, shuffle, fadeOut, tracks [{title, path, seconds, missing}], native, loaded,
failed`. Actions: `audio-play {index}`, `audio-pause`, `audio-toggle`, `audio-stop {fade}`, `audio-next`,
`audio-prev`, `audio-seek {seconds}`, `audio-volume {value}`, `audio-repeat {mode}` (no mode cycles),
`audio-shuffle {on}` (no on toggles), `audio-fade-out {seconds}`, `playlist-add {index | title | path | paths}`
(a Media tab item, default the selection, or audio files, which are added to the library too),
`playlist-remove {index}`, `playlist-move {from, to}`, `playlist-clear {confirm}`, `media-menu {item:
"add-to-playlist"}`. Phones: `audio-toggle`, `audio-next`, `audio-prev` (docs/REMOTE.md).

## Fallbacks

- A file AVFoundation cannot open or play (`status failed`): the slide shows its poster (or black for audio), the
  transport is greyed out, `/state.playback.*.failed` is true, and `[render] <path>: the media player could not play
  the file` is logged once.
- Off macOS the native player does not exist: `.ogv` plays through Theora as before, other videos show their poster
  until transcoded, audio shows black; `playback.*.failed` is true for those (nothing can play them), `native` is false.
- The smoke test's playback block skips itself where ffmpeg (which makes the test clip) is absent or the player is not
  native.

## Not done

- Poster frames through `AVAssetImageGenerator` (videos still need ffmpeg for a thumbnail on macOS).
- A per-item Repeat / Aspect property on `models.Media`: `models.Background.Loop` is the hook (`Media.Background()`
  sets it; `PlaybackDefaults` reads it).
- Audio playback off macOS (the audio playlist included: it plays through the same player layer).
- Audio playlist: drag-and-drop from the Media tab into the list, a crossfade between tracks, playlists kept in the
  library (reusable across services), routing the music to its own output device.
