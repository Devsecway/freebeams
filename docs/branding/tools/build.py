#!/usr/bin/env python3
"""Build the branding previews from the hand-written icon SVGs.

For every concept directory in docs/branding that holds an icon.svg this script

  1. writes the wordmark lockups (wordmark-dark.svg, wordmark-light.svg for the
     primary name, plus wordmark-{dark,light}-<name>.svg for the other names).
     The lockup nests the icon artwork and the app name outlined from a TrueType
     font (Inter SemiBold, SIL OFL 1.1), so the SVGs need no installed font;
  2. rasterises icon and wordmarks with QuickLook (qlmanage, the same renderer
     packaging/icons/make-icons.sh uses) and resizes the icon with Pillow to
     the sizes the OS icon sets contain (16 ... 512);
  3. composes preview sheets (icons on dark and light at 16/32/128, the
     wordmarks, and the icon beside a crop of the current toolbar) and writes
     docs/branding/preview.html.

Usage (macOS, Pillow and fontTools installed):

  python3 -I docs/branding/tools/build.py --font /path/to/Inter_600SemiBold.ttf \
      [--names Canticle Antiphon] [--label-font /path/to/Inter_500Medium.ttf]

Nothing outside docs/branding is written. Run it after editing an icon.svg.
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile

from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFont

HERE = os.path.dirname(os.path.abspath(__file__))
BRANDING = os.path.dirname(HERE)
REPO = os.path.dirname(os.path.dirname(BRANDING))
SCREEN = os.path.join(REPO, "docs", "screens", "app-main-start.png")

CONCEPTS = ["verse", "response", "window"]
TEXT_ON_DARK = "#F5F7FB"
TEXT_ON_LIGHT = "#17264A"
DARK_BG = (54, 54, 54, 255)  # UI-SPEC window.bk2 #363636
LIGHT_BG = (240, 240, 240, 255)  # UI-SPEC light window.bk2 #F0F0F0

# Lockup geometry (viewBox units; 1 unit = 1 px at 1x).
LOCKUP_H = 128  # icon canvas height; the tile inside is 103 px (824/1024)
TEXT_SIZE = 74  # Inter cap height 0.727 em -> ~54 px, half the tile height
TEXT_X = 136
TRACKING = -0.015  # em


class Outliner:
    """Outlines a string with a TrueType font into one SVG path, with GPOS
    pair kerning (the `kern` feature, PairPos formats 1 and 2)."""

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

    def layout(self, text, size, tracking=0.0):
        """Path data for `text` at `size` px with the pen origin on the
        baseline, y growing downward; returns (d, advance_px)."""
        scale = size / self.upem
        names = [self.cmap[ord(ch)] for ch in text]
        pen = SVGPathPen(self.glyphs, ntos=lambda v: f"{v:.2f}")
        x = 0.0
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
        return f"{names.getDebugName(1)} {names.getDebugName(5)}"


def icon_inner(svg_text):
    """The markup between <svg ...> and </svg>, comments removed."""
    body = re.sub(r"<!--.*?-->", "", svg_text, flags=re.S)
    m = re.search(r"<svg\b[^>]*>(.*)</svg>\s*$", body, flags=re.S)
    if not m:
        sys.exit("icon.svg: no <svg> element")
    return m.group(1).strip("\n")


def lockup_svg(inner, name_path, advance, color, concept, name, font_desc):
    width = int(round(TEXT_X + advance + 18))
    baseline = LOCKUP_H / 2 + CAP / 2
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{LOCKUP_H}" viewBox="0 0 {width} {LOCKUP_H}">
  <!-- Wordmark lockup for concept "{concept}": the app icon beside the name
       "{name}". Generated by docs/branding/tools/build.py from icon.svg; the
       name is outlined from {font_desc} (SIL Open Font License 1.1), so no
       font needs to be installed. Text colour {color}. -->
  <svg x="0" y="0" width="{LOCKUP_H}" height="{LOCKUP_H}" viewBox="0 0 1024 1024">
{indent(inner, 4)}
  </svg>
  <path transform="translate({TEXT_X} {baseline:.2f})" fill="{color}" d="{name_path}"/>
</svg>
"""


