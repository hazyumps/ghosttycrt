#!/usr/bin/env python3
"""Render a tmux capture into a PNG.

Takes the output of `tmux capture-pane -e -p` (a screen grid with SGR colour
escapes) and draws it with a monospace font, so the screenshots in the README
are the real interface rather than a mock-up. Regenerate with
scripts/screenshots.sh.

    render.py capture.txt out.png [--title "gcrt — sessions"]

Needs Pillow: python3 -m pip install --user pillow
"""

import argparse
import re
import sys

from PIL import Image, ImageDraw, ImageFont

FONT_CANDIDATES = [
    ("/System/Library/Fonts/Menlo.ttc", 0),
    ("/System/Library/Fonts/Menlo.ttc", 1),
    ("/System/Library/Fonts/SFNSMono.ttf", 0),
    ("/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 0),
]
BOLD_CANDIDATES = [
    ("/System/Library/Fonts/Menlo.ttc", 1),
    ("/System/Library/Fonts/SFNSMono.ttf", 0),
    ("/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf", 0),
]

# The app's own palette, used for anything the capture leaves at the default.
BG = (22, 22, 30)
FG = (228, 228, 228)
PAD = 22
TITLE_H = 34
RADIUS = 10
FONT_SIZE = 15
PALETTE16 = [
    (0, 0, 0), (205, 49, 49), (13, 188, 121), (229, 229, 16),
    (36, 114, 200), (188, 63, 188), (17, 168, 205), (229, 229, 229),
    (102, 102, 102), (241, 76, 76), (35, 209, 139), (245, 245, 67),
    (59, 142, 234), (214, 112, 214), (41, 184, 219), (255, 255, 255),
]

SGR = re.compile(r"\x1b\[([0-9;]*)m")


class Style:
    def __init__(self):
        self.fg = None
        self.bg = None
        self.bold = False
        self.reverse = False


def parse_line(line, style):
    """Yield (char, Style) for one captured line."""
    out = []
    i = 0
    while i < len(line):
        if line[i] == "\x1b":
            m = SGR.match(line, i)
            if m:
                apply_sgr(style, m.group(1))
                i = m.end()
                continue
            # skip any other escape (OSC etc.)
            j = i + 1
            while j < len(line) and line[j] not in "mH":
                j += 1
            i = j + 1
            continue
        out.append((line[i], StyleSnapshot(style)))
        i += 1
    return out


class StyleSnapshot:
    __slots__ = ("fg", "bg", "bold", "reverse")

    def __init__(self, s):
        self.fg, self.bg, self.bold, self.reverse = s.fg, s.bg, s.bold, s.reverse


def apply_sgr(style, params):
    if params == "":
        params = "0"
    parts = [int(p) for p in params.split(";") if p != ""] or [0]
    i = 0
    while i < len(parts):
        p = parts[i]
        if p == 0:
            style.fg = style.bg = None
            style.bold = style.reverse = False
        elif p == 1:
            style.bold = True
        elif p == 7:
            style.reverse = True
        elif p == 22:
            style.bold = False
        elif p == 27:
            style.reverse = False
        elif 30 <= p <= 37:
            style.fg = PALETTE16[p - 30]
        elif 90 <= p <= 97:
            style.fg = PALETTE16[p - 90 + 8]
        elif 40 <= p <= 47:
            style.bg = PALETTE16[p - 40]
        elif 100 <= p <= 107:
            style.bg = PALETTE16[p - 100 + 8]
        elif p in (38, 48) and i + 1 < len(parts):
            colour, consumed = read_extended(parts, i + 1)
            if p == 38:
                style.fg = colour
            else:
                style.bg = colour
            i += consumed
        i += 1


def read_extended(parts, i):
    if parts[i] == 5 and i + 1 < len(parts):
        return xterm256(parts[i + 1]), 2
    if parts[i] == 2 and i + 3 < len(parts):
        return (parts[i + 1], parts[i + 2], parts[i + 3]), 4
    return None, 1


def xterm256(n):
    if n < 16:
        return PALETTE16[n]
    if n < 232:
        n -= 16
        return (
            55 + (n // 36) * 40,
            55 + ((n // 6) % 6) * 40,
            55 + (n % 6) * 40,
        )
    v = 8 + (n - 232) * 10
    return (v, v, v)


def load_font(candidates, size):
    for path, index in candidates:
        try:
            return ImageFont.truetype(path, size, index=index)
        except OSError:
            continue
    return ImageFont.load_default()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("capture")
    ap.add_argument("output")
    ap.add_argument("--title", default="")
    args = ap.parse_args()

    with open(args.capture, "r", encoding="utf-8") as fh:
        raw = fh.read()
    lines = raw.split("\n")
    while lines and lines[-1].strip() == "":
        lines.pop()
    if not lines:
        sys.exit("capture is empty")

    font = load_font(FONT_CANDIDATES, FONT_SIZE)
    bold = load_font(BOLD_CANDIDATES, FONT_SIZE)

    # Cell metrics from the font itself, so the grid stays square.
    advance = font.getlength("M")
    cell_w = round(advance)
    cell_h = round(FONT_SIZE * 1.35)
    ascent = font.getmetrics()[0]

    cols = max(len(parse_line(l, Style())) for l in lines)
    rows = len(lines)

    width = PAD * 2 + cols * cell_w
    top = PAD + (TITLE_H if args.title else 0)
    height = top + rows * cell_h + PAD

    img = Image.new("RGBA", (width, height), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)
    draw.rounded_rectangle(
        [(0, 0), (width - 1, height - 1)], RADIUS, fill=BG,
        outline=(58, 58, 74), width=1,
    )

    if args.title:
        draw.ellipse([(PAD, 13), (PAD + 10, 23)], fill=(240, 96, 92))
        draw.ellipse([(PAD + 16, 13), (PAD + 26, 23)], fill=(245, 191, 79))
        draw.ellipse([(PAD + 32, 13), (PAD + 42, 23)], fill=(97, 197, 84))
        draw.text((PAD + 54, 10), args.title, font=font, fill=(150, 150, 168))

    for row, line in enumerate(lines):
        y = top + row * cell_h
        cells = parse_line(line, Style())
        for col, (ch, st) in enumerate(cells):
            if ch == " " and st.bg is None:
                continue
            fg, bg = st.fg or FG, st.bg
            if st.reverse:
                fg, bg = bg or BG, fg or FG
            x = PAD + col * cell_w
            if bg:
                draw.rectangle(
                    [(x, y), (x + cell_w, y + cell_h - 1)], fill=bg
                )
            draw.text((x, y + (cell_h - ascent) // 2 - 1), ch,
                      font=bold if st.bold else font, fill=fg)

    img.save(args.output)
    print(f"{args.output}  {width}x{height}  {cols}x{rows} cells")


if __name__ == "__main__":
    main()
