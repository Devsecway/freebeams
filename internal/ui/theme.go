package ui

import (
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Font"
	"graphics.gd/classdb/Image"
	"graphics.gd/classdb/ImageTexture"
	"graphics.gd/classdb/StyleBox"
	"graphics.gd/classdb/StyleBoxEmpty"
	"graphics.gd/classdb/StyleBoxFlat"
	"graphics.gd/classdb/StyleBoxLine"
	"graphics.gd/classdb/StyleBoxTexture"
	"graphics.gd/classdb/SystemFont"
	"graphics.gd/classdb/Theme"
	"graphics.gd/variant/Color"
)

// Palette is the EasyWorship interface palette (docs/UI-SPEC.md §1).
type Palette = uicore.Palette

// DarkPalette is the default (Dark) interface color.
func DarkPalette() Palette { return uicore.DarkPalette() }

// LightPalette is the Light interface color.
func LightPalette() Palette { return uicore.LightPalette() }

// Theme type names. Standard controls are styled under their class names;
// custom-drawn FreeBeams widgets read palette colors, fonts and icons from
// these pseudo types so that swapping the Theme (Light/Dark) restyles them too.
const (
	tFw    = "Fw"          // palette colors, fonts, shared styleboxes
	tIcons = "FwIcons"     // icon textures in palette colors (Tree items, multicolor icons)
	tMasks = "FwIconMasks" // single-color icon textures in white, tinted when drawn
)

// Type variations (Control.SetThemeTypeVariation).
const (
	vToolbar     = "FwToolbar"     // PanelContainer: 58px main toolbar gradient
	vMiniBar     = "FwMiniBar"     // PanelContainer: 28px header (toolbar.mini, bottom border.bl)
	vMiniBarTL   = "FwMiniBarTL"   // PanelContainer: 28px footer (toolbar.mini, top border.tl)
	vBk0         = "FwBk0"         // PanelContainer: window.bk0
	vBk1         = "FwBk1"         // PanelContainer: window.bk1
	vBk2         = "FwBk2"         // PanelContainer: window.bk2
	vLibrary     = "FwLibrary"     // PanelContainer: libraryselector.bk
	vTitle       = "FwTitle"       // Label: label.semibold (14pt semibold, 90%)
	vCaption     = "FwCaption"     // Label: 11pt caption
	vDim         = "FwDim"         // Label: dimmed text
	vPlaceholder = "FwPlaceholder" // Label: centered "Coming in Phase 2"
	vError       = "FwError"       // Label: red error text
	vSearch      = "FwSearch"      // LineEdit: embedded search locator field
	vSearchPanel = "FwSearchPanel" // PanelContainer: 24px search locator strip
	vLibraryTree = "FwLibraryTree" // Tree: library (collections) list
	vHint        = "FwHint"        // Label: small dim hint line
	vResTitle    = "FwResTitle"    // Label: resource preview title
	vScreen      = "FwScreen"      // PanelContainer: black output screen with 1px frame
	vThumb       = "FwThumb"       // PanelContainer: black thumbnail backing

	// Surfaces with Mac-measured tokens of their own (UI-SPEC §1).
	vGridBk   = "FwGridBk"   // PanelContainer: Preview / Live slide grid body (grid.bk)
	vOutput   = "FwOutput"   // PanelContainer: output monitor surround (output.surround)
	vResource = "FwResource" // PanelContainer: resource area surfaces (resource.bk)
)

// Base font sizes (UI-SPEC §2).
const (
	fontBase    = 13
	fontTitle   = 14
	fontCaption = 11
)

