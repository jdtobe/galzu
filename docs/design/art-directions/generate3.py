"""Art-direction studies, third set: five more processes that produce an image
but are not usually called art styles — a loom, a circuit board, a spectrogram,
a contour map, and a star chart.

Every study renders the shared profile in geometry.py, so what differs is the
medium. Not app assets.

    python3 generate3.py
"""
from pathlib import Path
import math
import random

import geometry as G
from geometry import (W, H, INK, LENS, TEMPLE, BRIDGE, MOUTH, NOSTRIL,
                      HAIR_STRANDS, catmull, d_of, ellipse, inside, in_hair,
                      edge_distance, outline_d, OUTLINE_POLY)

OUT = Path(__file__).parent


def glasses_tone(x, y):
    """True where a point falls on the spectacle rim or the bridge."""
    cx, cy, rx, ry = LENS
    r = ((x - cx) / rx) ** 2 + ((y - cy) / ry) ** 2
    return 0.72 <= r <= 1.0


# ── 1. loom: deflected weft on a warp ──────────────────────────────────
def loom():
    """The image is the fabric. Threads alternate over and under on a plain
    weave, so tone comes from how thick and how dark each thread is."""
    cell = 3.6
    parts = []
    cols = int(W / cell) + 1
    rows = int(H / cell) + 1
    for j in range(rows):
        y = (j + 0.5) * cell
        for i in range(cols):
            x = (i + 0.5) * cell
            if not inside(x, y):
                continue
            tone = 0.30
            if in_hair(x, y):
                tone = 0.85
            if glasses_tone(x, y):
                tone = 1.0
            if edge_distance(x, y) < 3.0:
                tone = 1.0
            weight = 0.9 + 1.5 * tone
            opacity = 0.45 + 0.55 * tone
            half = cell / 2 - 0.35
            if (i + j) % 2 == 0:
                parts.append(
                    f'<path d="M{x - half:.2f},{y:.2f} L{x + half:.2f},{y:.2f}" '
                    f'stroke="currentColor" stroke-width="{weight:.2f}" '
                    f'stroke-linecap="round" opacity="{opacity:.2f}"/>')
            else:
                parts.append(
                    f'<path d="M{x:.2f},{y - half:.2f} L{x:.2f},{y + half:.2f}" '
                    f'stroke="currentColor" stroke-width="{weight:.2f}" '
                    f'stroke-linecap="round" opacity="{opacity:.2f}"/>')
    return "".join(parts)


# ── 2. circuit board: copper routed on a grid ──────────────────────────
def _route(a, b):
    """Route between two points using only horizontal, vertical and 45° runs."""
    x1, y1 = a
    x2, y2 = b
    dx, dy = x2 - x1, y2 - y1
    if abs(dx) < 0.6 or abs(dy) < 0.6:
        return [(x2, y2)]
    if abs(dx) > abs(dy):
        mx = x1 + math.copysign(abs(dy), dx)
        return [(round(mx), y2), (x2, y2)]
    my = y1 + math.copysign(abs(dx), dy)
    return [(x2, round(my)), (x2, y2)]


def _traces(points, closed=True):
    """Snap a sampled curve onto the router and return its vertices."""
    verts = [(round(x), round(y)) for x, y in points]
    if closed:
        verts.append(verts[0])
    out = [verts[0]]
    for a, b in zip(verts, verts[1:]):
        out.extend(_route(a, b))
    return out


def _trace_d(verts):
    return "M" + " L".join(f"{x},{y}" for x, y in verts)


def _pad(x, y, r, width):
    return (f'<circle cx="{x}" cy="{y}" r="{r}" fill="none" stroke="currentColor" '
            f'stroke-width="{width}"/><circle cx="{x}" cy="{y}" r="0.7" fill="currentColor"/>')


def circuit():
    parts = []
    # The edge is one continuous copper trace, snapped to the routing grid so it
    # keeps a slight pixel stagger but stays faithful to the profile.
    verts = [(round(x), round(y)) for x, y in OUTLINE_POLY[::5]]
    parts.append(f'<path d="{_trace_d(verts)}" fill="none" stroke="currentColor" '
                 f'stroke-width="3.2" stroke-linejoin="miter" stroke-linecap="square"/>')
    for x, y in verts[::9]:
        parts.append(_pad(x, y, 3.1, 1.9))

    # The spectacles as a keep-out ring with a via at each node.
    spec = _traces(ellipse(*LENS)[::6])
    parts.append(f'<path d="{_trace_d(spec)}" fill="none" stroke="currentColor" '
                 f'stroke-width="2.4" stroke-linejoin="miter"/>')
    for x, y in spec[::2]:
        parts.append(_pad(x, y, 1.7, 1.1))
    arm = _traces(catmull(TEMPLE), closed=False)
    parts.append(f'<path d="{_trace_d(arm)}" fill="none" stroke="currentColor" '
                 f'stroke-width="2.4" stroke-linejoin="miter"/>')
    for x, y in (arm[0], arm[-1]):
        parts.append(_pad(x, y, 2.2, 1.4))
    parts.append(_pad(*NOSTRIL, 1.4, 1.0))
    return "".join(parts)


