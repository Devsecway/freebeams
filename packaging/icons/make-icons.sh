#!/usr/bin/env bash
# Rasterise graphics/icon.svg (the master artwork) into the icon files the
# exports reference. The outputs are committed; rerun this after changing the SVG.
#
#   graphics/icon.png           512 px PNG: the Linux icon (.desktop, AppImage)
#   graphics/icon.ico           Windows icon, 16..256 px (export_presets.cfg,
#                               project.godot windows_native_icon)
#   graphics/FreeBeams.icns     macOS icon, 16..1024 px at @1x and @2x
#                               (export_presets.cfg, project.godot macos_native_icon)
#
# Runs on macOS: QuickLook (qlmanage) rasterises the SVG and iconutil writes the
# .icns. The resizes and the .ico come from Pillow, the alpha recovery from
# NumPy (python3 -m pip install Pillow numpy).
#
# QuickLook thumbnails are opaque: the SVG is composited on white, so the tile's
# corners come out white instead of transparent. The SVG is therefore rendered
# twice, on a white and on a black backdrop, and the alpha recovered per pixel
# (alpha = 1 - (white - black); colour = black / alpha), which is exact for a
# renderer that composites linearly in sRGB, as QuickLook does.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SVG="$ROOT/graphics/icon.svg"
OUT="$ROOT/graphics"
ICNS="FreeBeams.icns"

need() { # need COMMAND "where it comes from"
	command -v "$1" >/dev/null 2>&1 || {
		echo "make-icons: '$1' not found ($2)" >&2
		exit 1
	}
}
need qlmanage "macOS QuickLook; this script runs on macOS"
need iconutil "part of macOS"
need python3 "with the Pillow and numpy packages"
python3 -I -c 'import PIL, numpy' 2>/dev/null || {
	echo "make-icons: the Python packages Pillow and numpy are required: python3 -m pip install Pillow numpy" >&2
	exit 1
}
[ -f "$SVG" ] || {
	echo "make-icons: $SVG not found" >&2
	exit 1
}

TMP="$(mktemp -d -t freebeams-icons.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

echo "==> rasterising $SVG at 1024 px on white and on black"
for bg in white black; do
	# The same artwork over an opaque backdrop; qlmanage names the PNG after the file.
	python3 -I - "$SVG" "$TMP/$bg.svg" "$bg" <<'PY'
import re
import sys

src, dst, bg = sys.argv[1], sys.argv[2], sys.argv[3]
text = open(src, encoding="utf-8").read()
m = re.search(r'<svg\b[^>]*\bwidth="1024"[^>]*\bheight="1024"[^>]*>', text)
if not m:
    sys.exit(f"make-icons: {src}: the root <svg> must be 1024x1024 (width/height attributes)")
body = text[m.start():].replace("<?xml", "<!--", 1).replace("?>", "-->", 1)
with open(dst, "w", encoding="utf-8") as f:
    f.write('<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">'
            f'<rect width="100%" height="100%" fill="{bg}"/>{body}</svg>')
PY
	qlmanage -t -s 1024 -o "$TMP" "$TMP/$bg.svg" >/dev/null 2>&1 || true
	[ -f "$TMP/$bg.svg.png" ] || {
		echo "make-icons: QuickLook produced no PNG for $TMP/$bg.svg" >&2
		exit 1
	}
done

echo "==> recovering alpha; writing icon.png, icon.ico and the iconset"
python3 -I - "$TMP/white.svg.png" "$TMP/black.svg.png" "$TMP/${ICNS%.icns}.iconset" "$OUT" <<'PY'
import os
import sys

import numpy as np
from PIL import Image

white_png, black_png, iconset, out = sys.argv[1:5]
white = np.asarray(Image.open(white_png).convert("RGB"), dtype=np.float32)
black = np.asarray(Image.open(black_png).convert("RGB"), dtype=np.float32)
if white.shape != (1024, 1024, 3) or black.shape != (1024, 1024, 3):
    sys.exit(f"make-icons: expected two 1024x1024 renders, got {white.shape[:2]} and {black.shape[:2]}")
alpha = np.clip(1.0 - (white - black).mean(axis=2) / 255.0, 0.0, 1.0)
safe = np.where(alpha > 1e-3, alpha, 1.0)[..., None]
rgb = np.clip(black / safe, 0, 255)
img = Image.fromarray(np.dstack([rgb, alpha[..., None] * 255.0]).round().astype(np.uint8), "RGBA")

# Sanity: the canvas corners (outside the tile) are fully transparent, the tile centre opaque.
corners = [img.getpixel(p)[3] for p in ((0, 0), (1023, 0), (0, 1023), (1023, 1023))]
if max(corners) != 0 or img.getpixel((512, 512))[3] != 255:
    sys.exit(f"make-icons: alpha recovery failed: corner alpha {corners}, centre alpha {img.getpixel((512, 512))[3]}")


def resized(size):
    return img.resize((size, size), Image.Resampling.LANCZOS)


# Linux icon.
resized(512).save(os.path.join(out, "icon.png"), optimize=True)

# Windows icon: Pillow writes one PNG-compressed entry per size.
sizes = [(s, s) for s in (16, 24, 32, 48, 64, 128, 256)]
resized(256).save(os.path.join(out, "icon.ico"), format="ICO", sizes=sizes)

# macOS iconset for iconutil: @1x and @2x of 16, 32, 128, 256, 512.
os.makedirs(iconset, exist_ok=True)
for base in (16, 32, 128, 256, 512):
    resized(base).save(os.path.join(iconset, f"icon_{base}x{base}.png"))
    resized(base * 2).save(os.path.join(iconset, f"icon_{base}x{base}@2x.png"))
print("    icon.png, icon.ico, iconset ok (corner alpha 0, centre alpha 255)")
PY

echo "==> writing $ICNS"
iconutil -c icns "$TMP/${ICNS%.icns}.iconset" -o "$OUT/$ICNS"

ls -l "$OUT/icon.png" "$OUT/icon.ico" "$OUT/$ICNS"
