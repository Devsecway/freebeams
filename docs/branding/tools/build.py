#!/usr/bin/env python3
"""Build the FreeBeams brand assets from the master icon, graphics/icon.svg.

  docs/branding/freebeams/icon.svg                a copy of the master (so the folder is self-contained)
  docs/branding/freebeams/wordmark-dark.svg       lockup for dark backgrounds (icon + "FreeBeams", light text)
  docs/branding/freebeams/wordmark-light.svg      lockup for light backgrounds (indigo text)
  docs/branding/freebeams/preview/icon-{16,32,64,128,256,512}.png
  docs/branding/freebeams/preview/wordmark-{dark,light}.png   (2x)
  docs/branding/freebeams/preview/context.png     the icon in a Dock row on a dark and on a light desktop
  --logo-screen PATH                              1920x1080 PNG: the brand gradient with the dark wordmark
                                                  centred (the app's built-in Logo screen,
                                                  internal/app/core/logo-screen.png)

The name is set in Inter (SIL Open Font License 1.1): "Free" in Medium, "Beams" in Bold, outlined to paths
with fontTools so the SVGs need no installed font. Rendering is QuickLook (qlmanage), the renderer
packaging/icons/make-icons.sh uses, with the alpha recovered from a render on white and one on black.

Usage (macOS; python3 -m pip install Pillow numpy fonttools):

  python3 -I docs/branding/tools/build.py --font-medium Inter-Medium.ttf --font-bold Inter-Bold.ttf \
      [--logo-screen internal/app/core/logo-screen.png]

Any Inter 4.x distribution works (https://github.com/rsms/inter releases, Google Fonts, @expo-google-fonts/inter).
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile
import uuid

import numpy as np
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
BRANDING = os.path.dirname(HERE)
REPO = os.path.dirname(os.path.dirname(BRANDING))
MASTER = os.path.join(REPO, "graphics", "icon.svg")
BRAND = os.path.join(BRANDING, "freebeams")
PREVIEW = os.path.join(BRAND, "preview")

# Palette (docs/branding/README.md).
INDIGO_DEEP = "#1A1750"
INDIGO_LIT = "#4740D8"
BEAM_GOLD = "#FFC94D"
TEXT_ON_DARK = "#F5F7FB"
TEXT_ON_LIGHT = INDIGO_DEEP
# The Logo screen's gradient: the tile gradient at about 70 % brightness, so the lockup's tile still
# reads against it and the screen stays calm in a dim room.
SCREEN_DEEP = "#12103A"
SCREEN_LIT = "#322D98"

# Lockup geometry (viewBox units; 1 unit = 1 px at 1x): a 128 px icon canvas (103 px tile), the name
# at 72 px (cap height 52 px, half the tile), text starting 20 px right of the tile.
LOCKUP_H = 128
TEXT_SIZE = 72
TEXT_X = 136
TRACKING = -0.015  # em
END_PAD = 18


class Outliner:
    """Outlines text with a TrueType font into SVG path data, applying GPOS pair kerning
    (the `kern` feature, PairPos formats 1 and 2)."""

    def __init__(self, path):
        self.font = TTFont(path)
        self.upem = self.font["head"].unitsPerEm
        self.cmap = self.font.getBestCmap()
        self.hmtx = self.font["hmtx"]
        self.glyphs = self.font.getGlyphSet()
        self.pairs = {}
        self.classes = []
        self._load_kerning()

    def _load_kerning(self):
        if "GPOS" not in self.font:
            return
        gpos = self.font["GPOS"].table
        ids = set()
        for fr in gpos.FeatureList.FeatureRecord:
            if fr.FeatureTag == "kern":
                ids.update(fr.Feature.LookupListIndex)
        for li in sorted(ids):
            lookup = gpos.LookupList.Lookup[li]
            for st in lookup.SubTable:
                if lookup.LookupType == 9:
                    st = st.ExtSubTable
                if getattr(st, "LookupType", lookup.LookupType) != 2:
                    continue
                if st.Format == 1:
                    for first, ps in zip(st.Coverage.glyphs, st.PairSet):
                        for rec in ps.PairValueRecord:
                            dx = getattr(rec.Value1, "XAdvance", 0) if rec.Value1 else 0
                            if dx:
                                self.pairs.setdefault((first, rec.SecondGlyph), dx)
                elif st.Format == 2:
                    self.classes.append((set(st.Coverage.glyphs), st.ClassDef1.classDefs, st.ClassDef2.classDefs, st.Class1Record))

    def _kern(self, left, right):
        if (left, right) in self.pairs:
            return self.pairs[(left, right)]
        for cov, cd1, cd2, recs in self.classes:
            if left not in cov:
                continue
            v = recs[cd1.get(left, 0)].Class2Record[cd2.get(right, 0)].Value1
            dx = getattr(v, "XAdvance", 0) if v else 0
            if dx:
                return dx
        return 0

    def layout(self, text, size, tracking=0.0, x=0.0):
        """Path data for `text` at `size` px starting at pen position x, origin on the baseline,
        y growing downward; returns (d, next_x)."""
        scale = size / self.upem
        names = [self.cmap[ord(ch)] for ch in text]
        pen = SVGPathPen(self.glyphs, ntos=lambda v: f"{v:.2f}")
        for i, name in enumerate(names):
            self.glyphs[name].draw(TransformPen(pen, (scale, 0, 0, -scale, x, 0)))
            adv = self.hmtx[name][0]
            if i + 1 < len(names):
                adv += self._kern(name, names[i + 1])
            x += adv * scale + tracking * size
        return pen.getCommands(), x

    def cap_height(self, size):
        return self.font["OS/2"].sCapHeight * size / self.upem

    def family(self):
        names = self.font["name"]
        return f"{names.getDebugName(1)} {names.getDebugName(2)} {names.getDebugName(5)}"


def icon_inner(svg_text):
    """The markup between <svg ...> and </svg>, comments removed."""
    body = re.sub(r"<!--.*?-->", "", svg_text, flags=re.S)
    m = re.search(r"<svg\b[^>]*>(.*)</svg>\s*$", body, flags=re.S)
    if not m:
        sys.exit("icon.svg: no <svg> element")
    return m.group(1).strip("\n")


def indent(text, n):
    pad = " " * n
    return "\n".join(pad + line if line.strip() else line for line in text.splitlines())


def lockup_svg(inner, parts, color, theme, fonts_desc):
    """parts: [(text, Outliner)], set one after the other on a shared baseline."""
    cap = parts[0][1].cap_height(TEXT_SIZE)
    baseline = LOCKUP_H / 2 + cap / 2
    x = 0.0
    paths = []
    for text, ol in parts:
        d, x = ol.layout(text, TEXT_SIZE, TRACKING, x)
        paths.append(f'  <path transform="translate({TEXT_X} {baseline:.2f})" fill="{color}" d="{d}"/>')
    width = int(round(TEXT_X + x + END_PAD))
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{LOCKUP_H}" viewBox="0 0 {width} {LOCKUP_H}">
  <!-- FreeBeams wordmark for {theme} backgrounds: the app icon beside the name, text {color}.
       Generated by docs/branding/tools/build.py from graphics/icon.svg; the name is outlined from
       {fonts_desc} (SIL Open Font License 1.1), so no font needs to be installed. -->
  <svg x="0" y="0" width="{LOCKUP_H}" height="{LOCKUP_H}" viewBox="0 0 1024 1024">
{indent(inner, 4)}
  </svg>
{chr(10).join(paths)}
</svg>
"""