# ── 3. spectrogram: energy binned against frequency ────────────────────
def spectrogram():
    """Time runs along the profile, frequency climbs the page. Each row is a
    filter band; the silhouette shows up as energy, and vertical ticks mark the
    analysis frames."""
    rnd = random.Random(202)
    rows = []
    y = 1.6
    while y < H:
        runs, start = [], None
        x = 1
        while x < W:
            ins = inside(x + 0.5, y)
            if ins and start is None:
                start = x
            elif not ins and start is not None:
                runs.append((start, x))
                start = None
            x += 1
        if start is not None:
            runs.append((start, W))

        for a, b in runs:
            if b - a < 2:
                continue
            mid = (a + b) / 2
            energy = 0.24 + 0.34 * rnd.random()
            width = 0.9 + 0.5 * energy
            if edge_distance(mid, y) < 3.5:
                energy = min(1.0, energy + 0.42)
                width = 1.9
            rows.append(f'<path d="M{a},{y:.1f} L{b},{y:.1f}" stroke="currentColor" '
                        f'stroke-width="{width:.2f}" opacity="{energy:.2f}"/>')
        y += 2.6

    # Analysis frames: short vertical ticks that break the bands like a real plot.
    frames = []
    fx = 2.0
    while fx < W:
        for fy in range(2, H - 2, 7):
            if not inside(fx, fy):
                continue
            frames.append(f'<path d="M{fx:.1f},{fy:.1f} L{fx:.1f},{fy + 3.4:.1f}" '
                          f'stroke="currentColor" stroke-width="0.6" opacity="0.35"/>')
        fx += 5.0

    # Reference harmonics across the whole frame.
    ref = []
    hy = 10.0
    while hy < H - 4:
        ref.append(f'<path d="M0,{hy:.1f} L{W},{hy:.1f}" stroke="currentColor" '
                   f'stroke-width="0.55" opacity="0.22"/>')
        hy += 19.0

    body = "".join(ref) + "".join(rows) + "".join(frames)
    body += (f'<path d="{outline_d()}" fill="none" stroke="currentColor" '
             f'stroke-width="1.1" opacity="0.9"/>'
             f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" stroke="currentColor" '
             f'stroke-width="1.6" opacity="0.95"/>'
             f'<path d="{d_of(catmull(BRIDGE))}" fill="none" stroke="currentColor" '
             f'stroke-width="1.2" opacity="0.9"/>')
    return body


# ── 4. contour map: level sets of the distance from the edge ───────────
def _field():
    step = 2.0
    xs = [i * step for i in range(int(W / step) + 1)]
    ys = [j * step for j in range(int(H / step) + 1)]
    h = []
    for y in ys:
        row = []
        for x in xs:
            d = edge_distance(x, y)
            row.append(d if inside(x, y) else -d)
        h.append(row)
    return xs, ys, h


def _marching(xs, ys, h, level):
    segs = []
    for j in range(len(ys) - 1):
        for i in range(len(xs) - 1):
            v = (h[j][i], h[j][i + 1], h[j + 1][i + 1], h[j + 1][i])
            corners = ((xs[i], ys[j]), (xs[i + 1], ys[j]),
                       (xs[i + 1], ys[j + 1]), (xs[i], ys[j + 1]))
            pts = []
            for k in range(4):
                a, b = v[k], v[(k + 1) % 4]
                if (a < level) != (b < level):
                    t = (level - a) / (b - a)
                    pa, pb = corners[k], corners[(k + 1) % 4]
                    pts.append((pa[0] + (pb[0] - pa[0]) * t,
                                pa[1] + (pb[1] - pa[1]) * t))
            if len(pts) == 2:
                segs.append((pts[0], pts[1]))
            elif len(pts) == 4:
                segs.append((pts[0], pts[1]))
                segs.append((pts[2], pts[3]))
    return segs


def topo():
    xs, ys, h = _field()
    levels = [2.0, 4.0, 6.0, 8.0, 10.0, 12.0, 14.0, 17.0, 20.0, 24.0, 29.0]
    parts = []
    for n, level in enumerate(levels):
        segs = _marching(xs, ys, h, level)
        if not segs:
            continue
        index = n % 3 == 0
        d = " ".join(f"M{a[0]:.1f},{a[1]:.1f} L{b[0]:.1f},{b[1]:.1f}" for a, b in segs)
        parts.append(f'<path d="{d}" fill="none" stroke="currentColor" '
                     f'stroke-width="{1.25 if index else 0.6:.2f}" '
                     f'opacity="{0.95 if index else 0.6:.2f}" '
                     f'stroke-linecap="round"/>')
    # The spectacles as surveyed features, not level sets.
    parts.append(f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" '
                 f'stroke="currentColor" stroke-width="1.3"/>')
    parts.append(f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="currentColor" '
                 f'stroke-width="1.2"/>')
    parts.append(f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="currentColor" '
                 f'stroke-width="1.1" stroke-linecap="round"/>')
    parts.append(f'<circle cx="{NOSTRIL[0]}" cy="{NOSTRIL[1]}" r="1.1" fill="currentColor"/>')
    return "".join(parts)


