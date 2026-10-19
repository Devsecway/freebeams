# Icons

FreeBeams draws its own icons (no EasyWorship artwork, no third-party icon packs). They are
hand-written SVG bodies in `internal/ui/uicore/icons.go`, rasterised by `internal/ui/icons.go` as
Godot `DPITexture`s and stored in the Theme (`internal/ui/theme.go`, `setIcons`). Screenshots:

- `docs/screens/icons-{before,after}-{toolbar,panels}-{dark,light}.png`: the Phase 1 line-art set
  and its first redesign (the audit below).
- `docs/screens/icons-ew-after-toolbar-{dark,light}.png`: the current toolbar, drawn after
  EasyWorship's Mac toolbar (filled screens), with Live off (top row) and Live + Black on (bottom).

## Design: the toolbar reads like EasyWorship's Mac toolbar

EasyWorship's output buttons are small *pictures of a screen*: a filled light-grey screen with a
thin darker frame whose content tells the function. Ours use the same metaphors, drawn on our grid:

| Button | Metaphor | Artwork (`icons.go`) |
|---|---|---|
| Go Live | a green screen with a white arrow and a small dot on the left | `golive` (toolbar), `golive16` (tab bar) |
| Alerts | a blue speech bubble with a signal glyph (arc + dot) | `alerts` |
| Logo | a light screen showing a picture: hills and a sun | `logo` |
| Black | a light screen whose lower-left half is black along the diagonal | `black` |
| Clear | a light screen whose content lines are being peeled off at the corner | `clear` |
| Live | a screen of seven colour bars: grey while the output is off, SMPTE colours while it is on | `live`, `live_on` |
| New | a page with a folded corner, a text line and a green plus badge | `new` |
| Open | a folder (back at 55 %, front filled) | `open` |
| Save | a floppy with its shutter and label cut out | `save` |
| Web | a green globe with white meridians | `web` |
| Remote | signal arcs around a dot | `remote` |

How the filled style works with one glyph color:

- A **screen** (`rim32` + `screen32`) is a 24 × 18 rounded rectangle: the 2 px rim is the glyph color
  at 55 % and the 20 × 14 interior the glyph color at 85 %, so on the toolbar the rim reads darker
  than the screen, as EasyWorship's frame does. Holes in the interior (the picture, the content
  lines, the peeled corner) are **evenodd subpaths** of the interior path: they show the button's own
  background in every state (toolbar gradient, `toolbutton_sel` when active), no second color
  needed. The same goes for the page, folder and floppy.
- Semi-transparent fills **compound** when they overlap (a 50 % flap over an 85 % screen is 92 %,
  not 50 %), so a lighter part is never painted over the base: the region is cut out of the base
  path and the part is painted onto the hole (Clear's fold, New's badge ring).
- A hole subpath must lie **inside** its outer shape: with evenodd, the part of a hole outside the
  shape is inside exactly one subpath and gets filled. New's badge ring sits fully inside the page.
- Black's black half is `{k}` and stays black under any tint; Live's bottom band is `{k}` too.
- The colored icons (Go Live, Alerts, Live on, New's badge, Web) carry palette colors of their own
  and are drawn untinted; see Colors below.

## Audit of the Phase 1 set (what the first redesign fixed)

Captured in `docs/screens/icons-before-*.png` (2× Retina, dark and light, Live off / Live + Black on):

- **Two unrelated stroke weights.** Toolbar icons were drawn at 1.7 px on a 32 grid with `.5`
  coordinates, mini icons at 1.35 px on a 16 grid; neither lands on device pixels at 1× or 2×, so
  horizontal and vertical lines rendered as soft 2–3 px smears instead of solid lines.
- **No shared geometry.** Five output icons used three different screens (with/without stand,
  different radii and sizes); the floppy's inner rectangle had square corners while everything else was
  rounded; the gear's teeth were too narrow for its stroke and read as a blob at 1×.
- **State was not in the icons.** Live showed its red dot whether the output was on or off; Logo,
  Black and Clear changed nothing but their background when active; Go Live had no accent, so the
  one button operators reach for most looked like every other button.
- **Black was a light grey slab** (glyph color at 85 %), which reads as "white screen" on the dark
  toolbar.
- **Logo was a screen with a plus** (the New metaphor), not a picture.
- **The tab bar's Go Live** mini button drew the 32 px toolbar texture squeezed to 16 px (the name
  lookup found the toolbar icon first), so it was blurrier than its neighbour.
