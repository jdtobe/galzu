"""Network mark studies: ten icons built from mesh, nesting, and circular
structure, with no profile in them at all.

Monochrome, transparent ground, no background tile. Each icon is drawn to read
as a silhouette first and a network second, so it survives a favicon.

    python3 generate6.py
"""
from pathlib import Path
import math

OUT = Path(__file__).parent
INK = "#141414"
S = 200          # canvas
C = 100.0        # centre


def pol(r, a):
    return (C + r * math.cos(a), C + r * math.sin(a))


def line(a, b, width=1.7, opacity=1.0):
    return (f'<path d="M{a[0]:.2f},{a[1]:.2f} L{b[0]:.2f},{b[1]:.2f}" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity}" '
            f'stroke-linecap="round"/>')


def ring(r, width=1.7, opacity=1.0, dash=None):
    extra = f' stroke-dasharray="{dash}"' if dash else ""
    return (f'<circle cx="{C}" cy="{C}" r="{r}" fill="none" stroke="currentColor" '
            f'stroke-width="{width}" opacity="{opacity}"{extra}/>')


def node(p, r=2.7):
    return f'<circle cx="{p[0]:.2f}" cy="{p[1]:.2f}" r="{r}" fill="currentColor"/>'


def hole(p, r=2.1):
    return (f'<circle cx="{p[0]:.2f}" cy="{p[1]:.2f}" r="{r}" fill="none" '
            f'stroke="currentColor" stroke-width="1.5"/>')


# ── 1. mesh band between two rings ─────────────────────────────────────
def band_mesh():
    """Agents on an outer rim, a smaller working set inside, and every thread
    crossing the gap. Nesting is the edge, mesh is the fill."""
    outer_n, inner_n = 12, 6
    o = [pol(84, 2 * math.pi * i / outer_n - math.pi / 2) for i in range(outer_n)]
    i_ = [pol(42, 2 * math.pi * i / inner_n - math.pi / 2) for i in range(inner_n)]
    parts = [ring(84), ring(42, 1.4)]
    for k, a in enumerate(o):
        parts.append(line(a, o[(k + 1) % outer_n], 1.0, 0.55))
    for k, a in enumerate(i_):
        parts.append(line(a, i_[(k + 1) % inner_n], 1.0, 0.7))
        # Each inner node triangulates to the two rim nodes nearest it.
        a_out = 2 * math.pi * k / inner_n
        for j in (0, 1):
            idx = round(a_out / (2 * math.pi / outer_n)) + 2 * j
            parts.append(line(a, o[idx % outer_n], 1.0, 0.5))
    parts += [node(p) for p in o] + [node(p, 2.3) for p in i_]
    return "".join(parts)


# ── 2. nested rings with spokes ────────────────────────────────────────
def nested_spokes():
    """Four rings, twelve spokes. The plainest possible statement of layered
    structure: every ring knows every other."""
    parts = []
    for r in (84, 58, 33):
        parts.append(ring(r, 1.6 if r == 84 else 1.2, 1.0 if r == 84 else 0.7))
    for k in range(12):
        a = 2 * math.pi * k / 12 - math.pi / 2
        parts.append(line(pol(12, a), pol(84, a), 1.5))
        for r in (33, 58):
            parts.append(node(pol(r, a), 1.9))
    for k in range(12):
        a = 2 * math.pi * k / 12 - math.pi / 2
        parts.append(node(pol(84, a), 2.4))
    parts.append(node((C, C), 3.4))
    return "".join(parts)


# ── 3. geodesic sphere ─────────────────────────────────────────────────
def geodesic():
    """A sphere wireframe: latitudes are straight lines, longitudes are
    ellipses. Two families of curves, one topology."""
    parts = [ring(84)]
    for k in (-0.72, -0.42, 0.0, 0.42, 0.72):
        half = 84 * math.sqrt(max(0.0, 1 - k * k))
        y = C + 84 * k
        parts.append(line((C - half, y), (C + half, y), 1.0, 0.65))
    for deg in (18, 42, 66):
        rx = 84 * math.cos(math.radians(deg))
        parts.append(f'<ellipse cx="{C}" cy="{C}" rx="{rx:.2f}" ry="84" fill="none" '
                     f'stroke="currentColor" stroke-width="1.0" opacity="0.65"/>')
    parts.append(line((C, C - 84), (C, C + 84), 1.0, 0.65))
    for k in range(8):
        parts.append(node(pol(84, 2 * math.pi * k / 8 - math.pi / 2), 2.5))
    return "".join(parts)