def qlrender(svg_path, size, out_dir):
    """Rasterise with QuickLook; returns the PNG path (longest side == size)."""
    subprocess.run(["qlmanage", "-t", "-s", str(size), "-o", out_dir, svg_path], check=False, capture_output=True)
    png = os.path.join(out_dir, os.path.basename(svg_path) + ".png")
    if not os.path.exists(png):
        sys.exit(f"qlmanage produced no PNG for {svg_path}")
    return png


def render_rgba(svg_path, scale, out_dir):
    """Rasterise an SVG at `scale` px per user unit with a real alpha channel.

    QuickLook thumbnails are opaque (white behind the artwork) and a non-square SVG is scaled
    unpredictably, so the SVG is nested in a square canvas, rendered on white and on black, and the
    alpha recovered: alpha = 1 - (white - black), colour = black / alpha. The result is cropped to
    the SVG's own width x height."""
    text = open(svg_path, encoding="utf-8").read()
    m = re.search(r'<svg\b[^>]*\bwidth="(\d+)"[^>]*\bheight="(\d+)"[^>]*>', text)
    if not m:
        sys.exit(f"{svg_path}: the root <svg> needs integer width and height attributes")
    w, h = int(m.group(1)), int(m.group(2))
    side = max(w, h)
    renders = []
    tag = uuid.uuid4().hex[:8]
    for bg, color in (("w", "#FFFFFF"), ("b", "#000000")):
        tmp_svg = os.path.join(out_dir, f"{tag}-{bg}-{os.path.basename(svg_path)}")
        with open(tmp_svg, "w", encoding="utf-8") as f:
            f.write(
                f'<svg xmlns="http://www.w3.org/2000/svg" width="{side}" height="{side}" viewBox="0 0 {side} {side}">'
                f'<rect width="100%" height="100%" fill="{color}"/>'
                + text[m.start():].replace("<?xml", "<!--", 1).replace("?>", "-->", 1)
                + "</svg>"
            )
        img = Image.open(qlrender(tmp_svg, side * scale, out_dir)).convert("RGB")
        renders.append(np.asarray(img, dtype=np.float32)[: h * scale, : w * scale])
    white, black = renders
    alpha = np.clip(1.0 - (white - black).mean(axis=2) / 255.0, 0.0, 1.0)
    safe = np.where(alpha > 1e-3, alpha, 1.0)[..., None]
    rgb = np.clip(black / safe, 0, 255)
    rgba = np.dstack([rgb, alpha[..., None] * 255.0]).round().astype(np.uint8)
    return Image.fromarray(rgba, "RGBA")


