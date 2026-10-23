# FreeBeams UI Spec (matches EasyWorship 8)

**Source:** extracted directly from the installed EasyWorship 8.0.49 binary. It is a Delphi
FireMonkey app, and its forms are embedded as TPF0 streams. They were decoded with
`docs/ew-ui/dfm.py` into `docs/ew-ui/forms/*.dfm` (894 forms). The main window tree with geometry is in
`docs/ew-ui/main-window-tree.txt`. **`docs/ew-ui/` is git-ignored: it is reference material only.**

All numbers below are EasyWorship's design-time values in logical points (1pt = 1 Godot unit at scale 1).
Splitter percentages are the defaults; at runtime the user can drag them and the values persist.

**Branding:** we copy layout, geometry, palette and behavior. We draw our **own** icons
(EasyWorship's `buttons_large` image collection is their artwork), and the name/logo is FreeBeams.

---

## 1. Palette (exact values from the `_sd.resources` style)

The default interface is **Dark** (`InterfaceColorSelector.ActiveColor = icDark`). View → Interface Color switches between Light and Dark.

| Token | Dark | Light | Used for |
|---|---|---|---|
| `window.bk0` | `#202020` | `#F9F9F9` | deepest background (text fields) |
| `window.bk1` | `#2C2C2C` | `#F9F9F9` | panels (schedule body); the output surrounds and the resource area take the Mac tokens below |
| `window.bk2` | `#363636` | `#F0F0F0` | **main window background** (the slide grids take `grid.bk` below) |
| `window.bk3` | `#484848` | `#D0D0D0` | raised areas |
| `window.border` | `#000000` | `#646464` | window border |
| `text` | `#B9B9B9` | `#2A2A2A` | normal text |
| `text.semibold` | `#F0F0F0` | `#000000` | panel titles ("Schedule", "Preview", "Live") |
| `text.hyperlink` | `#699AF9` | `#529AEF` | links |
| `highlight` | `#3A6DCE` | `#529AEF` | **selection (active)**: selected slide/item |
| `highlight.hot` | `#4B8CE9` | `#62AAFF` | hover on a selected item |
| `highlight.text` | `#F9F9F9` | `#F9F9F9` | text on highlight |
| `highlight.inactive` | `#626262` | `#CECECE` | selection when the panel is unfocused |
| `highlight.inactive.hot` | `#707070` | `#DCDCDC` | |
| `highlight.text.inactive` | `#C2C2C2` | `#2A2A2A` | |
| `highlight.2` | `#395792` | `#CCE4F7` | secondary selection (list rows, menu item hover) |
| `highlight.2.hot` | `#436AA2` | `#E5F1FB` | |
| `highlight.2.text` | `#B9B9B9` | `#2A2A2A` | |
| `button.bk` | `#2C2C2C` | `#E1E1E1` | button background |
| `splitterbar` | `#000000` | `#AAAAAA` | splitter lines |
| `border` | `#000000` | `#AAAAAA` | general borders |
| `border.bl` | `#101010` | `#D4D4D4` | bottom/left borders |
| `border.tl` | `#000000` | `#8C8C8C` | top/left borders |
| `selectorline` | `#4D4D4D` | `#D0D0D0` | slide-grid separators |
| `headercolumn.stroke` | `#000000` | `#D0D0D0` | list header column lines |
| `libraryselector.bk` | `#202020` | `#DEDEE7` | library (collections) list background |
| `preseditor` | `#1F1F1F` | `#D0D0D0` | slide editor canvas surround |
| `divider.menu` | `#5D5D5D` | `#D7D7D7` | menu separators |
| `divider.button.leading` / `trailing` | `#242424` / `#8F8F8F` | `#A0A0A0` / (none) | 2px etched toolbar dividers |
| `menuview.fill` / `stroke` | `#363636` / `#000000` | `#F0F0F0` / `#AAAAAA` | popup menus |
| `menuitem.fill` | `#395792` | `#CCE4F7` | menu item hover |
| `menugutter.fill` | `#303030` | `#E6E6E6` | |
| red | `#FF0000` | `#FF0000` | Live indicator / errors |

**Gradients** (top → bottom):

| Token | Dark | Light |
|---|---|---|
| `toolbar` (main 58px toolbar) | `#6E6E6E → #4C4C4C` | `#F7F3F7 → #CECFCE` |
| `toolbar.mini` (28px panel headers/footers) | `#343434 → #292929` | `#E5E5E5 → #F4F4F4` |
| `toolbar.mini2` | `#363636` flat | `#E5E5E5 → #F4F4F4` |
| `tabbar.1` (resource tab bar) | `#6E6E6E → #424242` | `#F7F7F7 → #CECECE` |
| `tabbar.2` (dialog tab bars) | `#2C2C2C → #4C4C4C → #4C4C4C → #404040` | `#CCCCCC → #ECECEC → #ECECEC → #E0E0E0` |
| `hsplitterbar.closed` | `#555555 → #2C2C2C` | `#F9F9F9 → #C8C8C8` |
| `gutter.fill` (trackbar) | `#262626 → #262626 → #343434 → #4C4C4C` | `#BDBDBD → #E7E7E7` |
| `gutter.progress.highlight` | `#66ACFF → #2B5CAF` | same |
| mini button normal | `#515151 → #3D3D3D → #424242` | `#E6E6E6 → #D2D2D2 → #E6E6E6` |
| mini button hot | `#606060 → #4A4A4A → #505050` | `#F0F0F0 → #E6E6E6 → #F0F0F0` |
| mini button selected/pressed | `#363636 → #303030 → #363636` | `#DCDCDC → #C8C8C8 → #DCDCDC` |
| inactive selection overlay | `#828282` @ 90% | `#000000` @ 50% |

### Mac measurements (the dark interface's surfaces)

The tokens above come from the Windows binary's style resources. The user's screenshots of
EasyWorship **for Mac** (dark interface, Retina; measured with PIL at the pixel) are darker and more
neutral on several surfaces, and the Mac build gives some surfaces a colour of their own where the
Windows style shares a token. FreeBeams's dark interface follows the Mac screenshots. These tokens
live in `uicore.Palette` next to the spec tokens (`TestPaletteMatchesSpec` checks both tables); the
light values are the Windows style's token for the same surface, so the light interface is unchanged
(there is no light Mac reference).

| Token | Dark (Mac) | Light | Was (Windows token) | Used for |
|---|---|---|---|---|
| `grid.bk` | `#2B2B2C` | `#F0F0F0` | `window.bk2` `#363636` | Preview / Live slide grid bodies (`FwGridBk`) |
| `output.surround` | `#161616` | `#F9F9F9` | `window.bk1` `#2C2C2C` | the letterbox around the Preview / Live output monitors (`MonitorHost`) |
| `resource.bk` | `#161616` | `#F9F9F9` | `window.bk1` `#2C2C2C` / `window.bk0` `#202020` | the resource area: item lists (`Tree` panel), media / presentation / theme grids, the search strip, the resource preview pane |
| `libraryselector.bk` | `#0F1010` | `#DEDEE7` | `#202020` | library (collections) list; the spec token itself, re-measured |
| `card.bk` | `#626161` | `#E1E1E1` (`button.bk`) | flat rows on `window.bk1` | schedule items: raised rounded cards with a white semibold title and an italic grey subtitle |
| `tab.sel` | `#3F3F3F` | `#CECECE` (`highlight.inactive`) | `highlight.inactive` `#626262` | body of the selected resource tab |
| `tab.divider` | `#303030` | `#E3E3E3` (`tabbardivider.fill`) | `tabbardivider.fill` `#606060` | the 5px band under the resource tab bar |
| `tile.frame` | `#233563` | `#529AEF` (`highlight`) | `highlight` `#3A6DCE` | frame of a selected slide / media tile; underline of the selected tab |
| `tile.caption` | `#395792` | `#529AEF` (`highlight`) | (part of the `highlight` fill) | the caption bar under a selected tile's thumbnail (`highlight.text` on it) |
| `list.sel.inactive` | `#313131` | `#CCE4F7` (`highlight.2`) | `highlight.2` `#395792` | the selected list row while the list is unfocused (blue `highlight` only with focus) |
| `icon.green` | `#5BB55B` | `#4FA64F` | – | icon placeholder `{g}`: Go Live screen, Web globe, New badge ([ICONS.md](ICONS.md)) |
| `icon.blue` | `#4B8CE9` | `#4A8CE0` | – | icon placeholder `{b}`: Alerts bubble |
| `tabbar.resource` (gradient) | `#2B2B2B` flat | `#F7F7F7 → #CECECE` (`tabbar.1`) | `tabbar.1` `#6E6E6E → #424242` | the resource tab bar |
| `footer` (gradient) | `#161616` flat | `#E5E5E5 → #F4F4F4` (`toolbar.mini`) | `toolbar.mini` | the 28px footer bars under lists and grids (`FwMiniBarTL`) |

Unchanged because they already match the Mac screenshots: the main toolbar gradient (`#6D6D6D → #4D4D4D`
measured), the panel headers and output bars (`toolbar.mini`, `#2F2F2F` measured), the schedule panel
body (`window.bk1`, `#2D2D2D` measured) and the 1px black splitter lines.

## 2. Typography
- Family: platform default (macOS: SF Pro / system; Windows: Segoe UI). In Godot use `SystemFont` with no family names, which gives the OS default.
- Base size **13pt** (118 occurrences). Panel titles use the `label.semibold` style: **14pt, semibold, opacity 0.9, color `text.semibold`**.
- Other sizes in use: 9, 11 (small captions/badges), 15, 16 (dialog headers), 20, 32 (onboarding).
- Toolbar button labels sit under a 32px icon, centered, 11–12pt.

## 3. Main window

Design size 1030 × 643, min 800 × 600. On macOS the menu is the native app menu bar.

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│ ToolBar_Main 58px  [New▾][Open▾][Save][Web] ┃ [Remote▾]     …    [Go Live] ┃ [Alerts▾] ┃ [Logo][Black][Clear] ┃ [Live] │
├───────────────┬──────────────────────────────────────────────────────────────────────────┤
│ Schedule   ⚙▾ │ Preview                                [▣▾]  ┃ Live              [▣▾]     │ ← 28px mini toolbars
│               │ ┌───────────────────────────────────┐        ┃ ┌──────────────────────┐   │
│ schedule list │ │ slide thumbnail grid (Preview)    │        ┃ │ slide thumbnail grid │   │
│ (19.44%)      │ │                                   │        ┃ │ (Live)               │   │
│               │ ├───────────────────────────────────┤        ┃ ├──────────────────────┤   │
│               │ │ Preview Output (16:9 monitor)     │        ┃ │ Live Output monitor  │   │
│               │ └───────────────────────────────────┘        ┃ │ + video controls     │   │
├───────────────┴──────────────────────────────────────────────────────────────────────────┤ ← main split 71% / 29%
│ [Songs][Scriptures][Media][Presentations][Themes]  [+sched][golive]                 37px │ ← resource tab bar
├─────────────────────────┬──────────────────────────────────────────────┬─────────────────┤
│ Library / collections   │ Item list (title, author, … columns)         │ Resource        │
│ (search locator 24px)   │                                              │ preview (16:9)  │
│ [+][⚙▾]   28px footer   │ [▣▾]            [+][⚙▾]   28px footer        │ (14.58%)        │
└─────────────────────────┴──────────────────────────────────────────────┴─────────────────┘
```

### 3.1 Main toolbar (`ToolBar_Main`): height 58, padding-left 6, gradient `toolbar`
Every button is **56 × 54**, margins top/bottom 2, with the icon on top (`Borders.Top = 4`) and the label below.
Dividers are **2 × 50**, margins 4, etched (leading dark line + trailing light line).

Left-aligned, in order: `New ▾` (dropdown: Schedule/Song/Presentation/Song from SongSelect), `Open ▾`, `Save`, `Web`,
divider, `Remote ▾`.

Right-aligned, reading left to right: `Go Live` | divider | `Alerts ▾` | divider | `Logo`, `Black`, `Clear` | divider | `Live` (toggle, StaysSelected; turns the output on and off).

Logo, Black and Clear are toggles (StaysSelected, AllowAllUnSelected): they show the selected state while active.

### 3.2 Splitters
| Splitter | Orientation | Default | Constraints |
|---|---|---|---|
| `SplitterPanel_Main` | vertical (top/bottom) | top **71%** | top min 300, bottom snap-closes below 150 |
| `SplitterPanel_Top` | horizontal | schedule **19.44%** | schedule snap-closes below 200, right min 300 |
| `SplitterPanel_LiveAndPreview` | horizontal | preview **71.19%** of width (design time) | each min 50 |
| `SplitterPanel_Preview` | vertical | slide grid **46.38%**, output below | grid min 150, output snap 200 |
| `SplitterPanel_Live` | horizontal (combined mode) | live slides **63.87%** \| live output | min 150, snap 200 |
| `SplitterPanel_Resources` | horizontal | library+list **85.42%** \| resource preview | min 300, preview snap 150 |
| `SplitterPanel_Songs` (and each resource tab) | horizontal | collections **49.83%** \| items | each min 150 |

Splitter bars are 1px `splitterbar`. A snapped-closed splitter shows a gradient grip (`hsplitterbar.closed` / `vsplitterbar.closed`) that reopens it on click.

FreeBeams defaults Preview | Live to **50%**, as in the mockup (`docs/mockup/main-window.html`); EasyWorship's 71.19% is
its design-time value. Positions the user drags to are saved in the `ui.layout` setting and restored at startup.

### 3.3 Panel header mini-toolbars (Schedule / Preview / Live)
- Height **28**, padding-left 6, gradient `toolbar.mini`, 1px bottom border `border.bl` (`sd.toolbar.mini.bl.style`).
- Left: title label (`label.semibold`: "Schedule", "Preview", "Live").
- Right (auto-size, padding-left 4): view-size button **24 × 24** (`view_large` icon) joined to a **16 × 24** dropdown arrow (split button: left half/right half styles).
  Schedule also has a **40 × 24** gear dropdown (`ScheduleAction`) before them, with a 2px divider.
- Live header has an extra left **GoBack** button with a divider (`ToolBar_GoBackLeft`).
- Under Preview's output: a 28px bar with the "Preview Output" label (margins 6). Live output's bar holds a foldback-select button and the "Live Output" label (62px wide).
- **FreeBeams addition (no EasyWorship equivalent):** the Schedule column ends with the **Audio Playlist** panel
  (background music that plays independently of the Live slide, docs/MEDIA.md "Audio playlist"): a 28px header
  like the Schedule's ("Audio Playlist", `+▾` and `⚙▾` 40 × 24 dropdowns with a 2px divider), a four-row track list
  (110px; ▶ / ❚❚ marks the current track, missing files at 45 % text alpha) and a transport on `window.bk1`: title +
  `0:12 / 3:00`, the seek slider, ⏮ (24 × 24), a 30px round ⏯, ⏭, ⏹, repeat (`loop` / `repeat_one`, pressed while
  on), shuffle, volume icon and slider.

### 3.4 Preview / Live content
- Each column has two modes (a hidden tab control): **Slide Selector** (thumbnail grid, the default) and **Outline View** (text outline editor).
- Output monitor (`Frame_LivePreview`): margins 4 all round, a 16:9 letterboxed rectangle on `window.bk1` (`output.surround` `#161616` in the Mac-measured dark palette, §1), with **video controls**
  (position trackbar using the `gutter` gradients, buttons, message line) underneath for media items.
- View → *Side by Side Preview* (default) / *Preview & Live Combined* / *No Preview*. These swap between the
  `TabItem_SideBySide`, `TabItem_Stacked` and `TabItem_Contiguous` layouts.
- **Preview Output context menu** (right-click on the Preview monitor, or Control-click on a Mac): restyles the item in
  Preview on its own. EasyWorship has no popup on `Frame_LivePreview`; it edits an item's look in the presentation
  editor's inspector (`Button_PresentationTheme` "Change Theme", `Button_RevertToGlobal` "Revert to Global Theme",
  the Fill inspector's "Select Media...", `Action_FontSizeInc` / `Dec` "Increase Font" / "Decrease Font"). The menu
  gathers those under their names:
  `Change Theme ▸` (radio: the saved themes, the item's own checked) · `Background ▸` (`Select Media…` (the
  library's images and videos as thumbnails; a video loops behind the text), `Solid Color…`, | `Reset to Theme
  Background`) · `Text ▸` (`Increase Font`, `Decrease Font` (±8 canvas units, the shrink-to floor scales along) |
  `Font Color…` | check: `Bold`, `Italic`, `Outline`, `Shadow` | radio: `Align Left`, `Align Center`, `Align Right` |
  `Reset to Theme Text`) | `Edit Theme for This Item…` (the theme editor on the item's copy: the name is fixed, OK /
  Apply restyle the item, nothing is saved to the library; font family, margins and the rest live here) |
  `Revert to Global Theme` (the library's current version of the theme the item is based on, or the default theme
  for its type when that theme was deleted). The Reset entries and Revert are disabled while the item looks like its
  saved theme; every entry is disabled when Preview holds media, a presentation or nothing.
  - Scope: only that item. A schedule item's own theme copy changes (the schedule turns dirty and the `.fbs` keeps
    it, marked `"custom": true`); library content (a song, a passage) keeps the override in the library deck behind
    the resource preview, so another verse, re-selecting the song, Go Live and Add to Schedule carry it. Editing the
    saved theme later no longer restyles a custom copy. The saved theme is never written.
  - Live is left alone: the output changes when the item is sent live again (the Live monitor's thumbnails and
    output keep the old look). The Live monitor has **no** such menu: a look is fixed while it is on air, so a
    change is previewed first and taken live with Go Live (a crossfade).

### 3.5 Resource tab bar (`SplitterBar_Resources`): height 37
It is also the drag handle for the main splitter (`Cursor = hand`). Clicking it toggles the library open or closed.
- `Resource_TabBar`: height 32, padding-top 2, gradient `tabbar.1`. A 1px divider sits at y=32 (`tabbardivider` stroke + fill).
- Tab buttons (radio group, StaysSelected), height **33**, margins top 1 / bottom −4 so they overlap the divider:
  `Songs` 66w · `Scriptures` 80w · `Media` 60w · `Presentations` 100w · `Themes` 70w.
- After the tabs: two **28 × 27** mini buttons, `Add to Schedule` and `Go Live`, separated by a mini divider.

### 3.6 Resource area (per tab: Songs, Scriptures, Media, Presentations, Themes)
- **Left (Library pane)**: background `libraryselector.bk`. Collections list (Library, My Collections, online sources),
  `SearchLocator` **24px** at the top (embedded search field), and a footer mini-toolbar **28px**
  (`sd.toolbar.mini.tl.style`, padding L8 T3 B1) with `+` (add collection) and a gear dropdown.
- **Center (Item selector)**: a list or grid of items (songs: Title, Author, Copyright… columns with
  `headercolumn.stroke`). Footer mini-toolbar 28px: view sizer (24 + 16 split button) on the left, a loading status, and `+` / `⚙▾` on the right.
- **Search box** (the Songs `SearchLocator`): typing is debounced (~200 ms), Enter searches at once. Every
  whitespace-separated term must match as a word prefix, case- and diacritic-insensitively, in the title, author or
  lyrics; a double-quoted run is one phrase. Results rank title matches first, then author, then songs that needed
  their lyrics, each group by title. A narrow **Match** column says `author` or `lyrics` for the latter rows (the
  lyric excerpt is the row's tooltip) and the footer counts them ("5 songs · 2 matched in lyrics"). The Media and
  Presentations search boxes fold case and diacritics the same way over the title and path.
- **Right (Resource preview)**: 16:9 preview with title, mini video controls (play, volume, navigate), and a 28px toolbar.
- Scriptures tab: translation picker + reference search locator. Media tab: sub-categories Videos / Images / Feeds / Audio / DVD.
- **Scripture locator** (`SearchLocator_Scriptures`, `Mode = slmLocate`: the reference *locates* verses in the
  listed chapters). Typing is debounced like the Songs box; Enter looks up at once.
  - **Book suggestions**: while the book part has no chapter yet, or is ambiguous even with one (`jo 3`), a
    dropdown under the field (drawn over the translations list, PopupMenu panel style, 22 px rows, 8 rows before
    it scrolls) lists every book it may name, in canonical order (`models.BookCompletions`: name, abbreviation or
    alias prefix; without an ordinal also the numbered books by the rest of their name, so `joh` → John, 1 John,
    2 John, 3 John; `1` → the eight books that start with 1). The first is highlighted; ↑/↓ move the highlight,
    Enter / Tab or a click pick it, Esc closes the list until the text changes. Picking completes the book part
    and keeps the rest (`joh` → `John `, `jo 3:16` → `John 3:16`). The list never takes the focus; it closes when
    the field loses it, after a completed book (`John `) and once a chapter follows an unambiguous book.
  - An ambiguous book is not an error: the footer says `Choose a book: Joshua, Job, …`. A book alone shows its
    first chapter (`gen` → Genesis 1; a single-chapter book is the whole book). Text that names no book (`xyz 3`)
    and out-of-range chapters stay red errors.
  - **Verse list**: always whole chapters (every chapter the reference touches), with the referenced verses
    selected and the first of them scrolled to the middle: `john 3` lists 36 verses, all selected; `john 3:16`
    lists chapter 3 with 16 selected; `john 3:16-18` with 16–18 selected.
  - **Multi-select**: click selects one verse, Shift-click the range from the anchor, Cmd-click (Ctrl on
    Windows / Linux) toggles a verse (the last selected verse stays selected), Shift-Cmd-click adds a range,
    ↑/↓/Home/End move (Shift extends), Cmd+A selects all. A plain click inside a multi-verse selection narrows it
    after 400 ms, so a double-click there keeps it. Non-contiguous selections are allowed.
  - The selection is one passage (`models.SelectionPassage`: adjacent rows make a range, a whole chapter prints as
    the chapter, a run into the next chapter as `John 3:36-4:2`): Preview shows exactly the selected verses (one
    verse per slide, from the first), the field shows its reference (`John 3:16-18, 20`), the footer
    `John 3:16-18 · 3 verses (KJV)`. Go Live (button, menu, Enter in the list) sends it live at the verse the
    resource preview shows; a double-click sends it live at the double-clicked verse; Add to Schedule adds it.
- **Preview monitor**: a double-click is Go Live (the Preview deck goes live at the selected slide); single clicks do
  nothing.
- **Media tile menu** (right-click on a tile and the footer `⚙▾`; the same menu, after EasyWorship's video tile menu):
  `Add Media Files…` | `Add to Schedule`, `Go Live` | `Aspect Ratio ▸` (radio: Fill, Fit, Stretch), `Repeating ▸`
  (radio: Loop, Play Once; videos only) | `Use As Logo Background` | `Remove from Library`, `Delete File…` (moves the file
  to the Trash; macOS only, left out elsewhere) | `Update items in Schedule` | `Open File Location` | `View ▸` (radio: Large
  / Medium / Small Thumbnails, Details), `Sort by ▸` (radio: Name, Date Added, Kind), `Refresh`. Aspect Ratio and
  Repeating are stored per item (`media.aspect` / `media.repeat`) and travel with the slide (`Background.Fit` /
  `Background.Loop`): Fill covers the canvas, Fit letterboxes on black, Stretch ignores the aspect ratio. Schedule
  items keep their snapshot until `Update items in Schedule`. View and Sort by are per tab and persisted
  (`ui.media.view`, `ui.media.sort`); Details is a list of rows with a 64 px thumbnail and the title. The footer's
  `[▤|▾]` sizer offers the same views. Refresh re-checks every file (missing ones are removed after a question) and
  re-derives missing thumbnails. Not offered (no counterpart yet): Edit Video Properties…, Media Usage ▸, Copy to Theme ▸.
- **Presentation tile menu** (same places): `Add Presentation…` | `Add to Schedule`, `Go Live` | `Use As Logo Background`
  (page 1; enabled once rendered) | `Remove from Library`, `Delete File…` | `Open File Location` | `View ▸`, `Sort by ▸`
  (Kind = PDF / PowerPoint / Keynote / OpenDocument), `Refresh`. Not offered: New / Edit Presentation… (no editor),
  Import PowerPoint File… (Add Presentation… takes every type), Export Presentation File… (no native format), Update
  items in Schedule (schedule items re-resolve their pages from the library by path).
- Both menus are control actions: `media-menu {item, index|title, confirm}` with `add-to-schedule`, `go-live`,
  `aspect-fill|fit|stretch`, `repeat-loop|once`, `logo-background`, `remove`, `delete-file`, `update-schedule`,
  `reveal`, `view-large|medium|small|details`, `sort-name|date|kind`, `refresh`; `presentation-menu` the same minus the
  aspect / repeat / schedule-update items. `/state.media` reports `aspect`, `repeat`, `view`, `sort`;
  `/state.presentations` reports `view`, `sort`.

### 3.7 Default hotkeys (from `HotKeys.2.ini`)
| Action | Keys |
|---|---|
| Next slide | → , Space, N |
| Previous slide | ← , Backspace, P |
| First / last slide | Home / End |
| Next / prev schedule item | ↓ / ↑ |
| Go live next / prev | PgDn / PgUp |
| Black / Clear / Logo | Ctrl+B / Ctrl+C / Ctrl+L (on a Mac both ⌘ and Control: ⌘B and ⌃B; FreeBeams tells them apart) |
| Jump to Verse / Chorus / Bridge / Pre-chorus / Tag / Intro / Ending / Slide | V / C / B / R / T / I / E / S (then number) |

## 4. Menus (macOS)
- **App**: About · Settings…
- **File**: New ▸ (Schedule, Song, Presentation, —, Song from SongSelect) · Open Schedule · Open Recent · — · Save Schedule · Save Schedule As · Export Packed Schedule · Close Schedule · — · Reports and Printing ▸ (Print Schedule Summary/Details, —, Song Activity Report, Song List Report, —, Save Schedule Report Summary/Details, —, Save Song Activity/List Report) · Exit
- **Edit**: Cut · Copy · Paste · Delete · Select All · — · Options…
- **Live**: Go Live · Go Back · — · Show Logo · Black Output · Clear Output · — · Show Live
- **Profiles**: (profile list) · — · Profiles Manager · — · Add/Remove Demo Data · — · Utilities ▸ (Reset Songs to Default Theme, Rebuild Search Keys, Clear Thumbnails)
- **View**: Side by Side Preview · Preview & Live Combined · No Preview · — · Interface Color ▸ (Light, Dark)
- **Help**: Contents · Show Onboarding · — · Troubleshooting Logs ▸ · — · Check for Updates · — · Helper · — · Register Products · About

## 5. Secondary windows
- **Presentation / Song Editor** (1020 × 528):
  - 58px toolbar. Left: title field, and under it Import / Print / Undo▾ / SpellCheck▾. Then dividers and 60 × 53 buttons: Preview | Theme, Text, Scripture, Media, Shape | (right side) Animate, Presentation (85w) | Format.
  - Body: left pane (27.1%) with a 34px tab bar `Words | Slides …… Layouts`, a center designer canvas (`preseditor` bg) with a 29px text-format toolbar on top and a 28px bottom bar (Insert, Zoom 194w, checkerboard toggle), and a right **Inspector 280w** that slides in and out.
  - Footer: "Apply changes to items in schedule" checkbox (left), with Apply / OK / Cancel buttons (75 × 25) on the right.
- **Options** (712 × 532): left menu 188w with 32px rows: Main Output, Alternate Output, Foldback, Service Intervals | Slide Labels, Remotes | Advanced (10px spacers between groups). A 16:9 output preview (146h) sits at the bottom left, with a 2px vertical line and the content on the right. Footer: "Preview Output" checkbox, OK / Cancel.
  - Output options frame: a 34px tab bar `General | Song | Scripture | Presentations | Transitions | Alerts | Alerts and Clocks`. Content is stacked **Inspector Cards** (titled cards: Select Output Monitor, Screen Margins, Options, Song Layouts, Song Copyright, Scripture Layout, Message/Nursery alert, Clock, Service Interval, Next Item…).
  - The Transitions tab has a centered 300w selector: Slide / Black / Clear / Logo.
- **Live Output** window: full-screen black `TRectangle`, with screen-margin layouts (top/left/right/bottom) around the output area. Layers: Black panel, PowerPoint host, web browser, slide.

## 6. Godot implementation mapping
| EasyWorship | Godot (graphics.gd) |
|---|---|
| `TsdSplitterPanel` (percent, min, snap) | custom `Splitter` Go extension on `Container` (Godot's `SplitContainer` has no percent/snap-close), with the percentage persisted |
| `TToolBar` gradient | `PanelContainer` + `StyleBoxTexture` from a 1×N `GradientTexture2D` |
| `TsdSpeedButton` (icon top + label, StaysSelected, group) | `Button` with `IconAlignment = top`, `ToggleMode`, a `ButtonGroup`, and theme styleboxes normal/hover/pressed |
| split view-sizer button (24 + 16) | two adjacent Buttons with left/right-rounded styleboxes |
| `TsdSearchLocator` | `LineEdit` with a search icon and clear button, embedded stylebox |
| `TsdSlideSelector` (thumbnail grid) | custom `Control` drawing thumbnails from SubViewport textures, selection border `highlight` |
| `TsdScheduleSelector`, song list | `Tree` with hidden root and columns, row selection `highlight.2` / `highlight` |
| Inspector cards | `PanelContainer` + title `Label` + content `VBoxContainer` |
| Interface color Light/Dark | two `Theme` resources built in Go from the palette table above, swapped at runtime |