// BuildTheme builds the Godot Theme for every control from the palette.
func BuildTheme(p Palette) Theme.Instance {
	th := Theme.New()
	regular := systemFont(400)
	semibold := systemFont(600)
	th.SetDefaultFont(regular)
	th.SetDefaultFontSize(fontBase)

	setPaletteColors(th, p)
	th.SetFont("regular", tFw, regular)
	th.SetFont("semibold", tFw, semibold)
	th.SetFont("italic", tFw, systemFontStyle(400, true))
	th.SetFont("mono", tFw, monoFont())
	setIcons(th, p)

	// Panels.
	for _, t := range []string{"Panel", "PanelContainer"} {
		th.SetStylebox("panel", t, flat(p.Bk2).AsStyleBox())
	}
	panelVariation(th, vBk0, flat(p.Bk0).AsStyleBox())
	panelVariation(th, vBk1, flat(p.Bk1).AsStyleBox())
	panelVariation(th, vBk2, flat(p.Bk2).AsStyleBox())
	panelVariation(th, vLibrary, flat(p.LibraryBk).AsStyleBox())
	panelVariation(th, vGridBk, flat(p.GridBk).AsStyleBox())
	panelVariation(th, vOutput, flat(p.OutputSurround).AsStyleBox())
	panelVariation(th, vResource, flat(p.ResourceBk).AsStyleBox())
	screen := flat(uicore.Hex(0x000000))
	screen.SetBorderWidthAll(1)
	screen.SetBorderColor(uicore.Hex(0x000000))
	screen.SetExpandMarginAll(1)
	panelVariation(th, vScreen, screen.AsStyleBox())
	panelVariation(th, vThumb, flat(uicore.Hex(0x000000)).AsStyleBox())

	toolbar := gradBox(p.Toolbar, 58, uicore.Edge{}, uicore.Edge{Width: 1, Color: p.SplitterBar}, uicore.Edge{}, uicore.Edge{})
	margins(toolbar.AsStyleBox(), 6, 2, 0, 2)
	panelVariation(th, vToolbar, toolbar.AsStyleBox())

	mini := gradBox(p.ToolbarMini, 28, uicore.Edge{}, uicore.Edge{Width: 1, Color: p.BorderBL}, uicore.Edge{}, uicore.Edge{})
	margins(mini.AsStyleBox(), 6, 1, 4, 3)
	panelVariation(th, vMiniBar, mini.AsStyleBox())

	// Footers take the footer gradient (flat resource.bk on the Mac) instead
	// of toolbar.mini.
	miniTL := gradBox(p.Footer, 28, uicore.Edge{Width: 1, Color: p.BorderTL}, uicore.Edge{}, uicore.Edge{}, uicore.Edge{})
	margins(miniTL.AsStyleBox(), 8, 3, 8, 1)
	panelVariation(th, vMiniBarTL, miniTL.AsStyleBox())
	th.SetStylebox("tabbar1", tFw, gradBox(p.ResourceTabBar, 32, uicore.Edge{}, uicore.Edge{}, uicore.Edge{}, uicore.Edge{}).AsStyleBox())
	th.SetStylebox("hsplit_closed", tFw, gradBox(p.HSplitterClosed, 8, uicore.Edge{}, uicore.Edge{}, uicore.Edge{}, uicore.Edge{}).AsStyleBox())

	// Labels.
	th.SetColor("font_color", "Label", p.Text)
	labelVariation(th, vTitle, semibold, fontTitle, uicore.WithAlpha(p.TextSemibold, 0.9))
	labelVariation(th, vCaption, regular, fontCaption, p.Text)
	labelVariation(th, vDim, regular, fontBase, uicore.WithAlpha(p.Text, 0.7))
	labelVariation(th, vPlaceholder, regular, fontTitle, uicore.WithAlpha(p.Text, 0.5))
	labelVariation(th, vError, regular, fontCaption, p.Red)
	labelVariation(th, vResTitle, regular, 12, p.TextSemibold)
	resTitle := StyleBoxEmpty.New()
	margins(resTitle.AsStyleBox(), 8, 2, 8, 2)
	th.SetStylebox("normal", vResTitle, resTitle.AsStyleBox())
	labelVariation(th, vHint, regular, fontCaption, uicore.WithAlpha(p.Text, 0.75))

	buildWidgetBoxes(th, p)
	buildButtonTheme(th, p)
	buildEditTheme(th, p)
	buildTreeTheme(th, p)
	buildScrollTheme(th, p)
	buildMenuTheme(th, p)
	buildSliderTheme(th, p) // the transport bars' sliders (transport.go)

	// Tooltips.
	tip := flat(p.MenuFill)
	tip.SetBorderWidthAll(1)
	tip.SetBorderColor(p.MenuStroke)
	margins(tip.AsStyleBox(), 6, 3, 6, 3)
	th.SetStylebox("panel", "TooltipPanel", tip.AsStyleBox())
	th.SetColor("font_color", "TooltipLabel", p.Text)
	th.SetFontSize("font_size", "TooltipLabel", 12)

	// CheckBox (dialog options).
	empty := StyleBoxEmpty.New().AsStyleBox()
	for _, s := range []string{"normal", "hover", "pressed", "hover_pressed", "focus", "disabled"} {
		th.SetStylebox(s, "CheckBox", empty)
	}
	for _, s := range []string{"font_color", "font_hover_color", "font_pressed_color", "font_hover_pressed_color", "font_focus_color"} {
		th.SetColor(s, "CheckBox", p.Text)
	}
	return th
}

