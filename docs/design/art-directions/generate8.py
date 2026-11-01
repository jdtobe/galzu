"""Germ marks: ten icons for the name "germ", drawn in the tree and gasket
idioms.

A germ is a seed that branches. That is the same recursion as the ternary tree
(split into three, split again) and the same silhouette as the Sierpinski
gasket (the parts are copies of the whole). These studies try each reading of
the word, from the botanical to the cellular to the negative-space one.

Monochrome, transparent ground. Not app assets.

    python3 generate8.py
"""
from pathlib import Path
import math

OUT = Path(__file__).parent
INK = "#141414"
S = 200          # canvas
C = 100.0        # centre


def pol(r, a, cx=C, cy=C):
    return (cx + r * math.cos(a), cy + r * math.sin(a))


def line(a, b, width=1.6, opacity=1.0):
    return (f'<path d="M{a[0]:.2f},{a[1]:.2f} L{b[0]:.2f},{b[1]:.2f}" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity}" '
            f'stroke-linecap="round"/>')


def poly(pts, width=1.6, opacity=1.0, close=False):
    d = "M" + " L".join(f"{x:.2f},{y:.2f}" for x, y in pts) + (" Z" if close else "")
    return (f'<path d="{d}" fill="none" stroke="currentColor" '
            f'stroke-width="{width}" opacity="{opacity}" stroke-linecap="round" '
            f'stroke-linejoin="round"/>')


def solid(pts):
    d = "M" + " L".join(f"{x:.2f},{y:.2f}" for x, y in pts) + " Z"
    return f'<path d="{d}" fill="currentColor"/>'


def disc(p, r=2.0):
    return f'<circle cx="{p[0]:.2f}" cy="{p[1]:.2f}" r="{r}" fill="currentColor"/>'


def ring(p, r, width=1.5, opacity=1.0):
    return (f'<circle cx="{p[0]:.2f}" cy="{p[1]:.2f}" r="{r}" fill="none" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity}"/>')


def arc(center, r, a0, a1, n=20, width=1.6, opacity=1.0):
    pts = [pol(r, a0 + (a1 - a0) * k / n, *center) for k in range(n + 1)]
    return poly(pts, width, opacity)


def leaf_triangles(radius, depth, cx=C, cy=C):
    """Filled Sierpinski triangles at the given radius, returned as bodies."""
    verts = [pol(radius, math.radians(a), cx, cy) for a in (-90, 30, 150)]
    out = []

    def mid(a, b):
        return ((a[0] + b[0]) / 2, (a[1] + b[1]) / 2)

    def walk(p1, p2, p3, left):
        if left == 0:
            out.append(solid([p1, p2, p3]))
            return
        a, b, c = mid(p2, p3), mid(p1, p3), mid(p1, p2)
        walk(p1, c, b, left - 1)
        walk(c, p2, a, left - 1)
        walk(b, a, p3, left - 1)

    walk(*verts, depth)
    return out, verts


# ── 1. seed: shoots up in threes, roots down ───────────────────────────
def seed():
    """The plainest reading: a seed low on the plate, one stem, three branches
    that each split in two, and a root system below. Growth has a direction."""
    base = (C, 126)
    parts = [disc(base, 6.5)]
    stem_top = (C, 100)
    parts.append(line(base, stem_top, 2.0))
    for a in (-112, -90, -68):
        p = pol(28, math.radians(a), *stem_top)
        parts.append(line(stem_top, p, 1.6))
        for b in (a - 15, a + 15):
            q = pol(46, math.radians(b), *stem_top)
            parts.append(line(p, q, 1.1, 0.85))
            parts.append(disc(q, 2.0))
        parts.append(disc(p, 2.4))
    for a in (62, 90, 118):
        p = pol(24, math.radians(a), *base)
        parts.append(line(base, p, 1.5, 0.9))
        for b in (a - 12, a + 12):
            q = pol(40, math.radians(b), *base)
            parts.append(line(p, q, 1.0, 0.6))
            parts.append(disc(q, 1.5))
    return "".join(parts)