# ── 4. orbital hub ─────────────────────────────────────────────────────
def orbital():
    """One authority, three orbits, each carrying its own members. Layering is
    spatial here rather than concentric."""
    parts = [node((C, C), 4.2)]
    for deg in (0, 60, 120):
        a = math.radians(deg)
        rx, ry = 82.0, 30.0
        parts.append(f'<ellipse cx="{C}" cy="{C}" rx="{rx}" ry="{ry}" fill="none" '
                     f'stroke="currentColor" stroke-width="1.5" '
                     f'transform="rotate({deg} {C} {C})"/>')
        for t in (25, 155, 250):
            e = math.radians(t)
            x = rx * math.cos(e)
            y = ry * math.sin(e)
            px = C + x * math.cos(a) - y * math.sin(a)
            py = C + x * math.sin(a) + y * math.cos(a)
            parts.append(node((px, py), 2.5))
    return "".join(parts)


# ── 5. stacked layers ──────────────────────────────────────────────────
def stacked():
    """Three planes of the same network, offset in depth and wired together.
    The stack is the point; no single plane tells you about the others."""
    parts = []
    offsets = [(-26, 26), (0, 0), (26, -26)]
    cells = {}
    for layer, (dx, dy) in enumerate(offsets):
        # pol() already returns absolute canvas coordinates.
        pts = [pol(34, 2 * math.pi * k / 4 - math.pi / 4) for k in range(4)]
        pts = [(x + dx, y + dy) for x, y in pts]
        cells[layer] = pts
        mid = (C + dx, C + dy)
        parts.append(f'<path d="M{" L".join(f"{x:.1f},{y:.1f}" for x, y in pts)} Z" '
                     f'fill="none" stroke="currentColor" stroke-width="1.5"/>')
        for p in pts:
            parts.append(line(mid, p, 0.9, 0.5))
        parts.append(node(mid, 2.2))
        for p in pts:
            parts.append(node(p, 2.0))

    # Vertical wiring between corresponding nodes of adjacent layers.
    for layer in (0, 1):
        for a, b in zip(cells[layer], cells[layer + 1]):
            parts.append(line(a, b, 0.9, 0.45))
    return "".join(parts)


# ── 6. knot ring ───────────────────────────────────────────────────────
def knot():
    """A single thread that coils around a ring and crosses itself. The network
    is one continuous dependency with three lobes."""
    R, r, p, q = 50.0, 24.0, 2, 3
    path = []
    for k in range(241):
        t = 2 * math.pi * k / 240
        rad = R + r * math.cos(q * t)
        path.append((C + rad * math.cos(p * t), C + rad * math.sin(p * t)))
    d = "M" + " L".join(f"{x:.2f},{y:.2f}" for x, y in path)
    parts = [f'<path d="{d}" fill="none" stroke="currentColor" stroke-width="2.2" '
             f'stroke-linejoin="round" stroke-linecap="round"/>']
    for k in range(0, 240, 20):
        parts.append(node(path[k], 2.3))
    return "".join(parts)


# ── 7. nested hexagons ─────────────────────────────────────────────────
def nested_hex():
    """Hexagonal cells inside hexagonal cells. The honeycomb nest says scaling
    without saying growth chart."""
    parts = []
    rings = [(84, 0), (56, 12), (29, -8)]
    verts = []
    for radius, rot in rings:
        pts = [pol(radius, 2 * math.pi * k / 6 - math.pi / 2 + math.radians(rot))
               for k in range(6)]
        verts.append(pts)
        parts.append(f'<path d="M{" L".join(f"{x:.1f},{y:.1f}" for x, y in pts)} Z" '
                     f'fill="none" stroke="currentColor" stroke-width="1.6"/>')
    for a, b in zip(verts, verts[1:]):
        for p, q in zip(a, b):
            parts.append(line(p, q, 0.9, 0.45))
    for group in verts:
        for p in group:
            parts.append(node(p, 2.4))
    parts.append(node((C, C), 3.0))
    return "".join(parts)


