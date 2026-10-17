# Phase 1 Goal: "Run a Sunday song & scripture service from the keyboard"

Phase 1 = PLAN.md milestones **M0–M3**, plus a basic schedule (part of M5). Media/video backgrounds, the theme
editor, presentations, settings UI, more transitions and packaging are **Phase 2**.

## Acceptance criteria (all must hold)

### Build & quality
- [ ] `go build ./...`, `go vet ./...` and `go test ./...` pass with no failures and no skipped core tests.
- [ ] `gd run` launches the app on macOS with no Go panics or Godot errors in the log.
- [ ] Packages under `internal/models`, `internal/store`, `internal/importers` never import `graphics.gd`.
- [ ] Core logic has table-driven unit tests: lyric parsing/slide splitting, scripture reference parsing,
      presenter state machine, schedule save/load round-trip, store CRUD/search, importers against the real
      EasyWorship data on this machine (tests skip cleanly when that data is absent, e.g. on CI).

### Operator window (matches docs/UI-SPEC.md, dark palette)
- [ ] 58px main toolbar with New, Open, Save | Go Live | Logo, Black, Clear | Live, using our own icons.
- [ ] Schedule | Preview | Live top area and the resource area below, with draggable splitters at the spec percentages.
- [ ] Panel headers (28px mini toolbars) titled Schedule / Preview / Live.
- [ ] Resource tab bar: Songs, Scriptures (Media/Presentations/Themes tabs present but may show "Coming in Phase 2").

### Songs
- [ ] On first launch the EasyWorship song library is imported (223 songs here). If EW is not installed, the library starts empty.
- [ ] Songs tab: search box (FTS on title/author/lyrics), list with Title/Author/Copyright columns, selecting shows the slides in Preview.
- [ ] New/Edit song dialog: title, author, copyright, CCLI and a lyrics text box using `[Verse 1]`/`[Chorus]` tags or blank-line separated stanzas. Saving persists the song.

### Scriptures
- [ ] The Bibles available in the EasyWorship install are imported. Only public-domain texts (e.g. KJV) are imported automatically. Copyrighted ones (e.g. HCSB) are skipped unless the user opts in.
- [ ] Typing a reference (`jn 3:16`, `John 3:16-18`, `1 cor 13`, `ps 23:1-3`) shows the verses as slides in Preview (one verse per slide, with a reference footer).

### Presenting
- [ ] Double-click a slide or press Enter/Go Live: the preview item goes live and the clicked slide is shown.
- [ ] Output window: borderless full-screen on the first non-primary display. With one display it opens as a normal 16:9 window.
- [ ] Slide text auto-fits (shrinks to fit the text box) with outline and shadow. Backgrounds can be a color, a gradient or an image.
- [ ] Crossfade transition (default 0.5s) between live slides and into/out of Black/Clear/Logo.
- [ ] Hotkeys from UI-SPEC §3.7: next/prev slide, first/last, schedule up/down, Ctrl+B/C/L, and section jumps V/C/B/R/T/I/E.
- [ ] The Live pane highlights the current live slide. The schedule marks the live item with a red outline.

### Schedule
- [ ] "Add to schedule" from Songs/Scriptures, remove, move up/down, and select an item to load it into Preview.
- [ ] Save/Open `.fws` (JSON) through native file dialogs. The window title shows the schedule name and a dirty marker.

### Testability
- [ ] Control server (bearer-key HTTP, as in Bufflehead) with `GET /state`, `GET /screenshot?window=main|output`,
      and `POST` actions to drive go-live/next/prev/black/clear/logo, select a song or reference, and add to schedule.
- [ ] `test/smoke.sh` launches the app, drives a song and a scripture to live through the control API, saves screenshots
      to `test/out/`, and asserts the expected `/state`.