# ── 5. star chart: points and the lines between them ───────────────────
def star_chart():
    rnd = random.Random(303)
    stars = [(x, y) for x, y in OUTLINE_POLY[::18]]
    for strand in HAIR_STRANDS:
        stars.extend(catmull(strand, samples=10)[::14])

    # Interior stars, kept off the edge so they read as field stars.
    tries = 0
    while len(stars) < 78 and tries < 4000:
        tries += 1
        x, y = rnd.uniform(40, 152), rnd.uniform(34, 190)
        if not inside(x, y) or edge_distance(x, y) < 5:
            continue
        if any((x - sx) ** 2 + (y - sy) ** 2 < 64 for sx, sy in stars):
            continue
        stars.append((x, y))

    # Link each star to its nearest unlinked neighbour.
    links, used = [], set()
    for i, a in enumerate(stars):
        if i in used:
            continue
        best, bd = None, 1e9
        for j, b in enumerate(stars):
            if j == i or j in used:
                continue
            d = (a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2
            if d < bd:
                best, bd = j, d
        if best is not None and bd < 26 ** 2:
            used.add(i)
            used.add(best)
            links.append(f'M{a[0]:.1f},{a[1]:.1f} L{stars[best][0]:.1f},{stars[best][1]:.1f}')

    # A few brighter anchors along the profile.
    anchors = [stars[i] for i in range(0, len(stars), 11)]

    parts = []
    # Survey grid, faint.
    g = []
    for gx in range(0, W, 25):
        g.append(f'<path d="M{gx},0 L{gx},{H}" stroke="currentColor" stroke-width="0.35" '
                 f'opacity="0.13"/>')
    for gy in range(0, H, 25):
        g.append(f'<path d="M0,{gy} L{W},{gy}" stroke="currentColor" stroke-width="0.35" '
                 f'opacity="0.13"/>')
    parts.append("".join(g))

    parts.append(f'<path d="{" ".join(links)}" fill="none" stroke="currentColor" '
                 f'stroke-width="0.7" opacity="0.55"/>')

    for x, y in stars:
        r = 0.9 + rnd.random() * 0.8
        if (x, y) in anchors:
            r += 0.9
        parts.append(f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{r:.2f}" fill="currentColor"/>')
    # Four-pointed flare on the anchors.
    for x, y in anchors:
        parts.append(f'<path d="M{x-4.5:.1f},{y:.1f} L{x+4.5:.1f},{y:.1f} '
                     f'M{x:.1f},{y-4.5:.1f} L{x:.1f},{y+4.5:.1f}" stroke="currentColor" '
                     f'stroke-width="0.7" opacity="0.75"/>')
    return "".join(parts)


STUDIES = [
    ("loom.svg", "Loom", loom),
    ("circuit.svg", "Circuit", circuit),
    ("spectrum.svg", "Spectrogram", spectrogram),
    ("topo.svg", "Contour", topo),
    ("stars.svg", "Star chart", star_chart),
]


def standalone(body):
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
            f'width="{W}" height="{H}" color="{INK}">\n{body}\n</svg>\n')


def contact_sheet(bodies):
    cell, pad, label = 200, 16, 26
    w = len(bodies) * (cell + pad) + pad
    h = cell + pad * 3 + label
    cells = []
    for i, ((_, name, _), body) in enumerate(zip(STUDIES, bodies)):
        x = pad + i * (cell + pad)
        cells.append(
            f'<g transform="translate({x},{pad})">'
            f'<rect width="{cell}" height="{cell}" fill="#fff" stroke="#e3e3e3"/>'
            f'{body}</g>'
            f'<text x="{x + cell/2:.0f}" y="{pad + cell + 18}" text-anchor="middle" '
            f'font-family="Helvetica,Arial,sans-serif" font-size="12" '
            f'letter-spacing="0.4" fill="{INK}">{name}</text>')
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" '
            f'width="{w}" height="{h}" color="{INK}">'
            f'<rect width="{w}" height="{h}" fill="#fff"/>{"".join(cells)}</svg>\n')


def main():
    bodies = []
    for filename, _, render in STUDIES:
        body = render()
        bodies.append(body)
        (OUT / filename).write_text(standalone(body))
        print("wrote", filename)
    (OUT / "comparison3.svg").write_text(contact_sheet(bodies))
    print("wrote comparison3.svg")


if __name__ == "__main__":
    main()