# ── 2. gasket: the seed is itself a gasket ─────────────────────────────
def gasket():
    """The two marks as one: a Sierpinski triangle as the germ, and from each
    of its three corners a branch that splits three ways."""
    parts, verts = leaf_triangles(38, 2)
    for a in (-90, 30, 150):
        v = pol(38, math.radians(a))
        t = pol(58, math.radians(a))
        parts.append(line(v, t, 1.7))
        for d in (-20, 0, 20):
            q = pol(76, math.radians(a + d))
            parts.append(line(t, q, 1.1, 0.8))
            parts.append(disc(q, 2.0))
    return "".join(parts)


# ── 3. burst: the small-size workhorse ─────────────────────────────────
def burst():
    """Three arms, each throwing off two side strokes. Heavy and blunt on
    purpose: drawn to survive 16 px, where every other mark here dissolves."""
    parts = [disc((C, C), 8.0)]
    for a in (-90, 30, 150):
        parts.append(line(pol(9, math.radians(a)), pol(64, math.radians(a)), 2.8))
        parts.append(disc(pol(64, math.radians(a)), 2.6))
        for d in (-24, 24):
            b = a + d
            parts.append(line(pol(28, math.radians(b)), pol(54, math.radians(b)), 1.9))
            parts.append(disc(pol(54, math.radians(b)), 1.9))
    return "".join(parts)


# ── 4. cell: one cell divides three ways ───────────────────────────────
def cell():
    """A germ cell split into three sectors, each sending a thread out to a
    triad of its own. Division as structure."""
    parts = [ring((C, C), 30, 1.5)]
    for a in (-90, 30, 150):
        p = pol(30, math.radians(a))
        parts.append(line((C, C), p, 1.1, 0.9))
        t = pol(62, math.radians(a))
        parts.append(line(p, t, 1.6))
        trio = [pol(80, math.radians(a + d)) for d in (-17, 0, 17)]
        for i, q in enumerate(trio):
            parts.append(line(t, q, 1.0, 0.7))
            parts.append(line(q, trio[(i + 1) % 3], 0.8, 0.45))
            parts.append(disc(q, 1.9))
    parts.append(disc((C, C), 3.4))
    return "".join(parts)


# ── 5. trilobe: three cotyledons from one seed ─────────────────────────
def leaf_shape(base, tip, w):
    mx, my = (base[0] + tip[0]) / 2, (base[1] + tip[1]) / 2
    dx, dy = tip[0] - base[0], tip[1] - base[1]
    length = math.hypot(dx, dy)
    nx, ny = -dy / length, dx / length
    c1 = (mx + nx * w, my + ny * w)
    c2 = (mx - nx * w, my - ny * w)
    d = (f'M{base[0]:.2f},{base[1]:.2f} '
         f'Q{c1[0]:.2f},{c1[1]:.2f} {tip[0]:.2f},{tip[1]:.2f} '
         f'Q{c2[0]:.2f},{c2[1]:.2f} {base[0]:.2f},{base[1]:.2f} Z')
    return f'<path d="{d}" fill="currentColor"/>'


def trilobe():
    """A seed opening into three leaves, stem below. A cotyledon count biology
    does not offer, which is the point: three is the group size everywhere."""
    parts = []
    for a in (-90, 30, 150):
        base = pol(9, math.radians(a))
        tip = pol(57, math.radians(a))
        parts.append(leaf_shape(base, tip, 13))
    parts.append(line((C, C), (C, 158), 2.4))
    parts.append(line((C, 158), (C - 17, 176), 1.4, 0.85))
    parts.append(line((C, 158), (C + 17, 176), 1.4, 0.85))
    parts.append(line((C, 158), (C, 182), 1.4, 0.85))
    parts.append(disc((C, C), 5.5))
    return "".join(parts)


# ── 6. line: a pedigree of dividing cells ──────────────────────────────
def dividing(p, r, width=1.6):
    parts = [ring(p, r, width)]
    for a in (-90, 30, 150):
        parts.append(line(p, pol(r, math.radians(a), *p), 1.0, 0.9))
    return "".join(parts)


def germline():
    """Germline: one germ divides into three, each of those into three. Drawn
    as descent rather than growth, so it reads as ancestry, not a shrub."""
    parts = [dividing((C, 38), 9, 1.8)]
    for m in ((C - 46, 92), (C, 92), (C + 46, 92)):
        parts.append(line((C, 38), m, 1.4))
        parts.append(dividing(m, 6, 1.3))
        for d in (-14, 0, 14):
            tip = (m[0] + d, 152)
            parts.append(line(m, tip, 1.0, 0.75))
            parts.append(disc(tip, 2.0))
    return "".join(parts)