def resize(img, w, h=None):
    return img.resize((w, h or w), Image.Resampling.LANCZOS)


def hex_rgb(h):
    return tuple(int(h[i:i + 2], 16) for i in (1, 3, 5))


def diagonal_gradient(w, h, c0, c1):
    """c0 at the bottom-left to c1 at the top-right, like the tile."""
    ys, xs = np.mgrid[0:h, 0:w].astype(np.float32)
    t = (xs / max(w - 1, 1) + (1 - ys / max(h - 1, 1))) / 2
    a, b = np.array(hex_rgb(c0), np.float32), np.array(hex_rgb(c1), np.float32)
    rgb = a + (b - a) * t[..., None]
    return Image.fromarray(np.clip(rgb.round(), 0, 255).astype(np.uint8), "RGB")


def rounded_tile(size, color, rx):
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ImageDraw.Draw(img).rounded_rectangle((0, 0, size - 1, size - 1), radius=rx, fill=color)
    return img


def placeholder_tiles(size):
    """Generic neighbour icons for the Dock mock: plain tiles with one simple shape, no real app."""
    tiles = []
    specs = [
        ((88, 90, 98, 255), "circle"),
        ((56, 140, 92, 255), "square"),
        ((214, 92, 76, 255), "bars"),
        ((60, 120, 200, 255), "ring"),
        ((236, 180, 56, 255), "triangle"),
        ((120, 84, 170, 255), "dot"),
    ]
    rx = int(size * 232 / 824)
    for color, shape in specs:
        t = rounded_tile(size, color, rx)
        d = ImageDraw.Draw(t)
        m = size * 0.28
        w = (255, 255, 255, 235)
        if shape == "circle":
            d.ellipse((m, m, size - m, size - m), fill=w)
        elif shape == "square":
            d.rounded_rectangle((m, m, size - m, size - m), radius=size * 0.06, fill=w)
        elif shape == "bars":
            for i in range(3):
                y = m + i * (size - 2 * m) / 2.6
                d.rounded_rectangle((m, y, size - m, y + size * 0.08), radius=size * 0.04, fill=w)
        elif shape == "ring":
            d.ellipse((m, m, size - m, size - m), outline=w, width=int(size * 0.08))
        elif shape == "triangle":
            d.polygon([(size / 2, m), (size - m, size - m), (m, size - m)], fill=w)
        elif shape == "dot":
            d.ellipse((size * 0.4, size * 0.4, size * 0.6, size * 0.6), fill=w)
        tiles.append(t)
    return tiles