func setPaletteColors(th Theme.Instance, p Palette) {
	colors := map[string]Color.RGBA{
		"bk0": p.Bk0, "bk1": p.Bk1, "bk2": p.Bk2, "bk3": p.Bk3,
		"window_border":   p.WindowBorder,
		"text":            p.Text,
		"text_semibold":   p.TextSemibold,
		"hyperlink":       p.TextHyperlink,
		"highlight":       p.Highlight,
		"highlight_hot":   p.HighlightHot,
		"highlight_text":  p.HighlightText,
		"highlight_inact": p.HighlightInactive,
		"highlight2":      p.Highlight2,
		"highlight2_hot":  p.Highlight2Hot,
		"highlight2_text": p.Highlight2Text,
		"splitter":        p.SplitterBar,
		"border":          p.Border,
		"border_bl":       p.BorderBL,
		"border_tl":       p.BorderTL,
		"selectorline":    p.SelectorLine,
		"header_stroke":   p.HeaderColumnStroke,
		"library_bk":      p.LibraryBk,
		"div_lead":        p.DividerLeading,
		"div_trail":       p.DividerTrailing,
		"divmini_lead":    p.DividerMiniLeading,
		"divmini_trail":   p.DividerMiniTrailing,
		"tab_div_stroke":  p.TabDividerStroke,
		"tab_div_fill":    p.TabDivider, // tab.divider (Mac); tabbardivider.fill in the Windows style
		"red":             p.Red,
		"thumb_bg":        uicore.Hex(0x000000),
		"monitor_border":  uicore.Hex(0x000000),
		"tab_text":        p.Text,
		"tab_text_sel":    p.TextSemibold,
		"toolbutton_text": p.Text,
		// Icon tints (docs/ICONS.md): glyph at rest, glyph_hot under the
		// pointer, glyph_on for an active toggle (Logo, Black, Clear) and
		// its label; accent is the Go Live play glyph; disabled icons keep
		// the glyph tint at 35% alpha.
		"glyph":               uicore.GlyphColor(p),
		"glyph_hot":           p.TextSemibold,
		"glyph_on":            p.TextSemibold,
		"accent":              uicore.AccentColor(p),
		"toolbutton_text_sel": p.TextSemibold,
		"inactive_overlay":    p.InactiveOverlay,
		"grip_a":              p.HSplitterClosed.Sample(0),
		"grip_b":              p.HSplitterClosed.Sample(1),
		"vgrip_a":             p.VSplitterClosed.Sample(0),
		"vgrip_b":             p.VSplitterClosed.Sample(1),

		// Mac-measured surfaces (UI-SPEC §1): the selected tile's frame and
		// caption bar (with highlight.text on both), the selected tab, the
		// unfocused list selection and the schedule cards.
		"grid_bk":           p.GridBk,
		"output_surround":   p.OutputSurround,
		"resource_bk":       p.ResourceBk,
		"card_bk":           p.CardBk,
		"tab_sel":           p.TabSel,
		"tile_frame":        p.TileFrame,
		"tile_caption":      p.TileCaption,
		"tile_text_sel":     p.HighlightText,
		"list_sel_inactive": p.ListSelInactive,
	}
	for name, c := range colors {
		th.SetColor(name, tFw, c)
	}
}