def indent(text, n):
    pad = " " * n
    return "\n".join(pad + line if line.strip() else line for line in text.splitlines())


def qlrender(svg_path, size, out_dir):
    """Rasterise with QuickLook; returns the PNG path (longest side == size)."""
    subprocess.run(["qlmanage", "-t", "-s", str(size), "-o", out_dir, svg_path], check=False, capture_output=True)
    png = os.path.join(out_dir, os.path.basename(svg_path) + ".png")
    if not os.path.exists(png):
        sys.exit(f"qlmanage produced no PNG for {svg_path}")
    return png


def render_rgba(svg_path, scale, out_dir):
    """Rasterise an SVG at `scale` px per user unit with a real alpha channel.

    QuickLook thumbnails of SVGs are opaque (white behind the artwork) and a
    non-square SVG is scaled unpredictably, so the SVG is nested in a square
    canvas, rendered twice (on white and on black) and the alpha recovered:
    alpha = 1 - (white - black), colour = black / alpha. The result is cropped
    to the SVG's own width x height."""
    import numpy as np

    text = open(svg_path, encoding="utf-8").read()
    m = re.search(r'<svg\b[^>]*\bwidth="(\d+)"[^>]*\bheight="(\d+)"[^>]*>', text)
    if not m:
        sys.exit(f"{svg_path}: the root <svg> needs integer width and height attributes")
    w, h = int(m.group(1)), int(m.group(2))
    side = max(w, h)
    renders = []
    for tag, color in (("w", "#FFFFFF"), ("b", "#000000")):
        tmp_svg = os.path.join(out_dir, f"{tag}-{os.path.basename(svg_path)}")
        with open(tmp_svg, "w", encoding="utf-8") as f:
            f.write(
                f'<svg xmlns="http://www.w3.org/2000/svg" width="{side}" height="{side}" viewBox="0 0 {side} {side}">'
                f'<rect width="100%" height="100%" fill="{color}"/>'
                + text[m.start() :]
                .replace("<?xml", "<!--", 1)
                .replace("?>", "-->", 1)
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


def zoom(img, factor):
    return img.resize((img.width * factor, img.height * factor), Image.Resampling.NEAREST)


def toolbar_crop():
    """The current operator toolbar (58 pt = 116 px at 2x): the left button
    group and the right button group joined, from docs/screens/app-main-start.png."""
    shot = Image.open(SCREEN).convert("RGBA")
    h = 116
    left = shot.crop((0, 0, 600, h))
    right = shot.crop((shot.width - 740, 0, shot.width, h))
    strip = Image.new("RGBA", (left.width + right.width, h), DARK_BG)
    strip.alpha_composite(left, (0, 0))
    strip.alpha_composite(right, (left.width, 0))
    return strip


def label(draw, xy, text, font, fill):
    draw.text(xy, text, font=font, fill=fill)


def sheet(concept, icon, wordmarks, names, primary, label_font, out_path):
    """One PNG per concept: icons at 16/32/128 on dark and light (16 and 32
    also 4x blown up), the wordmarks on both backgrounds, and the 32 px icon
    beside the current toolbar."""
    W = 1600
    toolbar = toolbar_crop()
    rows = []  # (height, draw_fn)

    def icon_row(bg, fg):
        def draw(canvas, y):
            d = ImageDraw.Draw(canvas)
            top = y + 30
            canvas.alpha_composite(resize(icon, 128), (40, top))
            label(d, (40, top + 128 + 10), "128 px", label_font, fg)
            x = 220
            for s in (32, 16):
                small = resize(icon, s)
                canvas.alpha_composite(small, (x, top))
                canvas.alpha_composite(zoom(small, 4), (x, top + s + 12))
                label(d, (x, top + s + 12 + s * 4 + 8), f"{s} px, 4x", label_font, fg)
                x += s * 4 + 36
            # Wordmarks for every name.
            wx = 520
            theme = "dark" if bg == DARK_BG else "light"
            for name in names:
                wm = wordmarks[(name, theme)]
                wmh = 72
                wmw = int(wm.width * wmh / wm.height)
                canvas.alpha_composite(resize(wm, wmw, wmh), (wx, top))
                label(d, (wx, top + wmh + 10), f"wordmark-{theme}" + ("" if name == primary else f"-{name.lower()}"), label_font, fg)
                wx += wmw + 60
        return (250, bg, draw)

    rows.append(icon_row(DARK_BG, (200, 200, 200, 255)))
    rows.append(icon_row(LIGHT_BG, (60, 60, 60, 255)))

    def toolbar_row(canvas, y):
        d = ImageDraw.Draw(canvas)
        # The 64 px (32 pt @2x) icon where a title-bar or Dock icon would sit, then the toolbar.
        canvas.alpha_composite(resize(icon, 64), (40, y + 26))
        tb = toolbar
        if tb.width > W - 140:
            tb = resize(tb, W - 140, int(tb.height * (W - 140) / tb.width))
        canvas.alpha_composite(tb, (124, y))
        label(d, (40, y + 124), "beside the current toolbar (docs/screens/app-main-start.png, 2x)", label_font, (200, 200, 200, 255))

    rows.append((160, DARK_BG, toolbar_row))

    H = sum(r[0] for r in rows)
    canvas = Image.new("RGBA", (W, H), DARK_BG)
    y = 0
    for h, bg, fn in rows:
        ImageDraw.Draw(canvas).rectangle((0, y, W, y + h), fill=bg)
        fn(canvas, y)
        y += h
    canvas.save(out_path, optimize=True)


def compare_sheet(icons, label_font, out_path):
    """All concepts side by side at 128, 32 and 16 px on dark and light."""
    colw = 300
    W = 60 + colw * len(icons)
    H = 2 * 240
    canvas = Image.new("RGBA", (W, H), DARK_BG)
    d = ImageDraw.Draw(canvas)
    for row, (bg, fg) in enumerate([(DARK_BG, (200, 200, 200, 255)), (LIGHT_BG, (60, 60, 60, 255))]):
        y = row * 240
        d.rectangle((0, y, W, y + 240), fill=bg)
        for col, (concept, icon) in enumerate(icons.items()):
            x = 40 + col * colw
            canvas.alpha_composite(resize(icon, 128), (x, y + 30))
            canvas.alpha_composite(resize(icon, 64), (x + 150, y + 30))
            canvas.alpha_composite(resize(icon, 32), (x + 150, y + 110))
            canvas.alpha_composite(resize(icon, 16), (x + 196, y + 110))
            canvas.alpha_composite(zoom(resize(icon, 16), 4), (x + 226, y + 94))
            label(d, (x, y + 176), concept, label_font, fg)
            label(d, (x + 150, y + 150), "64 / 32 / 16 (16 at 4x)", label_font, fg)
    canvas.save(out_path, optimize=True)


HTML_HEAD = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Branding preview</title>
<style>
  :root { --bg: #363636; --panel: #2C2C2C; --text: #B9B9B9; --strong: #F0F0F0; --accent: #3A6DCE; }
  * { box-sizing: border-box; }
  body { margin: 0; background: var(--bg); color: var(--text); font: 15px/1.5 -apple-system, "Inter", "Helvetica Neue", Arial, sans-serif; }
  header { padding: 28px 32px 8px; }
  h1 { color: var(--strong); font-size: 22px; font-weight: 600; margin: 0 0 4px; }
  h2 { color: var(--strong); font-size: 17px; font-weight: 600; margin: 36px 32px 10px; }
  p { margin: 0; max-width: 72ch; }
  .concept { margin: 0 32px 20px; border: 1px solid #000; background: var(--panel); border-radius: 6px; overflow: hidden; }
  .band { display: flex; align-items: center; gap: 36px; padding: 24px 28px; flex-wrap: wrap; }
  .band.light { background: #F0F0F0; color: #2A2A2A; }
  .band.dark { background: #363636; }
  .band.deep { background: #202020; }
  .band figure { margin: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 6px; }
  .band figcaption { font-size: 12px; opacity: .8; }
  .pixel { image-rendering: pixelated; }
  .icons { display: flex; align-items: flex-end; gap: 18px; }
  .wm img { height: 56px; width: auto; }
  .tag { display: inline-block; font-size: 12px; color: #fff; background: var(--accent); padding: 1px 8px; border-radius: 10px; margin-left: 10px; vertical-align: middle; }
  .toolbar img { width: 100%; max-width: 1340px; height: auto; display: block; }
  .meta { padding: 10px 28px 18px; font-size: 13px; }
  code { background: #202020; padding: 1px 5px; border-radius: 3px; font-size: 12.5px; }
  footer { padding: 20px 32px 40px; font-size: 13px; }
</style>
</head>
<body>
<header>
  <h1>Branding preview</h1>
  <p>Three logo concepts for the app, each as an app icon (16 / 32 / 128 px, the 16 and 32 also pixel-doubled) and a wordmark on dark and on light, for both candidate names. Open <code>README.md</code> for the rationale and the shortlist.</p>
</header>
"""

HTML_FOOT = """<footer>
  Generated by <code>docs/branding/tools/build.py</code> from the hand-written <code>icon.svg</code> files. Rendering: QuickLook (<code>qlmanage</code>) at 1024 px, resized with Pillow (Lanczos), the same pipeline as <code>packaging/icons/make-icons.sh</code>.
</footer>
</body>
</html>
"""


def html_concept(concept, names, primary, blurb, recommended):
    tag = '<span class="tag">recommended</span>' if recommended else ""
    rel = f"{concept}/preview"
    icons = "".join(
        f'<figure><img src="{rel}/icon-{s}.png" width="{s}" height="{s}" alt=""><figcaption>{s} px</figcaption></figure>'
        for s in (128, 64, 32, 16)
    )
    zooms = "".join(
        f'<figure><img class="pixel" src="{rel}/icon-{s}.png" width="{s * 4}" height="{s * 4}" alt=""><figcaption>{s} px at 4x</figcaption></figure>'
        for s in (32, 16)
    )

    def wms(theme):
        out = []
        for name in names:
            suffix = "" if name == primary else f"-{name.lower()}"
            out.append(f'<figure class="wm"><img src="{rel}/wordmark-{theme}{suffix}.png" alt="{name} wordmark"><figcaption>wordmark-{theme}{suffix}.svg</figcaption></figure>')
        return "".join(out)

    return f"""<h2 id="{concept}">{concept.capitalize()}{tag}</h2>
<div class="concept">
  <div class="band dark"><div class="icons">{icons}</div><div class="icons">{zooms}</div>{wms("dark")}</div>
  <div class="band light"><div class="icons">{icons}</div><div class="icons">{zooms}</div>{wms("light")}</div>
  <div class="band deep toolbar"><img src="{rel}/context.png" alt="icon beside the current toolbar"></div>
  <div class="meta">{blurb} Sources: <code>docs/branding/{concept}/icon.svg</code>, <code>wordmark-dark.svg</code>, <code>wordmark-light.svg</code>.</div>
</div>
"""


BLURBS = {
    "verse": "Three centred lyric lines; the gold one is the line being sung now (the Live line). Product-literal: what the congregation sees.",
    "response": "A double quotation mark: the sung words. The first stroke is white (the call), the second gold (the response). Name-literal for Antiphon, in the spirit of Canticle.",
    "window": "A church arch in white with a gold light inside: the nave, the sanctuary door, light in the room. Place-literal.",
}


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--font", required=True, help="TrueType font for the wordmark (Inter SemiBold, OFL)")
    ap.add_argument("--label-font", help="TrueType font for sheet labels (default: --font)")
    ap.add_argument("--names", nargs="+", default=["Canticle", "Antiphon"], help="app names; the first is primary")
    ap.add_argument("--recommend", default="response", help="concept tagged as recommended in preview.html")
    args = ap.parse_args()

    if shutil.which("qlmanage") is None:
        sys.exit("qlmanage not found: this script runs on macOS")
    outliner = Outliner(args.font)
    global CAP
    CAP = outliner.cap_height(TEXT_SIZE)
    label_font = ImageFont.truetype(args.label_font or args.font, 15)
    primary = args.names[0]
    font_desc = outliner.family()

    tmp = tempfile.mkdtemp(prefix="branding-")
    icons = {}
    try:
        for concept in CONCEPTS:
            cdir = os.path.join(BRANDING, concept)
            icon_svg = os.path.join(cdir, "icon.svg")
            if not os.path.exists(icon_svg):
                print(f"skip {concept}: no icon.svg")
                continue
            prev = os.path.join(cdir, "preview")
            os.makedirs(prev, exist_ok=True)
            inner = icon_inner(open(icon_svg, encoding="utf-8").read())

            # Wordmark SVGs.
            wm_files = {}
            for name in args.names:
                d, adv = outliner.layout(name, TEXT_SIZE, TRACKING)
                suffix = "" if name == primary else f"-{name.lower()}"
                for theme, color in (("dark", TEXT_ON_DARK), ("light", TEXT_ON_LIGHT)):
                    path = os.path.join(cdir, f"wordmark-{theme}{suffix}.svg")
                    with open(path, "w", encoding="utf-8") as f:
                        f.write(lockup_svg(inner, d, adv, color, concept, name, font_desc))
                    wm_files[(name, theme)] = path

            # Rasterise.
            ctmp = os.path.join(tmp, concept)
            os.makedirs(ctmp)
            icon = render_rgba(icon_svg, 1, ctmp)
            if icon.size != (1024, 1024):
                sys.exit(f"{concept}: expected 1024x1024, got {icon.size}")
            icons[concept] = icon
            for s in (16, 32, 64, 128, 256, 512):
                resize(icon, s).save(os.path.join(prev, f"icon-{s}.png"), optimize=True)
            wordmarks = {}
            for (name, theme), path in wm_files.items():
                png = render_rgba(path, 2, ctmp)  # 2x, i.e. 256 px tall
                wordmarks[(name, theme)] = png
                png.save(os.path.join(prev, os.path.basename(path)[:-4] + ".png"), optimize=True)

            # Context strip: 64 px icon beside the toolbar crop.
            tb = toolbar_crop()
            ctx = Image.new("RGBA", (tb.width + 100, tb.height), DARK_BG)
            ctx.alpha_composite(resize(icon, 64), (18, (tb.height - 64) // 2))
            ctx.alpha_composite(tb, (100, 0))
            ctx.save(os.path.join(prev, "context.png"), optimize=True)

            sheet(concept, icon, wordmarks, args.names, primary, label_font, os.path.join(prev, "sheet.png"))
            print(f"{concept}: wordmarks, icon-*.png, context.png, sheet.png")

        os.makedirs(os.path.join(BRANDING, "preview"), exist_ok=True)
        compare_sheet(icons, label_font, os.path.join(BRANDING, "preview", "compare.png"))

        with open(os.path.join(BRANDING, "preview.html"), "w", encoding="utf-8") as f:
            f.write(HTML_HEAD)
            f.write('<div class="concept" style="margin-top:16px"><div class="band dark"><img src="preview/compare.png" alt="all concepts" style="max-width:100%;height:auto"></div></div>\n')
            for concept in icons:
                f.write(html_concept(concept, args.names, primary, BLURBS.get(concept, ""), concept == args.recommend))
            f.write(HTML_FOOT)
        print("preview/compare.png, preview.html")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    main()
