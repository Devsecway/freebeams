# Icons

FreeWorship draws its own line icons (no EasyWorship artwork, no third-party icon packs). They are
hand-written SVG bodies in `internal/ui/uicore/icons.go`, rasterised by `internal/ui/icons.go` as
Godot `DPITexture`s and stored in the Theme (`internal/ui/theme.go`, `setIcons`). Before/after
screenshots: `docs/screens/icons-{before,after}-{toolbar,panels}-{dark,light}.png`.

## Audit of the Phase 1 set (what the redesign fixed)

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

## Grids and strokes

| Set | Canvas | Live area | Stroke | Coordinates of straight lines | Corner radius | Used by |
|---|---|---|---|---|---|---|
| Toolbar | 32 × 32 | 24 × 24 (4 px padding) | 2 px, round caps and joins | **integers** | 2 (`rx="2"`) | `toolButton` (56 × 54 buttons of the main toolbar) |
| Mini | 16 × 16 | 12 × 12 (2 px padding) | 1.5 px, round caps and joins | **.25 / .75** | 1 | `miniButton` (panel headers and footers, tab bar), Tree items, the search field |
| Arrow | 8 × 8 | 6.5 × 3.75 triangle | filled, no stroke | – | – | the `▾` of dropdown buttons (`drawArrow`) |

Why the coordinate rule: the operator UI runs at the window's content scale (2× on Retina, 1× on an
external 1080p monitor). A 2 px stroke centred on an integer coordinate covers whole pixels at both
scales; a 1.5 px stroke centred on .25/.75 covers exactly three device pixels at 2×. Diagonals and
curves are anti-aliased either way. `TestIconGrid` in `uicore/icons_test.go` enforces the rule for
`h`/`v` path segments and `<rect>` edges.

Shared parts: the output icons are one family. Go Live and Live use `monitor32` (screen on a stand);
Logo, Black and Clear use `screen32` (a plain 24 × 18 screen).

## Colors

The SVG bodies use placeholders, resolved by `uicore.IconSVG`:

| Placeholder | Meaning | Value |
|---|---|---|
| `{c}` | glyph (strokes, single-color fills) | the color the texture is rasterised with (see below) |
| `{a}` | accent | `uicore.AccentColor`: `highlight.hot` (dark) / `highlight` (light) |
| `{r}` | live red | palette `red` |
| `{k}` | black | `#000000` (the Black screen's interior; it stays black under any tint) |

An icon that uses `{a}` or `{r}` is **multicolor** (`IconSpec.Multi`): `golive`, `golive16`,
`live_on`. Everything else is **single-color**.

Each icon is put into the Theme twice:

- `FwIcons`: rasterised in palette colors (`paletteIconColors`). Built-in controls use these (Tree
  item icons: `library`, `bible`, `video`, `image`, `presentation`, `theme`), and so do the
  multicolor icons, which are drawn untinted (only their alpha follows the button state).
- `FwIconMasks`: single-color icons rasterised **white**. Custom-drawn widgets draw them through
  `drawGlyph` with a modulate color, so every button state gets a palette tint without more textures.

Tints (theme colors of type `Fw`, set in `setPaletteColors`; never hard-code hex in widgets):

| Theme color | Dark | Light | Used for |
|---|---|---|---|
| `glyph` | `text` blended 60 % toward `text.semibold` (`#DADADA`) | `text` (`#2A2A2A`) | icons at rest |
| `glyph_hot` | `text.semibold` | `text.semibold` | icon under the pointer |
| `glyph_on` | `text.semibold` | `text.semibold` | an active toggle (Logo, Black, Clear) over its `toolbutton_sel` background |
| `toolbutton_text_sel` | `text.semibold` | `text.semibold` | the label of an active toggle |
| `accent` | `highlight.hot` | `highlight` | the Go Live play glyph |
| `red` | `#FF0000` | `#FF0000` | the Live indicator while the output is on |
| (disabled) | `glyph` at 35 % alpha | same | Web, Remote, Alerts |

State-bearing toolbar buttons:

- **Live** is a toggle with its own "on" artwork: the set has `live` (hollow indicator) and `live_on`
  (filled red). `toolButton.Draw` uses `<icon>_on` whenever the button is pressed and that name exists.
- **Logo, Black, Clear** have no `_on` artwork; when active they draw in `glyph_on` with the label in
  `toolbutton_text_sel`, over the `toolbutton_sel` background.
- **Go Live** (toolbar and the tab bar's `golive16`) carries the accent at all times.

## Rendering

`DPITexture` keeps the SVG source and re-rasterises it for the oversampling of the viewport that
draws it, so a 32 px icon is a 64 × 64 bitmap on a 2× window and 32 × 32 at 1×, never a scaled
bitmap. `drawIcon` floors the draw position: a half-pixel offset (e.g. a 16 px icon centred in a
27 px button) would resample the texture and blur it.

## Adding an icon

1. Draw it on the right grid (see above) and add it to the `icons` table in
   `internal/ui/uicore/icons.go`: `"name": {16, `<path .../>`}`. Use `{c}` for everything that
   should take the tint; use `{a}` / `{r}` only when the icon must carry that color itself (it then
   becomes multicolor and is drawn untinted). Give a toggle's active artwork the name `<name>_on`.
2. `go test ./internal/ui/uicore/` checks the SVG is well-formed, has no unresolved placeholders and
   keeps its straight lines on the grid.
3. Preview without the app: `FW_ICON_DUMP=/tmp/fw-icons go test ./internal/ui/uicore/ -run Dump`
   writes every icon as an SVG in the dark palette colors; open them in any SVG viewer or browser at
   1×, 2× and 4×.
4. Use it: `newToolButton(text, "name", …)`, `newMiniButton(kind, "name", …)`, or
   `drawGlyph(ci, control, "name", pos, size, tint)` in a custom `Draw`. Built-in controls take
   `fwIcon(control, "name")` (palette colors). The Theme is rebuilt for a palette switch, so nothing
   else is needed for Light/Dark.