// setIcons rasterises the icon set (see icons.go): every icon in palette
// colors under FwIcons, and every single-color icon in white under
// FwIconMasks for tinting at draw time.
func setIcons(th Theme.Instance, p Palette) {
	colors, masks := paletteIconColors(p), maskIconColors()
	for _, name := range uicore.IconNames() {
		if tex, ok := rasterIcon(name, colors); ok {
			th.SetIcon(name, tIcons, tex)
		}
		if iconIsMulti(name) {
			continue
		}
		if tex, ok := rasterIcon(name, masks); ok {
			th.SetIcon(name, tMasks, tex)
		}
	}
	// LineEdit clear button (white glyph, tinted by clear_button_color).
	if tex, ok := rasterIcon("clear_field", masks); ok {
		th.SetIcon("clear", "LineEdit", tex)
	}
	// Tree folding arrows, in the text color.
	arrows := uicore.IconColors{Glyph: p.Text, Accent: p.Text, Red: p.Text}
	if tex, ok := rasterIcon("chev_down", arrows); ok {
		th.SetIcon("arrow", "Tree", tex)
	}
	if tex, ok := rasterIcon("chev_right", arrows); ok {
		th.SetIcon("arrow_collapsed", "Tree", tex)
		th.SetIcon("arrow_collapsed_mirrored", "Tree", tex)
	}
}

// buildWidgetBoxes stores the state backgrounds of the custom-drawn buttons in
// the theme (the engine owns them; Go code fetches them in Draw). Values come
// from the decoded EasyWorship styles: sd.toolbuttonbk.style (radius 2,
// opacity 0.5, fill highlight.hot / highlight, stroke window.border) and
// sd.button.mini.bk.style (radius 4, same fills, 0.5 stroke).
func buildWidgetBoxes(th Theme.Instance, p Palette) {
	box := func(fill Color.RGBA, radius [4]int) StyleBox.Instance {
		sb := flat(uicore.WithAlpha(fill, 0.5))
		sb.SetBorderWidthAll(1)
		sb.SetBorderColor(uicore.WithAlpha(p.WindowBorder, 0.45))
		sb.SetCornerRadiusTopLeft(radius[0])
		sb.SetCornerRadiusTopRight(radius[1])
		sb.SetCornerRadiusBottomRight(radius[2])
		sb.SetCornerRadiusBottomLeft(radius[3])
		sb.SetAntiAliasing(true)
		return sb.AsStyleBox()
	}
	all2, all4 := [4]int{2, 2, 2, 2}, [4]int{4, 4, 4, 4}
	left4, right4 := [4]int{4, 0, 0, 4}, [4]int{0, 4, 4, 0}
	th.SetStylebox("toolbutton_hot", tFw, box(p.HighlightHot, all2))
	th.SetStylebox("toolbutton_sel", tFw, box(p.Highlight, all2))
	th.SetStylebox("mini_hot", tFw, box(p.HighlightHot, all4))
	th.SetStylebox("mini_sel", tFw, box(p.Highlight, all4))
	th.SetStylebox("mini_l_hot", tFw, box(p.HighlightHot, left4))
	th.SetStylebox("mini_l_sel", tFw, box(p.Highlight, left4))
	th.SetStylebox("mini_r_hot", tFw, box(p.HighlightHot, right4))
	th.SetStylebox("mini_r_sel", tFw, box(p.Highlight, right4))

	// Resource tab button (sd.tabbarbutton.1.style): the selected fill is
	// tab.sel (highlight.inactive in the Windows style) with 2px top corners.
	tab := flat(p.TabSel)
	tab.SetCornerRadiusTopLeft(2)
	tab.SetCornerRadiusTopRight(2)
	th.SetStylebox("tab_sel", tFw, tab.AsStyleBox())

	// Slide tile selection: the muted tile.frame around the thumbnail (the
	// caption bar is drawn in tile.caption by the tile), hover a translucent
	// highlight.hot wash.
	th.SetStylebox("tile_sel", tFw, rounded(p.TileFrame, 3))
	th.SetStylebox("tile_hot", tFw, rounded(uicore.WithAlpha(p.HighlightHot, 0.35), 3))

	// Schedule item cards: raised card.bk, highlight when selected, the same
	// wash when hovered.
	th.SetStylebox("card", tFw, rounded(p.CardBk, 4))
	th.SetStylebox("card_sel", tFw, rounded(p.Highlight, 4))
	th.SetStylebox("card_hot", tFw, rounded(uicore.WithAlpha(p.HighlightHot, 0.35), 4))
}

// rounded is a flat, anti-aliased stylebox with one corner radius.
func rounded(fill Color.RGBA, radius int) StyleBox.Instance {
	sb := flat(fill)
	sb.SetCornerRadiusAll(radius)
	sb.SetAntiAliasing(true)
	return sb.AsStyleBox()
}