# ── 7. fiddle: three fronds unfurling ──────────────────────────────────
def fiddle():
    """A fern unfurls from the tip inward. Three of them off one germ: growth
    as a curl, which no straight branch can say."""
    parts = [disc((C, C), 5.0)]
    for base_deg in (-90, 30, 150):
        base = math.radians(base_deg)
        pts = []
        for k in range(34):
            t = k / 33.0
            r = 68 * (1 - 0.60 * t)
            pts.append(pol(r, base + 2.1 * t))
        parts.append(poly(pts, 1.8))
        parts.append(disc(pts[0], 2.0))
        parts.append(disc(pts[-1], 1.7))
    return "".join(parts)


# ── 8. void: the germ is the hole ──────────────────────────────────────
def germ_void():
    """A gasket at two levels, with three seeds floating in the void at the
    middle. The absent centre is where the growth comes from."""
    parts, _ = leaf_triangles(84, 2)
    seeds = [pol(13, math.radians(a)) for a in (-90, 30, 150)]
    for i, p in enumerate(seeds):
        parts.append(line(p, seeds[(i + 1) % 3], 1.0, 0.55))
        parts.append(disc(p, 3.0))
    return "".join(parts)


# ── 9. ring: a growth ring that sprouts ────────────────────────────────
def growth_ring():
    """One circular ring, broken at three points where the seeds sit, and three
    sprigs pushed outward. The circular reading of the same germ."""
    parts = []
    R = 56
    for a in (-90, 30, 150):
        parts.append(arc((C, C), R, math.radians(a + 15), math.radians(a + 105),
                         22, 1.9))
        mid = math.radians(a + 60)
        p = pol(R, mid)
        t = pol(R + 22, mid)
        parts.append(line(p, t, 1.5))
        for d in (-19, 19):
            q = pol(R + 32, math.radians(a + 60 + d))
            parts.append(line(t, q, 1.1, 0.8))
            parts.append(disc(q, 1.8))
        parts.append(disc(pol(R, math.radians(a)), 3.2))
    parts.append(disc((C, C), 4.5))
    return "".join(parts)


# ── 10. fan: three fronds of three leaflets ────────────────────────────
def fan():
    """Three fronds from one germ, each carrying leaflets. The most botanical
    of the set and the first to die below 24 px."""
    parts = [disc((C, C + 16), 5.0)]
    for a_deg in (-90, -90 - 42, -90 + 42):
        a = math.radians(a_deg)
        spine = [pol(k * 17, a, C, C + 16) for k in range(0, 5)]
        parts.append(poly(spine, 1.8))
        for i, p in enumerate(spine[1:], start=1):
            length = 15 - i * 1.8
            for side in (-1, 1):
                la = a + side * math.radians(52)
                q = (p[0] + length * math.cos(la), p[1] + length * math.sin(la))
                parts.append(line(p, q, 1.1, 0.85))
                parts.append(disc(q, 1.4))
    return "".join(parts)


# Optical scale: each mark was drawn inside its own bbox, so several sat small
# in the frame and lost their detail at 16 px. Scaling about the centre pushes
# each one out to the edge of the 200 box without redrawing it.
SCALES = {
    "germ-seed.svg": 1.20,
    "germ-gasket.svg": 1.22,
    "germ-burst.svg": 1.20,
    "germ-cell.svg": 1.18,
    "germ-trilobe.svg": 1.10,
    "germ-line.svg": 1.24,
    "germ-fiddle.svg": 1.18,
    "germ-void.svg": 1.12,
    "germ-ring.svg": 1.10,
    "germ-fan.svg": 1.14,
}


def framed(body, scale):
    if scale == 1.0:
        return body
    return (f'<g transform="translate({C},{C}) scale({scale}) '
            f'translate({-C},{-C})">{body}</g>')