# ── 8. void core ───────────────────────────────────────────────────────
def void_core():
    """A dense ring of mesh around a core that is deliberately not drawn. The
    absent centre is the most important node in the picture."""
    parts = []
    inner, outer = 40, 86
    spokes = 20
    for r in (inner, (inner + outer) / 2, outer):
        parts.append(ring(r, 1.1 if r != outer else 1.7, 0.6 if r != outer else 1.0))
    for k in range(spokes):
        a = 2 * math.pi * k / spokes - math.pi / 2
        parts.append(line(pol(inner, a), pol(outer, a), 1.0, 0.6))
        a2 = a + math.pi / spokes
        parts.append(line(pol(inner, a2), pol(outer, a), 0.9, 0.35))
        parts.append(node(pol(outer, a), 2.4))
        parts.append(node(pol(inner, a), 2.0))
    return "".join(parts)


# ── 9. small world ─────────────────────────────────────────────────────
def small_world():
    """Everyone knows their neighbours, and a handful of threads jump the ring.
    Two hops instead of six — the whole reason this product exists."""
    n = 16
    pts = [pol(80, 2 * math.pi * k / n - math.pi / 2) for k in range(n)]
    parts = [ring(80, 1.2, 0.45)]
    for k in range(n):
        for step in (1, 2):
            parts.append(line(pts[k], pts[(k + step) % n], 1.4, 0.75))
    for k, far in ((0, 7), (3, 10), (5, 13), (8, 15), (2, 11)):
        parts.append(line(pts[k], pts[far], 1.7, 1.0))
    for p in pts:
        parts.append(node(p, 2.6))
    return "".join(parts)


# ── 10. lattice nest ───────────────────────────────────────────────────
def lattice_nest():
    """A triangular lattice held inside two circles. The mesh would run off the
    page; the circles are what make it a mark."""
    parts = []
    step = 21.0
    h = step * math.sqrt(3) / 2
    grid = []
    row = 0
    y = C - 84
    while y <= C + 84:
        x = C - 84 - (step / 2 if row % 2 else 0)
        while x <= C + 84:
            if math.hypot(x - C, y - C) <= 78:
                grid.append((round(x, 2), round(y, 2)))
            x += step
        y += h
        row += 1
    pts = set(grid)
    for p in grid:
        for dx, dy in ((step, 0), (step / 2, h), (-step / 2, h)):
            q = (round(p[0] + dx, 2), round(p[1] + dy, 2))
            if q in pts:
                parts.append(line(p, q, 0.95, 0.5))
    parts.append(ring(84, 1.8))
    parts.append(ring(48, 1.2, 0.7))
    for p in grid:
        inside_core = math.hypot(p[0] - C, p[1] - C) <= 48
        parts.append(node(p, 1.5 if inside_core else 2.2))
    return "".join(parts)


ICONS = [
    ("band-mesh.svg", "Band mesh", band_mesh),
    ("nested-spokes.svg", "Nested spokes", nested_spokes),
    ("geodesic.svg", "Geodesic", geodesic),
    ("orbital.svg", "Orbital hub", orbital),
    ("stacked.svg", "Stacked layers", stacked),
    ("knot.svg", "Knot ring", knot),
    ("nested-hex.svg", "Nested hex", nested_hex),
    ("void-core.svg", "Void core", void_core),
    ("small-world.svg", "Small world", small_world),
    ("lattice-nest.svg", "Lattice nest", lattice_nest),
]


def standalone(body):
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}" '
            f'width="{S}" height="{S}" color="{INK}">\n{body}\n</svg>\n')