func buildButtonTheme(th Theme.Instance, p Palette) {
	mk := func(bg, border Color.RGBA) StyleBox.Instance {
		sb := flat(bg)
		sb.SetBorderWidthAll(1)
		sb.SetBorderColor(border)
		sb.SetCornerRadiusAll(3)
		margins(sb.AsStyleBox(), 10, 2, 10, 2)
		return sb.AsStyleBox()
	}
	th.SetStylebox("normal", "Button", mk(p.ButtonBk, p.Border))
	th.SetStylebox("hover", "Button", mk(p.Highlight2Hot, p.Border))
	th.SetStylebox("pressed", "Button", mk(p.Highlight2, p.Border))
	th.SetStylebox("hover_pressed", "Button", mk(p.Highlight2, p.Border))
	th.SetStylebox("disabled", "Button", mk(p.ButtonBk, uicore.WithAlpha(p.Border, 0.5)))
	focus := StyleBoxFlat.New()
	focus.SetDrawCenter(false)
	focus.SetBorderWidthAll(1)
	focus.SetBorderColor(p.Highlight)
	focus.SetCornerRadiusAll(3)
	th.SetStylebox("focus", "Button", focus.AsStyleBox())
	th.SetColor("font_color", "Button", p.Text)
	th.SetColor("font_hover_color", "Button", p.Highlight2Text)
	th.SetColor("font_pressed_color", "Button", p.Highlight2Text)
	th.SetColor("font_hover_pressed_color", "Button", p.Highlight2Text)
	th.SetColor("font_focus_color", "Button", p.Text)
	th.SetColor("font_disabled_color", "Button", uicore.WithAlpha(p.Text, 0.45))
}

func buildEditTheme(th Theme.Instance, p Palette) {
	field := func(border Color.RGBA) StyleBox.Instance {
		sb := flat(p.Bk0)
		sb.SetBorderWidthAll(1)
		sb.SetBorderColor(border)
		sb.SetCornerRadiusAll(2)
		margins(sb.AsStyleBox(), 5, 3, 5, 3)
		return sb.AsStyleBox()
	}
	for _, t := range []string{"LineEdit", "TextEdit"} {
		th.SetStylebox("normal", t, field(p.Border))
		th.SetStylebox("focus", t, field(p.Highlight))
		th.SetStylebox("read_only", t, field(p.Border))
		th.SetColor("font_color", t, p.TextSemibold)
		th.SetColor("font_placeholder_color", t, uicore.WithAlpha(p.Text, 0.55))
		th.SetColor("caret_color", t, p.TextSemibold)
		th.SetColor("selection_color", t, uicore.WithAlpha(p.Highlight, 0.75))
		th.SetColor("font_selected_color", t, p.HighlightText)
		th.SetColor("clear_button_color", t, p.Text)
		th.SetColor("clear_button_color_pressed", t, p.TextSemibold)
	}
	th.SetColor("current_line_color", "TextEdit", uicore.WithAlpha(p.Highlight2, 0.25))

	// Embedded search locator (sd.searchlocator.embedded.style): a flat strip
	// on resource.bk with a bottom border.bl line; the field inside has no
	// frame.
	strip := flat(p.ResourceBk)
	strip.SetBorderWidthBottom(1)
	strip.SetBorderColor(p.BorderBL)
	margins(strip.AsStyleBox(), 6, 1, 3, 2)
	panelVariation(th, vSearchPanel, strip.AsStyleBox())
	th.SetTypeVariation(vSearch, "LineEdit")
	bare := StyleBoxEmpty.New()
	margins(bare.AsStyleBox(), 1, 1, 1, 1)
	th.SetStylebox("normal", vSearch, bare.AsStyleBox())
	th.SetStylebox("focus", vSearch, bare.AsStyleBox())
	th.SetStylebox("read_only", vSearch, bare.AsStyleBox())
	th.SetColor("font_placeholder_color", vSearch, uicore.WithAlpha(p.Text, 0.6))
}

