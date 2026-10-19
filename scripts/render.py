#!/usr/bin/env python3
"""Render an actual PTY terminal grid saved by screenshots.py as a PNG.

The grid includes native tmux status bars, borders, overlays and resolved SGR
attributes. This renderer does not invent interface text or window chrome.
"""
import argparse
import json

from PIL import Image, ImageDraw, ImageFont

BG = (40, 44, 52)
FG = (228, 228, 228)
FONT_SIZE = 16
PAD = 12
FONT_CANDIDATES = [
    ("/System/Library/Fonts/Menlo.ttc", 0),
    ("/System/Library/Fonts/SFNSMono.ttf", 0),
    ("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 0),
]
BOLD_CANDIDATES = [
    ("/System/Library/Fonts/Menlo.ttc", 1),
    ("/System/Library/Fonts/SFNSMono.ttf", 0),
    ("/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf", 0),
]
COLORS = {
    "black": "000000", "red": "cd3131", "green": "0dbc79", "brown": "e5e510",
    "blue": "2472c8", "magenta": "bc3fbc", "cyan": "11a8cd", "white": "e5e5e5",
    "brightblack": "666666", "brightred": "f14c4c", "brightgreen": "23d18b",
    "brightbrown": "f5f543", "brightblue": "3b8eea", "brightmagenta": "d670d6",
    "brightcyan": "29b8db", "brightwhite": "ffffff",
}


def font(candidates):
    for path, index in candidates:
        try:
            return ImageFont.truetype(path, FONT_SIZE, index=index)
        except OSError:
            pass
    raise RuntimeError("Install Menlo or DejaVu Sans Mono to render screenshots")


def color(value, default):
    if value == "default":
        return default
    value = COLORS.get(value, value)
    return tuple(int(value[i:i+2], 16) for i in (0, 2, 4))


def render(grid, output):
    regular, bold = font(FONT_CANDIDATES), font(BOLD_CANDIDATES)
    cw, ch = round(regular.getlength("M")), 22
    image = Image.new("RGB", (PAD * 2 + grid["columns"] * cw,
                              PAD * 2 + grid["lines"] * ch), BG)
    draw = ImageDraw.Draw(image)
    for row, cells in enumerate(grid["cells"]):
        for col, cell in enumerate(cells):
            fg, bg = color(cell["fg"], FG), color(cell["bg"], BG)
            if cell["reverse"]:
                fg, bg = bg, fg
            x, y = PAD + col * cw, PAD + row * ch
            draw.rectangle((x, y, x + cw - 1, y + ch - 1), fill=bg)
            if cell["data"] not in ("", " "):
                draw.text((x, y), cell["data"], font=bold if cell["bold"] else regular, fill=fg)
            if cell["underscore"]:
                draw.line((x, y + ch - 3, x + cw - 1, y + ch - 3), fill=fg)
    image.save(output)
    print(f"{output}: {image.width}x{image.height}", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("capture")
    parser.add_argument("output")
    args = parser.parse_args()
    with open(args.capture) as source:
        render(json.load(source), args.output)