def sheet(bodies, columns=5):
    cell, pad, label = 200, 16, 26
    rows = (len(bodies) + columns - 1) // columns
    w = columns * (cell + pad) + pad
    h = rows * (cell + pad + label) + pad
    cells = []
    for i, ((_, name, _), body) in enumerate(zip(ICONS, bodies)):
        x = pad + (i % columns) * (cell + pad)
        y = pad + (i // columns) * (cell + pad + label)
        cells.append(
            f'<g transform="translate({x},{y})">'
            f'<rect width="{cell}" height="{cell}" fill="#fff" stroke="#e3e3e3"/>'
            f'{body}</g>'
            f'<text x="{x + cell/2:.0f}" y="{y + cell + 18}" text-anchor="middle" '
            f'font-family="Helvetica,Arial,sans-serif" font-size="12" '
            f'letter-spacing="0.4" fill="{INK}">{name}</text>')
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" '
            f'width="{w}" height="{h}" color="{INK}">'
            f'<rect width="{w}" height="{h}" fill="#fff"/>{"".join(cells)}</svg>\n')


def small_sheet(bodies):
    """The same icons at 48, 24 and 16 pixels, because a mark has to survive it."""
    sizes = (48, 24, 16)
    name_w, gap = 170, 96
    row_h, pad, head = 60, 18, 24
    w = pad * 2 + name_w + gap * len(sizes)
    h = pad + head + row_h * len(bodies) + pad
    parts = [
        f'<text x="{pad}" y="{pad + 14}" font-family="Helvetica,Arial,sans-serif" '
        f'font-size="11" letter-spacing="1.2" fill="{INK}" opacity="0.55">'
        f'ICON AT THREE SIZES</text>']
    for j, size in enumerate(sizes):
        x = pad + name_w + j * gap
        parts.append(f'<text x="{x}" y="{pad + 14}" font-family="Helvetica,Arial,sans-serif" '
                     f'font-size="11" fill="{INK}" opacity="0.55">{size} px</text>')
    for i, ((_, name, _), body) in enumerate(zip(ICONS, bodies)):
        y = pad + head + i * row_h
        mid = y + row_h / 2
        parts.append(f'<text x="{pad}" y="{mid + 4:.1f}" '
                     f'font-family="Helvetica,Arial,sans-serif" font-size="12" '
                     f'fill="{INK}">{name}</text>')
        for j, size in enumerate(sizes):
            x = pad + name_w + j * gap
            top = mid - size / 2
            parts.append(f'<g transform="translate({x},{top:.1f}) '
                         f'scale({size / S:.4f})">{body}</g>')
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h:.0f}" '
            f'width="{w}" height="{h:.0f}" color="{INK}">'
            f'<rect width="{w}" height="{h:.0f}" fill="#fff"/>{"".join(parts)}</svg>\n')


def compare_sheet(bodies, columns=5):
    """White-backed sheet for viewing in editors that paint transparency black.
    The production icons stay transparent; this is only for comparison."""
    cell, gap, pad, label, head = 240, 20, 26, 30, 34
    rows = (len(bodies) + columns - 1) // columns
    w = pad * 2 + columns * cell + (columns - 1) * gap
    h = pad * 2 + head + rows * (cell + label) + (rows - 1) * gap
    parts = [
        f'<text x="{pad}" y="{pad + 14}" font-family="Helvetica,Arial,sans-serif" '
        f'font-size="12" letter-spacing="1.4" fill="{INK}" opacity="0.55">'
        f'NETWORK MARK STUDIES · WHITE BACKING FOR COMPARISON ONLY</text>']
    for i, ((_, name, _), body) in enumerate(zip(ICONS, bodies)):
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


def main():
    bodies = []
    for filename, _, render in ICONS:
        body = render()
        bodies.append(body)
        (OUT / filename).write_text(standalone(body))
        print("wrote", filename)
    (OUT / "icons.svg").write_text(sheet(bodies))
    (OUT / "icons-small.svg").write_text(small_sheet(bodies))
    (OUT / "icons-compare.svg").write_text(compare_sheet(bodies))
    print("wrote icons.svg, icons-small.svg and icons-compare.svg")


if __name__ == "__main__":
    main()
