#!/usr/bin/env bash
# Rasterise graphics/icon.svg (the master artwork) into the icon files the
# exports reference. The outputs are committed; rerun this after changing the SVG.
#
#   graphics/icon.png           512 px PNG: the Linux icon (.desktop, AppImage)
#   graphics/icon.ico           Windows icon, 16..256 px (export_presets.cfg,
#                               project.godot windows_native_icon)
#   graphics/FreeWorship.icns   macOS icon, 16..1024 px at @1x and @2x
#                               (export_presets.cfg, project.godot macos_native_icon)
#
# Runs on macOS: QuickLook (qlmanage) rasterises the SVG and iconutil writes the
# .icns. The resizes and the .ico come from Pillow (python3 -m pip install Pillow).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SVG="$ROOT/graphics/icon.svg"
OUT="$ROOT/graphics"

need() { # need COMMAND "where it comes from"
	command -v "$1" >/dev/null 2>&1 || {
		echo "make-icons: '$1' not found ($2)" >&2
		exit 1
	}
}
need qlmanage "macOS QuickLook; this script runs on macOS"
need iconutil "part of macOS"
need python3 "with the Pillow package"
python3 -I -c 'import PIL' 2>/dev/null || {
	echo "make-icons: the Python package Pillow is required: python3 -m pip install Pillow" >&2
	exit 1
}
[ -f "$SVG" ] || {
	echo "make-icons: $SVG not found" >&2
	exit 1
}

TMP="$(mktemp -d -t freeworship-icons)"
trap 'rm -rf "$TMP"' EXIT

echo "==> rasterising $SVG at 1024 px"
qlmanage -t -s 1024 -o "$TMP" "$SVG" >/dev/null 2>&1 || true
MASTER="$TMP/icon.svg.png"
[ -f "$MASTER" ] || {
	echo "make-icons: QuickLook produced no PNG for $SVG" >&2
	exit 1
}

echo "==> writing icon.png, icon.ico and the iconset"
python3 -I - "$MASTER" "$TMP/FreeWorship.iconset" "$OUT" <<'PY'
import os
import sys

from PIL import Image

master, iconset, out = sys.argv[1], sys.argv[2], sys.argv[3]
img = Image.open(master).convert("RGBA")
if img.size != (1024, 1024):
    sys.exit(f"make-icons: expected a 1024x1024 render, got {img.size}")


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
print("    icon.png, icon.ico, iconset ok")
PY

echo "==> writing FreeWorship.icns"
iconutil -c icns "$TMP/FreeWorship.iconset" -o "$OUT/FreeWorship.icns"

ls -l "$OUT/icon.png" "$OUT/icon.ico" "$OUT/FreeWorship.icns"
