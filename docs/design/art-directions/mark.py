"""The Trilobe mark for Galzu: three leaves on a stem, drawn for production.

Trilobe was picked from generate8.py. This rebuilds it as a mark rather than a
study:

- trilobe-mark.svg     master, 200 box, currentColor, full detail (roots)
- trilobe-small.svg    the 16 to 24 px cut: fatter leaves, solid stem, no roots
- lockup-galzu.svg     mark set against the wordmark
- sheet.svg            all of the above, white-backed, with size proofs

The master keeps its strokes thin because it is meant to be seen at 48 px and
up. Below that the roots and stem disappear, which is why the small cut exists
rather than one file scaled.

    python3 mark.py
"""
from pathlib import Path
import math

OUT = Path(__file__).parent / "mark"
INK = "#141414"
S = 200
C = 100.0


def pol(r, a, cx=C, cy=C):
    return (cx + r * math.cos(a), cy + r * math.sin(a))


def lens(base, tip, w):
    """A leaf: two quadratic curves between base and tip."""
    mx, my = (base[0] + tip[0]) / 2, (base[1] + tip[1]) / 2
    dx, dy = tip[0] - base[0], tip[1] - base[1]
    length = math.hypot(dx, dy)
    nx, ny = -dy / length, dx / length
    c1 = (mx + nx * w, my + ny * w)
    c2 = (mx - nx * w, my - ny * w)
    return (f'M{base[0]:.2f},{base[1]:.2f} '
            f'Q{c1[0]:.2f},{c1[1]:.2f} {tip[0]:.2f},{tip[1]:.2f} '
            f'Q{c2[0]:.2f},{c2[1]:.2f} {base[0]:.2f},{base[1]:.2f} Z')


def stroke(a, b, width, opacity=1.0):
    return (f'<path d="M{a[0]:.2f},{a[1]:.2f} L{b[0]:.2f},{b[1]:.2f}" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity}" '
            f'stroke-linecap="round"/>')


ANGLES = (math.radians(-90), math.radians(30), math.radians(150))


def master():
    """Full mark: three leaves, stem, three roots."""
    parts = []
    for a in ANGLES:
        parts.append(f'<path d="{lens(pol(6, a), pol(62, a), 17.0)}" '
                     f'fill="currentColor"/>')
    parts.append(stroke((C, C), (C, 168), 3.0))
    for a in (66, 90, 114):
        parts.append(stroke((C, 168), pol(24, math.radians(a), C, 168), 1.7, 0.85))
    parts.append(f'<circle cx="{C}" cy="{C}" r="6.2" fill="currentColor"/>')
    return "".join(parts)


def small():
    """The favicon cut. Every element is a filled shape at least 10 units wide,
    because at 16 px a 3-unit stroke is a fifth of a pixel and simply goes."""
    parts = []
    for a in ANGLES:
        parts.append(f'<path d="{lens(pol(4, a), pol(68, a), 24.0)}" '
                     f'fill="currentColor"/>')
    # Tapered stem as a filled shape: 18 units at the leaves, 11 at the foot.
    parts.append(f'<path d="M{C - 9:.1f},{C - 6:.1f} L{C + 9:.1f},{C - 6:.1f} '
                 f'L{C + 5.5:.1f},190 L{C - 5.5:.1f},190 Z" fill="currentColor"/>')
    return "".join(parts)


def wordmark(x, y, size, text="galzu"):
    """Placeholder lettering. Real work: draw or outline the wordmark."""
    return (f'<text x="{x}" y="{y}" font-family="Helvetica Neue,Helvetica,Arial,'
            f'sans-serif" font-size="{size}" font-weight="500" '
            f'letter-spacing="{size * 0.02:.2f}" fill="currentColor">{text}</text>')


def standalone(body, box=S, w=None, h=None):
    w = w or box
    h = h or box
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" '
            f'width="{w}" height="{h}" color="{INK}">\n{body}\n</svg>\n')


def lockup(text="galzu"):
    """Mark and wordmark. The mark is aligned to the optical centre of the
    type, not to its box: its ink is bottom-heavy thanks to the stem and roots,
    so the transform is offset down from a naive centring."""
    scale = 0.40
    ink_centre_y = 115.0        # the mark's ink centre in its own 200 box
    target_y = 62.5             # cap-height centre of 60 px type on a 84 baseline
    ty = target_y - ink_centre_y * scale
    parts = [f'<g transform="translate(0,{ty:.2f}) scale({scale})">{master()}</g>',
             wordmark(84, 84, 60, text)]
    return "".join(parts)


def sheet(bodies):
    """White-backed: master at size, both cuts at 48/24/16, the small cut
    isolated as a favicon would appear, and the lockup."""
    pad = 30
    w, h = 760, 600
    parts = [
        f'<rect width="{w}" height="{h}" fill="#ffffff"/>',
        f'<text x="{pad}" y="34" font-family="Helvetica,Arial,sans-serif" '
        f'font-size="12" letter-spacing="1.4" fill="{INK}" opacity="0.55">'
        f'GALZU · TRILOBE</text>',
    ]

    def label(x, y, text, size=12, opacity=1.0):
        return (f'<text x="{x}" y="{y}" font-family="Helvetica,Arial,sans-serif" '
                f'font-size="{size}" fill="{INK}" opacity="{opacity}">{text}</text>')

    # Row one: master at full size, then both cuts at 48, 24 and 16 px.
    top = 70
    parts.append(label(pad, top - 8, "Master", 12, 0.55))
    parts.append(f'<g transform="translate({pad},{top + 6}) scale(1)">'
                 f'{bodies["master"]}</g>')

    for col, key in ((280, "small"), (500, "master")):
        parts.append(label(col, top - 8, "Small cut" if key == "small"
                           else "Master scaled", 12, 0.55))
        y = top + 6
        for size in (48, 24, 16):
            parts.append(f'<g transform="translate({col},{y}) '
                         f'scale({size / S:.4f})">{bodies[key]}</g>')
            parts.append(label(col + 70, y + size / 2 + 4, f"{size} px", 11, 0.55))
            y += 60

    # Row two: the small cut on its own at favicon sizes.
    row2 = 320
    parts.append(label(pad, row2, "SMALL CUT ISOLATED", 11, 0.55))
    x = pad
    for size in (16, 24, 32, 48, 64):
        parts.append(f'<g transform="translate({x},{row2 + 14}) '
                     f'scale({size / S:.4f})">{bodies["small"]}</g>')
        parts.append(label(x, row2 + 14 + size + 14, str(size), 10, 0.45))
        x += size + 34

    # Row three: the lockup, drawn inline so it scales with this sheet.
    row3 = 470
    parts.append(label(pad, row3, "LOCKUP", 11, 0.55))
    parts.append(f'<g transform="translate({pad},{row3 + 10}) '
                 f'scale({0.72})">{bodies["lockup"]}</g>')

    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" '
            f'width="{w}" height="{h}" color="{INK}">{"".join(parts)}</svg>\n')


def main():
    OUT.mkdir(exist_ok=True)
    m, s = master(), small()
    (OUT / "trilobe-mark.svg").write_text(standalone(m))
    (OUT / "trilobe-small.svg").write_text(standalone(s))
    lock = lockup("galzu")
    (OUT / "lockup-galzu.svg").write_text(standalone(lock, w=330, h=110))
    (OUT / "sheet.svg").write_text(sheet({"master": m, "small": s, "lockup": lock}))
    print("wrote mark/: trilobe-mark.svg, trilobe-small.svg, "
          "lockup-galzu.svg, sheet.svg")


if __name__ == "__main__":
    main()