- **Dropdown arrows** were non-anti-aliased polygons (`DrawColoredPolygon`), jagged at 1×.
- **Half-pixel positions.** A 16 px icon centred in a 27 px button (the tab bar) was drawn at
  y = 5.5, resampling the texture.
- **Hard-coded glyph colors** (`#DCDCDC` / `#3A3A3A`) in `theme.go` instead of palette-derived tones,
  and the glyph tone was baked into every texture, so a different tint per state needed a new
  texture.
- **The Media, Themes and Presentations library lists showed Godot's red fallback icon** instead
  of `library` / `video` / `image` / `theme` / `presentation`. Those panels built their Tree in the
  constructor (before entering the themed tree, so `GetThemeIcon` cached the fallback) and rebuilt it
  inside the `theme_changed` handler, which Godot emits *before* invalidating that cache. Fixed with
  `onThemeReady` (see below); Songs only looked up in the handler and happened to work.

The second pass (`icons-ew-after-*`) replaced the 2 px line-art screens of that redesign with the
filled screens above, after the user's EasyWorship Mac screenshots: line art read as a different
product next to EasyWorship's pictures-of-a-screen, and Live's red dot said nothing about colour bars.

## Grids and strokes

| Set | Canvas | Live area | Stroke | Coordinates of straight edges | Corner radius | Used by |
|---|---|---|---|---|---|---|
| Toolbar | 32 × 32 | 24 × 24 (4 px padding) | filled shapes; 2 px, round caps and joins where stroked | **integers** | 2 (`rx="2"`) | `toolButton` (56 × 54 buttons of the main toolbar) |
| Mini | 16 × 16 | 12 × 12 (2 px padding) | 1.5 px, round caps and joins | **.25 / .75** | 1 | `miniButton` (panel headers and footers, tab bar), Tree items, the search field |
| Arrow | 8 × 8 | 6.5 × 3.75 triangle | filled, no stroke | – | – | the `▾` of dropdown buttons (`drawArrow`) |

Why the coordinate rule: the operator UI runs at the window's content scale (2× on Retina, 1× on an
external 1080p monitor). A fill edge or a 2 px stroke centred on an integer coordinate covers whole
pixels at both scales; a 1.5 px stroke centred on .25/.75 covers exactly three device pixels at 2×.
Diagonals and curves are anti-aliased either way. `TestIconGrid` in `uicore/icons_test.go` enforces
the rule for `h`/`v` path segments (including hole subpaths) and `<rect>` edges. Live's seven bars are
3, 3, 3, 2, 3, 3, 3 px wide so they tile the 20 px interior on whole pixels (`TestLiveBars`).

