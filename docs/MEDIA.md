# Media playback

How videos and audio files in the Media tab play, per platform, and what the operator gets.

## What plays where

| Platform | Player (`/state.media.player`) | Videos | Audio | Needs |
|---|---|---|---|---|
| macOS | `avfoundation` (`avplayer_darwin.go`) | every format AVFoundation decodes (H.264 / HEVC in `.mp4` `.mov` `.m4v`, and `.webm` / `.ogv` only where the OS has a decoder) | `.mp3` `.m4a` `.aac` `.wav` `.aiff` `.aif` `.flac` (`.ogg` only with an OS decoder) | nothing: the source file plays as it is |
| Windows, Linux | `ffmpeg` (`ffplayer.go`, `internal/media/ffplayer`) | everything ffmpeg decodes (H.264, HEVC, VP8/9, AV1, MPEG-2/4, ProRes, Theora, … in any container), scaled to at most 1920×1080 | everything ffmpeg decodes (mp3, AAC, WAV, AIFF, FLAC, Ogg Vorbis / Opus, …) | ffmpeg + ffprobe: the releases bundle an LGPL build next to the executable, else PATH |
| Windows, Linux without ffmpeg | `theora` (`video.go`) | Ogg Theora (`.ogv`) only, muted motion backgrounds through Godot's `VideoStreamPlayer` | not played (badge `Needs ffmpeg`) | — |

`avfoundation` and `ffmpeg` are "native" (`core.MediaCaps.Native`, `/state.media.native`, `playback.*.native`):
the source file plays as it is, with sound, and the transport works. The app picks the player once at startup
(`App.selectMediaPlayer` → `core.ChoosePlayer` → `render.UseMediaPlayer`, logged as `[freebeams] media player:
ffmpeg (<path>)`); **`FREEBEAMS_PLAYER=ffmpeg|avfoundation|theora`** forces one, which is how the ffmpeg player
is verified on a Mac (`FREEBEAMS_PLAYER=ffmpeg test/smoke.sh`). **`FREEBEAMS_FFMPEG_DIR=<folder>`** makes
`media.Locate` take ffmpeg / ffprobe from that folder before the bundled copy and PATH.