def context_mock(icon, label_font, out_path):
    """Two desktops (dark, light), each with a Dock row of 64 pt icons at 2x, FreeBeams in the middle,
    and a Finder-style row with the 32 and 16 px icons beside the name."""
    W, H = 1600, 420
    tile = 128  # 64 pt at 2x, the macOS Dock default
    gap = 28
    canvas = Image.new("RGBA", (W, H * 2), (0, 0, 0, 255))
    for row, (deep, lit, text, dock_fill, line) in enumerate((
        ("#101428", "#2A2F5C", (235, 236, 242, 255), (255, 255, 255, 34), (255, 255, 255, 60)),
        ("#C9D3E6", "#EEF1F7", (34, 36, 52, 255), (255, 255, 255, 150), (0, 0, 0, 40)),
    )):
        y0 = row * H
        canvas.paste(diagonal_gradient(W, H, deep, lit), (0, y0))
        d = ImageDraw.Draw(canvas)
        tiles = placeholder_tiles(tile)
        order = tiles[:3] + [resize(icon, tile)] + tiles[3:]
        total = len(order) * tile + (len(order) - 1) * gap
        dock_x = (W - total) // 2
        dock_y = y0 + H - tile - 70
        pad = 18
        dock = Image.new("RGBA", (total + 2 * pad, tile + 2 * pad), (0, 0, 0, 0))
        ImageDraw.Draw(dock).rounded_rectangle((0, 0, dock.width - 1, dock.height - 1), radius=34, fill=dock_fill, outline=line)
        canvas.alpha_composite(dock, (dock_x - pad, dock_y - pad))
        x = dock_x
        for t in order:
            canvas.alpha_composite(t, (x, dock_y))
            x += tile + gap
        # Finder-style rows: 32 px and 16 px icons with the name.
        fx, fy = 60, y0 + 48
        canvas.alpha_composite(resize(icon, 32), (fx, fy))
        d.text((fx + 44, fy + 4), "FreeBeams", font=label_font[0], fill=text)
        canvas.alpha_composite(resize(icon, 16), (fx, fy + 54))
        d.text((fx + 28, fy + 52), "FreeBeams.app", font=label_font[1], fill=text)
    canvas.convert("RGB").save(out_path, optimize=True)


def logo_screen(wordmark_svg, out_path, tmp):
    """1920x1080: the screen gradient (deep indigo bottom-left to lit indigo top-right) with the dark
    wordmark centred, 1/8 of the screen height tall."""
    W, H = 1920, 1080
    screen = diagonal_gradient(W, H, SCREEN_DEEP, SCREEN_LIT).convert("RGBA")
    wm = render_rgba(wordmark_svg, 4, tmp)  # 512 px tall, for a clean downsample
    target_h = 136
    wm = resize(wm, int(round(wm.width * target_h / wm.height)), target_h)
    screen.alpha_composite(wm, ((W - wm.width) // 2, (H - wm.height) // 2))
    screen.convert("RGB").save(out_path, optimize=True)
    return wm.size


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--font-medium", required=True, help="Inter Medium TTF (the word \"Free\")")
    ap.add_argument("--font-bold", required=True, help="Inter Bold TTF (the word \"Beams\")")
    ap.add_argument("--logo-screen", metavar="PATH", help="also write the 1920x1080 Logo screen PNG here")
    args = ap.parse_args()

    if shutil.which("qlmanage") is None:
        sys.exit("qlmanage not found: this script runs on macOS")
    medium, bold = Outliner(args.font_medium), Outliner(args.font_bold)
    fonts_desc = f"{medium.family()} and {bold.family()}"
    parts = [("Free", medium), ("Beams", bold)]
    label_font = (ImageFont.truetype(args.font_medium, 26), ImageFont.truetype(args.font_medium, 20))

    os.makedirs(PREVIEW, exist_ok=True)
    master = open(MASTER, encoding="utf-8").read()
    shutil.copyfile(MASTER, os.path.join(BRAND, "icon.svg"))
    inner = icon_inner(master)

    wordmarks = {}
    for theme, color in (("dark", TEXT_ON_DARK), ("light", TEXT_ON_LIGHT)):
        path = os.path.join(BRAND, f"wordmark-{theme}.svg")
        with open(path, "w", encoding="utf-8") as f:
            f.write(lockup_svg(inner, parts, color, theme, fonts_desc))
        wordmarks[theme] = path

    tmp = tempfile.mkdtemp(prefix="freebeams-brand-")
    try:
        icon = render_rgba(MASTER, 1, tmp)
        if icon.size != (1024, 1024):
            sys.exit(f"icon: expected 1024x1024, got {icon.size}")
        for s in (16, 32, 64, 128, 256, 512):
            resize(icon, s).save(os.path.join(PREVIEW, f"icon-{s}.png"), optimize=True)
        for theme, path in wordmarks.items():
            render_rgba(path, 2, tmp).save(os.path.join(PREVIEW, f"wordmark-{theme}.png"), optimize=True)
        context_mock(icon, label_font, os.path.join(PREVIEW, "context.png"))
        print("freebeams/icon.svg, wordmark-dark.svg, wordmark-light.svg, preview/icon-*.png, wordmark-*.png, context.png")
        if args.logo_screen:
            size = logo_screen(wordmarks["dark"], args.logo_screen, tmp)
            print(f"{args.logo_screen}: 1920x1080, wordmark {size[0]}x{size[1]}")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    main()