MARKS = [
    ("germ-seed.svg", "Seed", seed),
    ("germ-gasket.svg", "Gasket germ", gasket),
    ("germ-burst.svg", "Burst", burst),
    ("germ-cell.svg", "Cell divides", cell),
    ("germ-trilobe.svg", "Trilobe", trilobe),
    ("germ-line.svg", "Germline", germline),
    ("germ-fiddle.svg", "Fiddlehead", fiddle),
    ("germ-void.svg", "Void germ", germ_void),
    ("germ-ring.svg", "Growth ring", growth_ring),
    ("germ-fan.svg", "Fronds", fan),
]


def standalone(body):
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}" '
            f'width="{S}" height="{S}" color="{INK}">\n{body}\n</svg>\n')


def compare(bodies, columns=5):
    cell, gap, pad, label, head = 240, 20, 26, 30, 34
    rows = (len(bodies) + columns - 1) // columns
    w = pad * 2 + columns * cell + (columns - 1) * gap
    h = pad * 2 + head + rows * (cell + label) + (rows - 1) * gap
    parts = [
        f'<text x="{pad}" y="{pad + 14}" font-family="Helvetica,Arial,sans-serif" '
        f'font-size="12" letter-spacing="1.4" fill="{INK}" opacity="0.55">'
        f'GERM · TEN MARKS</text>']
    for i, ((_, name, _), body) in enumerate(zip(MARKS, bodies)):
        x = pad + (i % columns) * (cell + gap)
        y = pad + head + (i // columns) * (cell + label + gap)
        parts.append(
            f'<g transform="translate({x},{y})">'
            f'<rect width="{cell}" height="{cell}" fill="#ffffff" stroke="#dcdcdc"/>'
            f'<g transform="scale({cell / S:.4f})">{body}</g></g>'
            f'<text x="{x + cell/2:.0f}" y="{y + cell + 19}" text-anchor="middle" '
            f'font-family="Helvetica,Arial,sans-serif" font-size="13" '
            f'letter-spacing="0.3" fill="{INK}">{name}</text>')
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" '
            f'width="{w}" height="{h}" color="{INK}">'
            f'<rect width="{w}" height="{h}" fill="#ffffff"/>{"".join(parts)}</svg>\n')


def small(bodies):
    """Every mark at 48, 24 and 16 px, because a mark that only works at 200 is
    a poster."""
    sizes = (48, 24, 16)
    name_w, gap = 170, 96
    row_h, pad, head = 60, 18, 24
    w = pad * 2 + name_w + gap * len(sizes)
    h = pad + head + row_h * len(bodies) + pad
    parts = [
        f'<text x="{pad}" y="{pad + 14}" font-family="Helvetica,Arial,sans-serif" '
        f'font-size="11" letter-spacing="1.2" fill="{INK}" opacity="0.55">'
        f'GERM AT THREE SIZES</text>']
    for j, size in enumerate(sizes):
        parts.append(f'<text x="{pad + name_w + j * gap}" y="{pad + 14}" '
                     f'font-family="Helvetica,Arial,sans-serif" font-size="11" '
                     f'fill="{INK}" opacity="0.55">{size} px</text>')
    for i, ((_, name, _), body) in enumerate(zip(MARKS, bodies)):
        y = pad + head + i * row_h
        mid = y + row_h / 2
        parts.append(f'<text x="{pad}" y="{mid + 4:.1f}" '
                     f'font-family="Helvetica,Arial,sans-serif" font-size="12" '
                     f'fill="{INK}">{name}</text>')
        for j, size in enumerate(sizes):
            x = pad + name_w + j * gap
            parts.append(f'<g transform="translate({x},{mid - size / 2:.1f}) '
                         f'scale({size / S:.4f})">{body}</g>')
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h:.0f}" '
            f'width="{w}" height="{h:.0f}" color="{INK}">'
            f'<rect width="{w}" height="{h:.0f}" fill="#ffffff"/>'
            f'{"".join(parts)}</svg>\n')


def main():
    bodies = []
    for filename, _, render in MARKS:
        body = framed(render(), SCALES[filename])
        bodies.append(body)
        (OUT / filename).write_text(standalone(body))
        print("wrote", filename)
    (OUT / "germ.svg").write_text(compare(bodies))
    (OUT / "germ-small.svg").write_text(small(bodies))
    print("wrote germ.svg and germ-small.svg")


if __name__ == "__main__":
    main()
