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
- Audio playback off macOS.
