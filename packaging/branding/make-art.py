#!/usr/bin/env python3
"""Generate the installer and file-type artwork from the brand assets.

    python3 -I packaging/branding/make-art.py        (macOS: needs Pillow and numpy)

Reads graphics/icon.png (the 512 px app icon, packaging/icons/make-icons.sh) and
the palette of docs/branding/README.md; writes the committed files below. Rerun
it after the icon or the palette changes.

  packaging/macos/dmg-background.tiff     the DMG window background, 660x400 @1x + @2x
  packaging/macos/schedule.icns           document icons for .fbs/.fws and .fbbible
  packaging/macos/bible.icns
  packaging/windows/wizard-large-*.bmp    Inno Setup WizardImageFile (100..200 % DPI)
  packaging/windows/wizard-small-*.bmp    Inno Setup WizardSmallImageFile
  packaging/windows/schedule.ico          document icons for the file associations
  packaging/windows/bible.ico
  packaging/linux/icons/hicolor/<n>x<n>/apps/freebeams.png
  packaging/linux/icons/hicolor/<n>x<n>/mimetypes/application-x-freebeams-{schedule,bible}.png
  packaging/linux/icons/hicolor/scalable/apps/freebeams.svg (a copy of graphics/icon.svg)
"""
import os
import shutil
import subprocess
import sys
import tempfile

from PIL import Image, ImageDraw, ImageFilter, ImageFont

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
ICON = Image.open(os.path.join(ROOT, "graphics", "icon.png")).convert("RGBA")

INDIGO_DEEP = (0x1A, 0x17, 0x50)
INDIGO_LIT = (0x47, 0x40, 0xD8)
GOLD = (0xFF, 0xC9, 0x4D)
WHITE = (0xFF, 0xFF, 0xFF)
TEXT_DARK = (0xF5, 0xF7, 0xFB)

FONTS = [
    "/System/Library/Fonts/Avenir Next.ttc",
    "/System/Library/Fonts/HelveticaNeue.ttc",
    "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
]


def font(size, bold=True):
    for path in FONTS:
        if os.path.exists(path):
            # Avenir Next: index 0 Bold, 7 Medium (ttc faces); others: face 0.
            index = 0
            if path.endswith("Avenir Next.ttc"):
                index = 0 if bold else 7
            try:
                return ImageFont.truetype(path, size, index=index)
            except OSError:
                continue
    sys.exit("make-art: no usable font (Avenir Next, Helvetica Neue or Arial Bold)")


def gradient(w, h, a=INDIGO_DEEP, b=INDIGO_LIT):
    """The brand's diagonal gradient: deep at the bottom-left, lit at the top-right."""
    import numpy as np

    y, x = np.mgrid[0:h, 0:w].astype("float32")
    t = (x / max(w - 1, 1) + (1 - y / max(h - 1, 1))) / 2
    rgb = np.stack([a[i] + (b[i] - a[i]) * t for i in range(3)], axis=-1)
    return Image.fromarray(rgb.clip(0, 255).astype("uint8"), "RGB")


def icon(size):
    return ICON.resize((size, size), Image.LANCZOS)


def out(*parts):
    path = os.path.join(ROOT, *parts)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    return path


# --- DMG background ---------------------------------------------------------------
def dmg_background(scale):
    """Finder draws the icon labels in black: the icons sit on a light body
    under an indigo header that carries the instruction."""
    w, h = 660 * scale, 400 * scale
    img = Image.new("RGBA", (w, h), TEXT_DARK + (255,))
    header = 104 * scale
    img.paste(gradient(w, header), (0, 0))
    d = ImageDraw.Draw(img)
    d.rectangle([0, header, w, header + 3 * scale], fill=GOLD + (255,))
    # A soft gold beam behind the arrow from the app slot to the Applications
    # slot (the icons sit at (170, 220) and (490, 220) in window points, see
    # bin/release-dmg).
    y = 220 * scale
    beam = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    ImageDraw.Draw(beam).polygon([(250 * scale, y - 8 * scale), (400 * scale, y - 34 * scale),
                                  (400 * scale, y + 34 * scale), (250 * scale, y + 8 * scale)], fill=GOLD + (110,))
    img = Image.alpha_composite(img, beam.filter(ImageFilter.GaussianBlur(8 * scale)))
    d = ImageDraw.Draw(img)
    x0, x1, t = 262 * scale, 398 * scale, 7 * scale
    d.rounded_rectangle([x0, y - t, x1 - 18 * scale, y + t], radius=t, fill=INDIGO_LIT + (255,))
    d.polygon([(x1 - 30 * scale, y - 24 * scale), (x1 + 4 * scale, y), (x1 - 30 * scale, y + 24 * scale)], fill=INDIGO_LIT + (255,))
    title = font(22 * scale)
    note = font(12 * scale, bold=False)
    text = "Drag FreeBeams to Applications to install it"
    tw = d.textlength(text, font=title)
    d.text(((w - tw) / 2, 38 * scale), text, font=title, fill=TEXT_DARK + (255,))
    small = "Free worship presentation software  ·  MIT License  ·  github.com/Devsecway/freebeams"
    sw = d.textlength(small, font=note)
    d.text(((w - sw) / 2, h - 40 * scale), small, font=note, fill=INDIGO_DEEP + (200,))
    return img.convert("RGB")