func buildTreeTheme(th Theme.Instance, p Palette) {
	th.SetStylebox("panel", "Tree", flat(p.ResourceBk).AsStyleBox())
	th.SetStylebox("focus", "Tree", StyleBoxEmpty.New().AsStyleBox())
	// The selected row is highlight only while the list has focus; unfocused
	// it is the subtle list.sel.inactive (highlight.2 in the Windows style).
	sel := flat(p.ListSelInactive)
	selFocus := flat(p.Highlight)
	th.SetStylebox("selected", "Tree", sel.AsStyleBox())
	th.SetStylebox("selected_focus", "Tree", selFocus.AsStyleBox())
	hover := flat(uicore.WithAlpha(p.Highlight2Hot, 0.25))
	th.SetStylebox("hovered", "Tree", hover.AsStyleBox())
	th.SetStylebox("hovered_dimmed", "Tree", hover.AsStyleBox())
	th.SetStylebox("hovered_selected", "Tree", sel.AsStyleBox())
	th.SetStylebox("hovered_selected_focus", "Tree", selFocus.AsStyleBox())
	th.SetStylebox("cursor", "Tree", StyleBoxEmpty.New().AsStyleBox())
	th.SetStylebox("cursor_unfocused", "Tree", StyleBoxEmpty.New().AsStyleBox())

	title := gradBox(p.ToolbarMini, 24, uicore.Edge{}, uicore.Edge{Width: 1, Color: p.HeaderColumnStroke},
		uicore.Edge{}, uicore.Edge{Width: 1, Color: p.HeaderColumnStroke})
	margins(title.AsStyleBox(), 8, 3, 8, 4)
	for _, s := range []string{"title_button_normal", "title_button_hover", "title_button_pressed"} {
		th.SetStylebox(s, "Tree", title.AsStyleBox())
	}
	th.SetColor("font_color", "Tree", p.Text)
	th.SetColor("font_hovered_color", "Tree", p.Text)
	th.SetColor("font_hovered_dimmed_color", "Tree", p.Text)
	// One selected-text color must read on both highlight (focused) and
	// highlight.2 (unfocused): light text on dark, dark text on light.
	selText := p.HighlightText
	if !p.IsDark() {
		selText = p.Highlight2Text
	}
	th.SetColor("font_selected_color", "Tree", selText)
	th.SetColor("font_hovered_selected_color", "Tree", selText)
	th.SetColor("title_button_color", "Tree", p.TextSemibold)
	th.SetColor("guide_color", "Tree", uicore.WithAlpha(p.SelectorLine, 0))
	th.SetColor("relationship_line_color", "Tree", uicore.WithAlpha(p.SelectorLine, 0))
	th.SetConstant("draw_guides", "Tree", 0)
	th.SetConstant("draw_relationship_lines", "Tree", 0)
	th.SetConstant("v_separation", "Tree", 5)
	th.SetConstant("h_separation", "Tree", 4)
	th.SetConstant("inner_item_margin_left", "Tree", 6)
	th.SetConstant("inner_item_margin_right", "Tree", 6)
	th.SetConstant("item_margin", "Tree", 0) // Phase 1 lists are flat
	th.SetConstant("scrollbar_margin_right", "Tree", 1)
	th.SetConstant("scrollbar_margin_top", "Tree", 1)
	th.SetConstant("scrollbar_margin_bottom", "Tree", 1)
	th.SetConstant("scroll_border", "Tree", 16)
	th.SetFontSize("font_size", "Tree", fontBase)
	th.SetFontSize("title_button_font_size", "Tree", fontBase)

	th.SetTypeVariation(vLibraryTree, "Tree")
	th.SetStylebox("panel", vLibraryTree, flat(p.LibraryBk).AsStyleBox())
}

func buildScrollTheme(th Theme.Instance, p Palette) {
	for _, t := range []string{"VScrollBar", "HScrollBar"} {
		track := StyleBoxEmpty.New()
		margins(track.AsStyleBox(), 2, 2, 2, 2)
		th.SetStylebox("scroll", t, track.AsStyleBox())
		th.SetStylebox("scroll_focus", t, track.AsStyleBox())
		grab := func(c Color.RGBA) StyleBox.Instance {
			sb := flat(c)
			sb.SetCornerRadiusAll(4)
			margins(sb.AsStyleBox(), 3, 3, 3, 3)
			return sb.AsStyleBox()
		}
		th.SetStylebox("grabber", t, grab(uicore.WithAlpha(p.ScrollThumb, p.ScrollThumb.A*0.8)))
		th.SetStylebox("grabber_highlight", t, grab(p.ScrollThumb))
		th.SetStylebox("grabber_pressed", t, grab(p.ScrollThumb))
	}
}

