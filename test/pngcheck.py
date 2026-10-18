#!/usr/bin/env python3
"""Pixel assertions on screenshots (pure Python: zlib only, no PIL).

  pngcheck.py FILE black                      # >= 99.9% of pixels are pure black
  pngcheck.py FILE notblank                   # the image has real content (not one flat color)
  pngcheck.py FILE has RRGGBB X0 Y0 X1 Y1 [N] # >= N (default 200) pixels of that color
                                              # (+-3 per channel) inside the region given
                                              # as fractions of the width/height

Exit status 0 when the assertion holds; a message on stderr otherwise.
"""
import struct
import sys
import zlib


def read_png(path):
    data = open(path, "rb").read()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG")
    pos, idat, w = 8, b"", 0
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        chunk = data[pos + 8:pos + 8 + n]
        pos += 12 + n
        if kind == b"IHDR":
            w, h, depth, ctype, _, _, interlace = struct.unpack(">IIBBBBB", chunk)
            if depth != 8 or ctype not in (2, 6) or interlace:
                raise ValueError("unsupported PNG (depth %d, color type %d)" % (depth, ctype))
            bpp = 3 if ctype == 2 else 4
        elif kind == b"IDAT":
            idat += chunk
    raw = zlib.decompress(idat)
    stride = w * bpp
    rows, prev = [], bytearray(stride)
    for y in range(h):
        f = raw[y * (stride + 1)]
        line = bytearray(raw[y * (stride + 1) + 1:(y + 1) * (stride + 1)])
        for i in range(stride):
            a = line[i - bpp] if i >= bpp else 0
            b = prev[i]
            c = prev[i - bpp] if i >= bpp else 0
            if f == 1:
                line[i] = (line[i] + a) & 255
            elif f == 2:
                line[i] = (line[i] + b) & 255
            elif f == 3:
                line[i] = (line[i] + (a + b) // 2) & 255
            elif f == 4:
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                pr = a if pa <= pb and pa <= pc else (b if pb <= pc else c)
                line[i] = (line[i] + pr) & 255
        rows.append(bytes(line))
        prev = line
    return w, h, bpp, rows


def pixels(w, h, bpp, rows, x0=0.0, y0=0.0, x1=1.0, y1=1.0, step=1):
    for y in range(int(y0 * h), int(y1 * h), step):
        row = rows[y]
        for x in range(int(x0 * w), int(x1 * w), step):
            i = x * bpp
            yield row[i], row[i + 1], row[i + 2]


def main():
    path, what = sys.argv[1], sys.argv[2]
    w, h, bpp, rows = read_png(path)
    if what == "black":
        total = nonblack = 0
        for r, g, b in pixels(w, h, bpp, rows, step=2):
            total += 1
            nonblack += (r, g, b) != (0, 0, 0)
        if nonblack > total * 0.001:
            sys.exit("%s: %d of %d sampled pixels are not black" % (path, nonblack, total))
    elif what == "notblank":
        colors = set()
        for px in pixels(w, h, bpp, rows, step=7):
            colors.add(px)
            if len(colors) > 50:
                return
        sys.exit("%s: only %d distinct colors; looks blank" % (path, len(colors)))
    elif what == "has":
        want = tuple(int(sys.argv[3][i:i + 2], 16) for i in (0, 2, 4))
        x0, y0, x1, y1 = (float(v) for v in sys.argv[4:8])
        need = int(sys.argv[8]) if len(sys.argv) > 8 else 200
        n = sum(1 for px in pixels(w, h, bpp, rows, x0, y0, x1, y1)
                if all(abs(px[i] - want[i]) <= 3 for i in range(3)))
        if n < need:
            sys.exit("%s: %d pixels of #%s in the region, want >= %d" % (path, n, sys.argv[3], need))
    else:
        sys.exit("unknown check " + what)


if __name__ == "__main__":
    main()