Poster frames and metadata (`Describe`: size and duration): on macOS AVFoundation first (`media.AVFoundation`,
`internal/media/avasset_darwin.go`: `AVURLAsset` for the duration and the first video track's display size,
`AVAssetImageGenerator` for the frame one second in, else frame 0, drawn into a `CGBitmapContext` over a C buffer in
the frame's own colour space and written as a 640 px JPEG by Go), with ffmpeg / ffprobe, when present, for what it
cannot read (`media.Chain`: WebM without a system decoder, WebP / BMP stills). So a Mac needs no ffmpeg for video
thumbnails at all. It runs in the media pipeline's goroutine (Core Graphics without AppKit is thread-safe).
Elsewhere ffmpeg / ffprobe do it; without them videos show a plain tile. `/state.media.posters` says which:
`avfoundation`, `ffmpeg` or `""`. A native player never
transcodes (`media.Processor.Native`, set by the app from `render.NativeVideoSupported()`), so the "Needs libtheora"
badge and the "videos cannot play" footer note only appear on the Theora fallback, where audio items say
`Needs ffmpeg` and the footer `ffmpeg not found: audio cannot play` (or `videos and audio`).

Audio goes to the system's default output device at the player's volume (the transport's volume slider). With
AVFoundation it is not routed through Godot's audio bus; the ffmpeg player plays through Godot (an
`AudioStreamPlayer` on the `Master` bus). A video with a sound track plays its sound too.

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

## Windows and Linux: the ffmpeg player (`internal/media/ffplayer`, `internal/render/ffplayer.go`)

PLAN §3 option B. The decoding is pure Go (`internal/media/ffplayer`, unit-tested with plain `go test` against
clips ffmpeg generates, also on `windows-latest` / `ubuntu-latest` in CI); `render/ffplayer.go` is the thin
adapter behind `nativePlayer`, so `MediaSource`, the transport, looping, seeking, volume, the crossfade and the
audio playlist work unchanged.

- **Open**: `ffprobe -show_streams -show_format -of json` (in a goroutine) finds the first video stream that is
  not cover art (`disposition.attached_pic`), the first audio stream, the duration, the display size (sample
  aspect ratio and rotation applied: `Tracks` reports it like AVFoundation's `presentationSize`) and the frame
  rate (`avg_frame_rate`, capped at 60). `Status` is loading until the first frame (audio: the first samples)
  is decoded, failed when ffprobe or the first decoder fails (logged as `the media player could not play the
  file (<ffmpeg's last stderr line>)`).
- **Two processes per position**, one pipe each (stdout), started at the same `-ss`: video `-map 0:<v> -vf
  fps=fps=<r>:start_time=0,scale=<w>:<h>:flags=bicubic+accurate_rnd+full_chroma_int,setsar=1 -pix_fmt rgba -f
  rawvideo pipe:1` (frame *n* is at *n* / fps; the frame size is fixed by the probe, so a frame is exactly
  w·h·4 bytes) and audio `-map 0:<a> -af aresample=<mix rate>:async=1:first_pts=0 -ac 2 -f f32le pipe:1`
  (sample *n* is at *n* / rate). Two processes because `exec.Cmd.ExtraFiles` does not exist on Windows and one
  process writing two pipes stalls the sound whenever the video queue is full. Every process gets
  `sysproc.HideWindow` (`CREATE_NO_WINDOW`) and a `WaitDelay`.
- **Goroutines → main thread**: the video reader fills a queue of 2 frames from a free list of 4 RGBA buffers
  (≈ 33 MB per player at 1080p); the audio reader fills a lock-protected ring of 1 s. `Update` (called from the
  adapter's `Tick`, which `pumpSources` calls once per frame before `TakeEnded` / `CopyFrame`) advances the
  clock, stops at the end and takes the frames whose time has come (the newest one is shown, older ones are
  dropped, a future one is held). The first frame after an open or a seek is shown at once, playing or not
  (the preview's frame 0).
- **Clock**: with sound, the audio clock: the samples handed out minus what the `AudioStreamGenerator` still
  holds (`SetAudioBacklog`, the generator's capacity minus `get_frames_available`); without sound, the wall
  clock. A sound that does not move for 2 s and 30 frames (no audio device, a stuck decoder) hands over to the
  wall clock until it moves again; one long hitch (a modal dialog) is not a stall. A sound track that cannot be
  decoded at all plays the video silent on the wall clock (`[ffplayer] <path>: the sound cannot play: …`).
- **Audio out**: an `AudioStreamGenerator` (the mix rate, 0.25 s buffer) on an `AudioStreamPlayer` under the
  scene root (found through `Engine.GetMainLoop`, so the playlist's `RoleAudio` source with no view has sound
  too). Pause is `stream_paused`; the volume is `volume_linear`. A seek or a restart is a new *audio epoch*:
  the adapter stops the player and plays it again, which makes a new, empty generator playback
  (`clear_buffer` errors while the playback is active).
- **Seek** restarts both decoders at the new `-ss` (accurate input seeking); the position is the target at
  once. **Play after the end** (looping off) starts again from 0.
- **Looping, gapless**: every pass has the container's duration *D* (the first one *D* − `ss`): the audio reader
  pads its pass with silence or cuts it to exactly that many samples, the video reader drops frames past it,
  so the two timelines agree on every pass and nothing drifts. While looping, the video reader starts the next
  pass's process as soon as the current one delivered its first frame, so the next pass's first frame is
  already decoded when the current one ends; the audio has its 1 s ring ahead. Turning looping off mid-pass
  ends at the end of that pass (`TakeEnded`, position = duration); turning it on after a reader reached its
  end lets it carry on. A file without a duration ends when every stream is played out.
- **Close** cancels the run's context (kills both processes through `exec.CommandContext`), closes the ring and
  frees the audio node; `Player.Done()` is closed once every goroutine and process is gone
  (`TestCloseStopsEverything` checks the goroutine count; the smoke's playlist block checks that no ffmpeg is
  left decoding a track after Stop and after the fade).
- Colour: the scale filter reads the frame's colour matrix / range; the smoke clip's BT.709 `#2a7f62` comes out
  within 2 levels per channel (`#297d61` / `#297e62`; the smoke's pixel check allows 3);
  `accurate_rnd+full_chroma_int` matter when scaling (without them a 320→64 downscale gave `#277c5f`).
- Cost: two `ffmpeg` processes per open file (plus the next pass's, idle, while looping), one RGBA copy per new
  frame into the source buffer, `Image.SetData` + `ImageTexture.Update` as with AVFoundation.

## Shared sources (`internal/render/videosource.go`)

A `MediaSource` is keyed by `(role, path)` and refcounted:

- `RoleLive`: the Live Output monitor **and** the projector window (`render.Output` gives its view `RoleLive`), so one
  decoder feeds both and the sound plays once;
- `RolePreview`: the Preview Output monitor, its own decoder, paused at 0 by default;
- `RoleResource`: the resource preview, its own decoder, paused at 0 (its mini transport);
- `RoleNone` (thumbnails): no player, the poster still only.

A source owns the native player, one RGBA8 `Image` and the `ImageTexture` every attached layer draws
(`ImageTexture.Update` when the size is unchanged, `SetImage` otherwise). Views with a role keep `Process` on while they
hold a source and call `pumpSources()` once per engine frame (`Engine.GetProcessFrames()` guard): status transitions,
the tracks once ready, pending play / seek, a new frame if there is one. `AcquireAnimating` is held while a source
plays (low-processor mode off), released when it pauses, ends, fails or closes.

Layers (`layer.native`, a `TextureRect` between the poster and the text, stretched by the item's Aspect Ratio)
acquire their source when they have a path, a role and are visible in the tree, and release it when hidden: the
outgoing side of a finished crossfade keeps its last frame on its (now hidden) texture rect and drops its reference.

**Holds** (`render/hold.go`): the app also holds the file each playback state is about (`render.Hold(role,
path)`, from `renderPlayback`, idempotent), so the player stays open whatever the layers do. `render.Source(role,
path)` finds it even while another file (a Logo video) is the role's `Current`. Held sources no view pumps are
pumped by the app (`playbackEvents` calls `render.PumpMedia()` while any source is open).

`RoleResource` is the resource preview's own player (its mini transport), so a clip can be previewed without
touching the Preview or Live players.

`render.Current(role)` is the source most recently opened for a role (what the monitor shows now).
`render.CloseSources()` runs from `ReleaseCaches()` at shutdown.

## Black, Clear and Logo

EasyWorship's Black and Logo cover the output; they do not stop a video. The decoded forms have no setting for it
(Form_VideoProperties has Media Usage, Repeating, Aspect Ratio and Volume / Mute, nothing about Black), and in
EasyWorship a clip keeps running, with its sound, under Black. FreeBeams does the same by default:

- `core.MonitorMedia(frame)` is the media a monitor keeps playing: the slide's (or the theme's) video or audio in
  the normal and clear modes (shown) and **also under Black and Logo** (`shown` false, `/state.playback.*.hidden`).
- `core.AdvancePlayback` keeps the operator's playback (play / pause, loop, volume, seek count) while the file stays
  the same, Black / Logo and back included; a new file gets its defaults.
- The hold keeps the player open, so the position carries on and the **sound goes on under Black**; coming back
  shows the clip where it has got to (paused under Black: exactly where it was). The transport bar stays visible
  under Black, so the operator can pause the sound there.
- Going live with another item under Black starts that item's playback (hidden) as it would without Black.
- To silence a clip with the output, the operator pauses it (or uses the volume), or turns on **Pause on Black**.

### Pause on Black (Options ▸ Logo)

A FreeBeams option, not in EasyWorship's forms: Options ▸ Logo ▸ Black and Logo ▸ "Pause video and audio under Black
and Logo" (`core.Settings.PauseOnBlack`, persisted as `playback.pauseOnBlack`, **off by default** so the behaviour
above stays the default; `set-setting {key: "playback.pauseOnBlack" | "pauseOnBlack", value}`,
`/state.settings.pauseOnBlack`). It is authoritative playback state, not render: `core.CoverPlayback` runs after
`core.AdvancePlayback` every frame on each monitor's `core.Playback` and acts only on a *change* of "covered"
(`Playback.Hidden`, from `core.MonitorMedia`):

- Black or Logo covering the **live** file while it plays pauses it and marks it `BlackPaused`
  (`/state.playback.live.blackPaused`); a paused file stays paused and is not marked.
- Leaving Black / Logo (to the normal or clear mode) plays a `BlackPaused` file again, from where it was; a file
  that was paused when Black was pressed stays paused.
- The operator's play or pause in between (`Playback.SetPlaying`: the transport, `play` / `pause` /
  `toggle-play`, the phones) clears the mark, so the operator can still play a clip under Black and their choice
  wins when Black goes off. Black to Logo and back stays covered (no change).
- An item going live under Black waits paused (marked) and starts when Black goes off. Turning the option off
  while a file is `BlackPaused` still resumes it when Black goes off. Preview and the resource preview are never
  covered by Black. The audio playlist (background music, its own player) is not paused: walk-in music under the
  Logo screen is the usual case.

## Playback state (`internal/app/playback.go`)

State-machine rule: what the operator wants of each monitor is app state, `core.Playback{Key, Playing, Loop, Volume,
SeekSeq, SeekTo}` per monitor. Events mutate it:

- a new video / audio item arriving on a monitor (`core.AdvancePlayback` over `core.MonitorMedia`: Black, Logo and
  Clear keep the file, see above) gets `core.PlaybackDefaults`: live autoplays, preview waits
  paused at 0, the item's `Background.Loop` is the default (videos loop, audio does not), the volume is the item's
  (see "Per-item volume and mute") or carries over;
- Black / Logo covering and uncovering the live file under Pause on Black (`core.CoverPlayback`, above);
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

## Resource preview transport (`internal/ui/minitransport.go`)

EasyWorship's `Frame_VideoControlsMini` under the resource preview: a 28 px mini toolbar with skip to start, a round
24 px play / pause, skip to end, the progress slider, `0:00 / 0:30` and the volume (an icon and a slider: the
mute button and the volume dropdown of EasyWorship are folded into it). The bar clips its parts instead of
widening the pane: when the pane is too narrow it drops the volume, then the time (`MiniTransport.fit`; at the
default split it shows the time but not the volume). It is shown while the resource preview
shows a video or audio file (also a theme's video) and drives `RoleResource`: its own decoder, paused at 0, its own
volume, never the Preview or Live players nor the output. Same dumb-view contract as `ui.Transport`
(`ui.TransportCallbacks`, `SetState`); the state is `App.pb.resource`, `/state.playback.resource`, and the
transport actions take `"target": "resource"`.

## Per-item properties (Edit Properties…)

The media tile menu has **Edit Properties…** (EasyWorship's Edit Video / Audio Properties, `Form_VideoProperties`),
a small dialog (`ui.MediaPropertiesDialog`, titled Video / Audio / Image Properties) with what FreeBeams keeps per
item: **Title**, **Aspect Ratio** (Fill / Fit / Stretch: EasyWorship's maintain / zoom / stretch), **Repeating**
(Loop / Play Once; videos and audio, also in the tile menu's Repeating ▸ now) and **Tags** (the Info tab's Tags memo,
comma-separated, normalised by `models.NormalizeTags`, matched by the library search). Saved by
`store.SetMediaProperties` (a targeted update; migration 7 adds `media.tags`; the pipeline's whole-row saves re-read
title, tags, aspect and repeat first). `Media.Background()` turns Aspect / Repeating into `Background.Fit` /
`Loop`, so the library deck, Preview and **schedule items** (their snapshot, made when added; Update items in
Schedule refreshes it) play once or loop and letterbox / stretch as set. Left out: Mark In / Out, Media Usage,
Alpha Channel, Select Thumbnail, Author / Copyright / Description.

Control: `media-menu {item: "properties", index|title}` opens it; `media-properties {props: {title, tags, aspect,
repeat, volume, mute}}` fills it and presses OK (an empty aspect / repeat keeps the dialog's choice; title, tags,
volume and mute are always filled in, a missing volume is Carry Over; a volume outside 0..100 is refused),
`{cancel: true}` presses Cancel; `/state.dialog.mediaProperties`, `/state.media.{tags, volume, mute}` (the
selected item); screenshot window `media-properties`.

### Per-item volume and mute

The dialog's **Volume** row (videos and audio; EasyWorship's Audio Properties: `TrackBar_Volume` +
`SpinEdit_Volume`, `CheckBox_Mute`) is **Carry Over** (the default) or **Fixed** with a slider and a 1..100 % box
that follow each other, and **Mute**. `models.Media.Volume` (percent, 0 = Carry Over) and `Mute`, migration 8
(`media.volume`, `media.mute`), saved by `store.SetMediaProperties` only (`SaveMedia` never writes them; the
pipeline's save re-reads them like the tags). `Media.Background()` copies them into `Background.Volume` / `Mute`,
so the library deck, Preview, the resource preview and schedule snapshots (`omitempty`: schedules without them are
unchanged; Update items in Schedule refreshes them) all carry them.

The item's volume is the *start* volume when the item arrives on a monitor (`core.StartVolume` in
`core.PlaybackDefaults`):

- **Mute**: starts at 0 (the Fixed volume is kept for when Mute is unticked). The operator's transport can
  still raise it for this playback.
- **Fixed** n %: starts at n / 100.
- **Carry Over**: starts at the monitor's carried volume, as before: the operator's volume of the previous item
  without a volume of its own (1.0 at start).
- An item with a volume of its own (`Playback.Own`, `/state.playback.*.itemVolume`) does not pass it on: the
  operator's changes stay with that playback, and the next Carry Over item starts at the volume from before it
  (`Playback.Carry`, `CarriedVolume`). So a quiet clip set to 40 % does not leave the next song's video at 40 %.
- The same file staying on a monitor (Black, Logo, Clear and back) keeps the playback, the volume included. Live
  and the schedule keep their snapshot's volume until Update items in Schedule, as with Repeating.

## Control actions

`play`, `pause`, `toggle-play`, `seek {"seconds"}`, `volume {"value"}`, `loop {"on"}`, each with `"target": "live"`
(default), `"preview"` or `"resource"`. `search-media {"kind": "audio"}` lists the Audio collection; `/state.media` has `audio`
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

- A file the player cannot open or play (`status failed`): the slide shows its poster (or black for audio), the
  transport is greyed out, `/state.playback.*.failed` is true, and `[render] <path>: the media player could not play
  the file` is logged once (the ffmpeg player adds ffmpeg's error in parentheses).
- Off macOS without ffmpeg (the `theora` player): `.ogv` plays through Theora as before, other videos show their
  poster until transcoded, audio shows black; `playback.*.failed` is true for those (nothing can play them),
  `native` is false.
- The smoke test's playback and playlist blocks skip themselves where ffmpeg (which makes the test clips) is absent
  or the player is not native.

## Not done

- EasyWorship's Mark In / Out and Media Usage. A per-item volume for the audio playlist's tracks (they play at the
  playlist's own volume).
- The ffmpeg player on a real Windows or Linux machine: verified on macOS (`FREEBEAMS_PLAYER=ffmpeg`, Homebrew ffmpeg
  and the bundled LGPL build), and its pure-Go tests run in CI on `windows-latest` / `ubuntu-latest`, but the app
  itself has not run there yet. Hardware decoding (the bundled build has none: `--disable-autodetect`), a
  YUV → RGB shader instead of the RGBA conversion in ffmpeg, and 4K output.
- Audio playlist: drag-and-drop from the Media tab into the list, a crossfade between tracks, playlists kept in the
  library (reusable across services), routing the music to its own output device.