func buildMenuTheme(th Theme.Instance, p Palette) {
	panel := flat(p.MenuFill)
	panel.SetBorderWidthAll(1)
	panel.SetBorderColor(p.MenuStroke)
	panel.SetCornerRadiusAll(4)
	margins(panel.AsStyleBox(), 4, 4, 4, 4)
	th.SetStylebox("panel", "PopupMenu", panel.AsStyleBox())
	th.SetStylebox("panel", "PopupPanel", panel.AsStyleBox())
	hover := flat(p.MenuItemFill)
	hover.SetCornerRadiusAll(3)
	th.SetStylebox("hover", "PopupMenu", hover.AsStyleBox())
	sep := StyleBoxLine.New()
	sep.SetColor(p.DividerMenu)
	sep.SetThickness(1)
	sep.SetGrowBegin(-6)
	sep.SetGrowEnd(-6)
	th.SetStylebox("separator", "PopupMenu", sep.AsStyleBox())
	th.SetStylebox("labeled_separator_left", "PopupMenu", sep.AsStyleBox())
	th.SetStylebox("labeled_separator_right", "PopupMenu", sep.AsStyleBox())
	th.SetColor("font_color", "PopupMenu", p.Text)
	th.SetColor("font_hover_color", "PopupMenu", p.TextSemibold)
	th.SetColor("font_disabled_color", "PopupMenu", uicore.WithAlpha(p.Text, 0.4))
	th.SetColor("font_accelerator_color", "PopupMenu", uicore.WithAlpha(p.Text, 0.6))
	th.SetColor("font_separator_color", "PopupMenu", p.Text)
	th.SetConstant("v_separation", "PopupMenu", 7)
	th.SetConstant("item_start_padding", "PopupMenu", 10)
	th.SetConstant("item_end_padding", "PopupMenu", 14)
	th.SetFontSize("font_size", "PopupMenu", fontBase)
}

func panelVariation(th Theme.Instance, name string, sb StyleBox.Instance) {
	th.SetTypeVariation(name, "PanelContainer")
	th.SetStylebox("panel", name, sb)
}

func labelVariation(th Theme.Instance, name string, font Font.Instance, size int, c Color.RGBA) {
	th.SetTypeVariation(name, "Label")
	th.SetFont("font", name, font)
	th.SetFontSize("font_size", name, size)
	th.SetColor("font_color", name, c)
}

func systemFont(weight int) Font.Instance { return systemFontStyle(weight, false) }

func systemFontStyle(weight int, italic bool) Font.Instance {
	f := SystemFont.New()
	f.SetFontNames([]string{".AppleSystemUIFont", "SF Pro Text", "SF Pro", "Segoe UI", "Helvetica Neue", "Arial", "sans-serif"})
	if weight != 400 {
		f.SetFontWeight(weight)
	}
	if italic {
		f.SetFontItalic(true)
	}
	return f.AsFont()
}

func monoFont() Font.Instance {
	f := SystemFont.New()
	f.SetFontNames([]string{"SF Mono", "Menlo", "Consolas", "monospace"})
	return f.AsFont()
}

func flat(bg Color.RGBA) StyleBoxFlat.Instance {
	sb := StyleBoxFlat.New()
	sb.SetBgColor(bg)
	return sb
}

func margins(sb StyleBox.Instance, l, t, r, b float32) {
	sb.SetContentMarginLeft(l)
	sb.SetContentMarginTop(t)
	sb.SetContentMarginRight(r)
	sb.SetContentMarginBottom(b)
}

// gradBox builds a StyleBoxTexture from a vertical gradient strip whose border
// rows/columns sit in the texture margins (so they never stretch).
func gradBox(g uicore.Gradient, height int, top, bottom, left, right uicore.Edge) StyleBoxTexture.Instance {
	w, px := uicore.GradientBox(g, height, top, bottom, left, right)
	img := Image.CreateFromData(w, height, false, Image.FormatRgba8, px)
	sb := StyleBoxTexture.New()
	sb.SetTexture(ImageTexture.CreateFromImage(img).AsTexture2D())
	sb.SetTextureMarginTop(float32(top.Width))
	sb.SetTextureMarginBottom(float32(bottom.Width))
	sb.SetTextureMarginLeft(float32(left.Width))
	sb.SetTextureMarginRight(float32(right.Width))
	return sb
}