def write_dmg_background():
    with tempfile.TemporaryDirectory() as tmp:
        one, two = os.path.join(tmp, "bg.png"), os.path.join(tmp, "bg@2x.png")
        dmg_background(1).save(one, dpi=(72, 72))
        dmg_background(2).save(two, dpi=(144, 144))
        dst = out("packaging", "macos", "dmg-background.tiff")
        # One TIFF holding both: Finder picks the representation for the screen.
        subprocess.run(["tiffutil", "-cathidpicheck", one, two, "-out", dst], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        print("wrote", os.path.relpath(dst, ROOT))


# --- document icons ----------------------------------------------------------------
def document_icon(label, band):
    """A page with a folded corner, the app icon and a coloured label band, 1024 px."""
    s = 1024
    img = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    left, top, right, bottom, fold, r = 196, 72, 828, 952, 170, 40
    page = [(left + r, top), (right - fold, top), (right, top + fold), (right, bottom - r),
            (right - r, bottom), (left + r, bottom), (left, bottom - r), (left, top + r)]
    shadow = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    ImageDraw.Draw(shadow).polygon([(x, y + 14) for x, y in page], fill=(0, 0, 0, 90))
    img = Image.alpha_composite(img, shadow.filter(ImageFilter.GaussianBlur(16)))
    d = ImageDraw.Draw(img)
    d.polygon(page, fill=WHITE + (255,))
    for cx, cy in [(left + r, top + r), (left + r, bottom - r), (right - r, bottom - r)]:
        d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=WHITE + (255,))
    d.rectangle([left, top + r, left + r, bottom - r], fill=WHITE + (255,))
    d.polygon([(right - fold, top), (right - fold, top + fold), (right, top + fold)], fill=(0xD9, 0xDC, 0xEC, 255))
    d.polygon(page, outline=(0xC8, 0xCB, 0xDD, 255), width=6)
    img.alpha_composite(icon(440), (s // 2 - 220, 250))
    # Label band.
    d = ImageDraw.Draw(img)
    d.rectangle([left + 3, 730, right - 3, 850], fill=band + (255,))
    f = font(96)
    tw = d.textlength(label, font=f)
    ink = INDIGO_DEEP if band == GOLD else WHITE
    d.text(((s - tw) / 2, 735), label, font=f, fill=ink + (255,))
    return img


def write_icns(img, dst):
    with tempfile.TemporaryDirectory() as tmp:
        iconset = os.path.join(tmp, "doc.iconset")
        os.makedirs(iconset)
        for n in (16, 32, 128, 256, 512):
            img.resize((n, n), Image.LANCZOS).save(os.path.join(iconset, f"icon_{n}x{n}.png"))
            img.resize((n * 2, n * 2), Image.LANCZOS).save(os.path.join(iconset, f"icon_{n}x{n}@2x.png"))
        subprocess.run(["iconutil", "-c", "icns", iconset, "-o", dst], check=True)
    print("wrote", os.path.relpath(dst, ROOT))


def write_ico(img, dst):
    img.save(dst, format="ICO", sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])
    print("wrote", os.path.relpath(dst, ROOT))


# --- Inno Setup wizard images ----------------------------------------------------------
LARGE = [(164, 314), (205, 393), (246, 471), (287, 550), (328, 628)]  # 100 .. 200 % DPI
SMALL = [(55, 55), (69, 69), (83, 83), (97, 97), (110, 110)]


def wizard_large(w, h):
    img = gradient(w, h).convert("RGBA")
    k = w / 164
    n = int(112 * k)
    img.alpha_composite(icon(n), ((w - n) // 2, int(70 * k)))
    d = ImageDraw.Draw(img)
    f = font(int(22 * k))
    tw = d.textlength("FreeBeams", font=f)
    d.text(((w - tw) / 2, int(196 * k)), "FreeBeams", font=f, fill=TEXT_DARK + (255,))
    g = font(int(10 * k), bold=False)
    for i, line in enumerate(["Free worship", "presentation software"]):
        lw = d.textlength(line, font=g)
        d.text(((w - lw) / 2, int((232 + 15 * i) * k)), line, font=g, fill=GOLD + (255,))
    return img.convert("RGB")


def wizard_small(w, h):
    img = Image.new("RGBA", (w, h), WHITE + (255,))
    n = int(min(w, h) * 0.92)
    img.alpha_composite(icon(n), ((w - n) // 2, (h - n) // 2))
    return img.convert("RGB")


def write_wizard():
    for i, (w, h) in enumerate(LARGE):
        dst = out("packaging", "windows", f"wizard-large-{100 + 25 * i}.bmp")
        wizard_large(w, h).save(dst)
    for i, (w, h) in enumerate(SMALL):
        dst = out("packaging", "windows", f"wizard-small-{100 + 25 * i}.bmp")
        wizard_small(w, h).save(dst)
    print("wrote packaging/windows/wizard-{large,small}-{100..200}.bmp")


# --- Linux icon theme ---------------------------------------------------------------------
def write_linux(docs):
    base = ("packaging", "linux", "icons", "hicolor")
    for n in (16, 24, 32, 48, 64, 128, 256, 512):
        icon(n).save(out(*base, f"{n}x{n}", "apps", "freebeams.png"))
        for name, img in docs.items():
            img.resize((n, n), Image.LANCZOS).save(out(*base, f"{n}x{n}", "mimetypes", f"application-x-freebeams-{name}.png"))
    shutil.copyfile(os.path.join(ROOT, "graphics", "icon.svg"), out(*base, "scalable", "apps", "freebeams.svg"))
    print("wrote packaging/linux/icons/hicolor/*")


def main():
    write_dmg_background()
    docs = {"schedule": document_icon("SCHEDULE", INDIGO_LIT), "bible": document_icon("BIBLE", GOLD)}
    for name, img in docs.items():
        write_icns(img, out("packaging", "macos", f"{name}.icns"))
        write_ico(img, out("packaging", "windows", f"{name}.ico"))
    write_wizard()
    write_linux(docs)


if __name__ == "__main__":
    main()