Shared parts: the output icons are one family built from `rim32` (the screen's rim), `screen32(holes)`
(rim + interior with holes: Logo, Black, Clear) and `bars32(fills)` (rim + bars: Live).

## Colors

The SVG bodies use placeholders, resolved by `uicore.IconSVG`:

| Placeholder | Meaning | Value |
|---|---|---|
| `{c}` | glyph (strokes, single-color fills, the screens) | the color the texture is rasterised with (see below) |
| `{a}` | accent | `uicore.AccentColor`: `highlight.hot` (dark) / `highlight` (light) |
| `{r}` | live red | palette `red` |
| `{g}` | green | palette `icon.green` (`#5BB55B` dark / `#4FA64F` light): the Go Live screen, the Web globe, New's badge |
| `{b}` | blue | palette `icon.blue` (`#4B8CE9` dark / `#4A8CE0` light): the Alerts bubble |
| `{s1}`…`{s7}` | the SMPTE bars | `uicore.SMPTEBars` (white, yellow, cyan, green, magenta, red, blue) |
| `{k}` | black | `#000000` (Black's dark half, Live's bottom band; black under any tint) |
| `{w}` | white | `#FFFFFF` (the arrow, dot, plus and signal glyphs on colored shapes) |

An icon that uses anything but `{c}` and `{k}` is **multicolor** (`IconSpec.Multi`): `golive`,
`golive16`, `alerts`, `live_on`, `new`, `web`. Everything else is **single-color**.
`uicore.PaletteIconColors(p)` is the one place that maps a palette to `IconColors`.

Each icon is put into the Theme twice:

- `FwIcons`: rasterised in palette colors (`paletteIconColors`). Built-in controls use these (Tree
  item icons: `library`, `bible`, `video`, `image`, `presentation`, `theme`), and so do the
  multicolor icons, which are drawn untinted (only their alpha follows the button state).
- `FwIconMasks`: single-color icons rasterised **white**. Custom-drawn widgets draw them through
  `drawGlyph` with a modulate color, so every button state gets a palette tint without more textures.
  The screens' 55 % / 85 % opacities are alpha in the mask, so they hold under any tint.

Tints (theme colors of type `Fw`, set in `setPaletteColors`; never hard-code hex in widgets):

| Theme color | Dark | Light | Used for |
|---|---|---|---|
| `glyph` | `text` blended 60 % toward `text.semibold` (`#DADADA`) | `text` (`#2A2A2A`) | icons at rest (the light screens) |
| `glyph_hot` | `text.semibold` | `text.semibold` | icon under the pointer |
| `glyph_on` | `text.semibold` | `text.semibold` | an active toggle (Logo, Black, Clear) over its `toolbutton_sel` background |
| `toolbutton_text_sel` | `text.semibold` | `text.semibold` | the label of an active toggle |
| `accent` | `highlight.hot` | `highlight` | (available to icons through `{a}`) |
| `red` | `#FF0000` | `#FF0000` | `{r}` |
| (disabled) | `glyph` at 35 % alpha; multicolor icons at 35 % alpha | same | Web, Remote, Alerts |

In the light interface the single-color icons are dark filled shapes with light cut-outs (macOS
template-icon style); the colored ones are the same green and blue as in the dark interface.

State-bearing toolbar buttons:

- **Live** is a toggle with its own "on" artwork: `live` (grey bars, tinted like any single-color
  icon) and `live_on` (SMPTE colour bars, multicolor). `toolButton.Draw` uses `<icon>_on` whenever
  the button is pressed and that name exists.
- **Logo, Black, Clear** have no `_on` artwork; when active their screens draw in `glyph_on` with the
  label in `toolbutton_text_sel`, over the `toolbutton_sel` background.
- **Go Live** (toolbar and the tab bar's `golive16`) is green at all times; **Alerts** is blue,
  dimmed to 35 % while the feature is disabled.

## Rendering

`DPITexture` keeps the SVG source and re-rasterises it for the oversampling of the viewport that
draws it, so a 32 px icon is a 64 × 64 bitmap on a 2× window and 32 × 32 at 1×, never a scaled
bitmap. `drawIcon` floors the draw position: a half-pixel offset (e.g. a 16 px icon centred in a
27 px button) would resample the texture and blur it.

## Adding an icon

1. Draw it on the right grid (see above) and add it to the `icons` table in
   `internal/ui/uicore/icons.go`: `"name": {16, `<path .../>`}`. Use `{c}` for everything that
   should take the tint, cut details out of fills as evenodd holes, and use the other placeholders
   only when the icon must carry that color itself (it then becomes multicolor and is drawn
   untinted). Give a toggle's active artwork the name `<name>_on`. A screen-type output icon should
   reuse `screen32(holes)`.
2. `go test ./internal/ui/uicore/` checks the SVG is well-formed, has no unresolved placeholders and
   keeps its straight edges on the grid.
3. Preview without the app: `FW_ICON_DUMP=/tmp/fw-icons go test ./internal/ui/uicore/ -run Dump`
   writes every icon as an SVG in the dark palette colors; open them in any SVG viewer or browser at
   1×, 2× and 4×. Without a browser, `qlmanage -t -s 128 -o <dir> <file>.svg` rasterises one through
   QuickLook; give the SVG `width`/`height` of 128 (keeping the `viewBox`) first, since QuickLook does
   not scale the intrinsic size, and add a toolbar-grey `<rect>` behind the body to judge the opacities.
4. Use it: `newToolButton(text, "name", …)`, `newMiniButton(kind, "name", …)`, or
   `drawGlyph(ci, control, "name", pos, size, tint)` in a custom `Draw`. Built-in controls take
   `fwIcon(control, "name")` (palette colors). The Theme is rebuilt for a palette switch, so nothing
   else is needed for Light/Dark, **except** for textures a control stores (Tree item icons): set
   those from an `onThemeReady(control, fn)` callback (`widgets.go`), never from a plain
   `OnThemeChanged` handler or a constructor, or you get the fallback icon / the old palette's
   texture (see the audit).
