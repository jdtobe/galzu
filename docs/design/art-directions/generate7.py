"""Ternary hierarchy studies: nine marks built from "three groups of three
groups of three", arranged three by three.

Depth counts the grouping levels above a leaf:
    depth 2 -> 9 leaves      3 x 3
    depth 3 -> 27 leaves     3 x 3 x 3
    depth 4 -> 81 leaves     3 x 3 x 3 x 3

Monochrome, transparent ground. Not app assets.

    python3 generate7.py
"""
from pathlib import Path
import math

OUT = Path(__file__).parent
INK = "#141414"
S = 200
C = 100.0


def line(a, b, width=1.5, opacity=1.0):
    return (f'<path d="M{a[0]:.2f},{a[1]:.2f} L{b[0]:.2f},{b[1]:.2f}" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity}" '
            f'stroke-linecap="round"/>')


def node(p, r=2.0):
    return f'<circle cx="{p[0]:.2f}" cy="{p[1]:.2f}" r="{r}" fill="currentColor"/>'


def circle(cx, cy, r, width=1.4, opacity=1.0):
    return (f'<circle cx="{cx:.2f}" cy="{cy:.2f}" r="{r:.2f}" fill="none" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity}"/>')


def box(x, y, s, width=1.3, opacity=1.0):
    return (f'<rect x="{x:.2f}" y="{y:.2f}" width="{s:.2f}" height="{s:.2f}" '
            f'fill="none" stroke="currentColor" stroke-width="{width}" '
            f'opacity="{opacity}"/>')


def at(cx, cy, r, a):
    return (cx + r * math.cos(a), cy + r * math.sin(a))


def arc(cx, cy, r, a0, a1, n=18, width=1.4, opacity=1.0):
    pts = [at(cx, cy, r, a0 + (a1 - a0) * k / n) for k in range(n + 1)]
    d = "M" + " L".join(f"{x:.2f},{y:.2f}" for x, y in pts)
    return (f'<path d="{d}" fill="none" stroke="currentColor" stroke-width="{width}" '
            f'opacity="{opacity}" stroke-linecap="round"/>')


# ── 1. radial ternary tree, depth 3 (27 tips) ──────────────────────────
def tree(depth, length=27.0, spread=math.radians(31), root_angles=(90, 210, 330),
         tip=1.9):
    parts = [node((C, C), 3.0)]

    def grow(x, y, ang, ln, left):
        ex, ey = x + ln * math.cos(math.radians(ang)), y + ln * math.sin(math.radians(ang))
        parts.append(line((x, y), (ex, ey), 1.5 if left > 1 else 1.0,
                          1.0 if left > 1 else 0.75))
        if left == 1:
            parts.append(node((ex, ey), tip))
            return
        for d in (-1, 0, 1):
            grow(ex, ey, ang + math.degrees(d * spread), ln * 0.62, left - 1)

    for a in root_angles:
        grow(C, C, a, length, depth)
    return "".join(parts)


def tree_27():
    return tree(3)


def tree_81():
    return tree(4, length=24.0, tip=1.25)


# ── 3. ternary spiral: 3 arms, 3 rings, 3 nodes ────────────────────────
def spiral():
    """Three arms, three rings out from the centre, three nodes on each. Read
    outward, every ring is a third of the one inside it."""
    parts = [node((C, C), 3.4)]
    for arm in range(3):
        base = -math.pi / 2 + arm * 2 * math.pi / 3
        prev_ring = None
        for ring, rad in enumerate((30.0, 58.0, 86.0)):
            angles = [base + (k - 1) * 0.26 + ring * 0.10 for k in range(3)]
            pts = [at(C, C, rad, a) for a in angles]
            for k, q in enumerate(pts):
                if k < 2:
                    parts.append(line(q, pts[k + 1], 1.0, 0.5))
                # The innermost ring is tied back to the centre, so the arm is
                # rooted rather than floating.
                parts.append(line(q, prev_ring[k] if prev_ring else (C, C), 1.2, 0.7))
                parts.append(node(q, 2.1))
            prev_ring = pts
    return "".join(parts)


# ── 4. nested triad circles: 3 -> 9 -> 27 ──────────────────────────────
def nested_circles():
    parts = []
    for i in range(3):
        bx, by = at(C, C, 54, -math.pi / 2 + i * 2 * math.pi / 3)
        parts.append(circle(bx, by, 30, 1.6))
        for j in range(3):
            mx, my = at(bx, by, 17, -math.pi / 2 + j * 2 * math.pi / 3)
            parts.append(circle(mx, my, 9, 1.2, 0.8))
            for k in range(3):
                sx, sy = at(mx, my, 5.2, -math.pi / 2 + k * 2 * math.pi / 3)
                parts.append(circle(sx, sy, 2.6, 1.0, 0.9))
    return "".join(parts)


# ── 5. three grids inside three grids inside three grids ───────────────
def nested_grids():
    big = 58.0
    origin = C - big * 1.5
    parts = []
    for i in range(3):
        for j in range(3):
            bx, by = origin + i * big, origin + j * big
            parts.append(box(bx, by, big, 1.5))
            mid = big / 3
            for a in range(3):
                for b in range(3):
                    mx, my = bx + a * mid, by + b * mid
                    parts.append(box(mx, my, mid, 0.9, 0.55))
                    small = mid / 3
                    for c in range(3):
                        for d in range(3):
                            sx, sy = mx + c * small, my + d * small
                            parts.append(box(sx, sy, small, 0.55, 0.55))
    return "".join(parts)


# ── 6. sector rings: 3 rings, 3 sectors, 3 slices ──────────────────────
def sector_rings():
    parts = []
    radii = (26.0, 54.0, 82.0)
    for r in radii:
        parts.append(circle(C, C, r, 1.1, 0.45))
    for ring in range(3):
        r0, r1 = (0.0, radii[0])[0], radii[ring]
        inner = radii[ring - 1] if ring else 0.0
        for sec in range(3):
            a0 = -math.pi / 2 + sec * 2 * math.pi / 3
            a1 = a0 + 2 * math.pi / 3
            for k in range(3):
                b0 = a0 + (a1 - a0) * k / 3
                b1 = a0 + (a1 - a0) * (k + 1) / 3
                parts.append(arc(C, C, r1, b0, b1, 10, 1.6, 1.0))
                for t in (b0, b1):
                    parts.append(line(at(C, C, inner, t), at(C, C, r1, t), 0.8, 0.4))
                parts.append(node(at(C, C, r1, (b0 + b1) / 2), 1.8))
    parts.append(node((C, C), 3.0))
    return "".join(parts)


# ── 7. Sierpinski, three levels of removal ─────────────────────────────
def sierpinski(depth=3):
    verts = [at(C, C, 84, -math.pi / 2 + k * 2 * math.pi / 3) for k in range(3)]
    parts = []

    def mid(a, b):
        return ((a[0] + b[0]) / 2, (a[1] + b[1]) / 2)

    def walk(p1, p2, p3, left):
        if left == 0:
            parts.append(f'<path d="M{p1[0]:.2f},{p1[1]:.2f} L{p2[0]:.2f},{p2[1]:.2f} '
                         f'L{p3[0]:.2f},{p3[1]:.2f} Z" fill="currentColor"/>')
            return
        a, b, c = mid(p2, p3), mid(p1, p3), mid(p1, p2)
        walk(p1, c, b, left - 1)
        walk(c, p2, a, left - 1)
        walk(b, a, p3, left - 1)

    walk(*verts, depth)
    return "".join(parts)


# ── 8. cluster mesh: groups of groups of nodes ─────────────────────────
def cluster_mesh():
    parts = []
    hubs = [at(C, C, 58, -math.pi / 2 + i * 2 * math.pi / 3) for i in range(3)]
    for h in hubs:
        parts.append(node(h, 3.2))
    for i, h in enumerate(hubs):
        subs = [at(h[0], h[1], 24, -math.pi / 2 + i * 0.7 + j * 2 * math.pi / 3)
                for j in range(3)]
        for s in subs:
            parts.append(line(h, s, 1.2, 0.8))
            parts.append(node(s, 2.4))
            members = [at(s[0], s[1], 9, -math.pi / 2 + j * 2 * math.pi / 3)
                       for j in range(3)]
            for k, m in enumerate(members):
                parts.append(line(s, m, 0.9, 0.6))
                parts.append(line(m, members[(k + 1) % 3], 0.7, 0.4))
                parts.append(node(m, 1.6))
        # Each hub also links to the other hubs: three super-groups in one ring.
        parts.append(line(h, hubs[(i + 1) % 3], 1.4, 0.6))
    return "".join(parts)


# ── 9. lattice of lattices ─────────────────────────────────────────────
def lattice():
    parts = []
    size, gap = 52.0, 8.0
    span = size * 3 + gap * 2
    origin = C - span / 2
    for i in range(3):
        for j in range(3):
            ox, oy = origin + i * (size + gap), origin + j * (size + gap)
            pts = []
            for a in range(3):
                for b in range(3):
                    pts.append((ox + a * (size / 2), oy + b * (size / 2)))
            parts.append(box(ox - 4, oy - 4, size + 8, 1.4, 0.9))
            for p in pts:
                for q in pts:
                    if p == q:
                        continue
                    d = math.hypot(p[0] - q[0], p[1] - q[1])
                    if abs(d - size / 2) < 0.01 or abs(d - size / math.sqrt(2)) < 0.01:
                        parts.append(line(p, q, 0.6, 0.45))
            for p in pts:
                parts.append(node(p, 1.8))
    # The outer level: modules joined to their neighbours.
    for i in range(3):
        for j in range(3):
            ox, oy = origin + i * (size + gap) + size / 2, origin + j * (size + gap) + size / 2
            if i < 2:
                parts.append(line((ox, oy), (ox + size + gap, oy), 1.5, 0.75))
            if j < 2:
                parts.append(line((ox, oy), (ox, oy + size + gap), 1.5, 0.75))
    return "".join(parts)


MARKS = [
    ("ternary-tree-27.svg", "Tree · 27", tree_27),
    ("ternary-tree-81.svg", "Tree · 81", tree_81),
    ("ternary-spiral.svg", "Spiral · 27", spiral),
    ("nested-circles.svg", "Nested circles", nested_circles),
    ("nested-grids.svg", "Grids of grids", nested_grids),
    ("sector-rings.svg", "Sector rings", sector_rings),
    ("sierpinski.svg", "Sierpinski", sierpinski),
    ("cluster-mesh.svg", "Cluster mesh", cluster_mesh),
    ("lattice.svg", "Lattice of lattices", lattice),
]


def standalone(body):
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {S} {S}" '
            f'width="{S}" height="{S}" color="{INK}">\n{body}\n</svg>\n')


def sheet(bodies, columns=3, back=True):
    """White-backed by default; set back=False for the transparent export."""
    cell, gap, pad, label, head = 230, 22, 26, 28, 36
    rows = (len(bodies) + columns - 1) // columns
    w = pad * 2 + columns * cell + (columns - 1) * gap
    h = pad * 2 + head + rows * (cell + label) + (rows - 1) * gap
    parts = [
        f'<text x="{pad}" y="{pad + 15}" font-family="Helvetica,Arial,sans-serif" '
        f'font-size="12" letter-spacing="1.4" fill="{INK}" opacity="0.55">'
        f'THREE GROUPS OF THREE · NINE MARKS</text>']
    for i, ((_, name, _), body) in enumerate(zip(MARKS, bodies)):
        x = pad + (i % columns) * (cell + gap)
        y = pad + head + (i // columns) * (cell + label + gap)
        parts.append(
            f'<g transform="translate({x},{y})">'
            f'<rect width="{cell}" height="{cell}" fill="#ffffff" stroke="#dcdcdc"/>'
            f'<g transform="translate({(cell - S) / 2:.1f},{(cell - S) / 2:.1f})">{body}</g>'
            f'</g>'
            f'<text x="{x + cell/2:.0f}" y="{y + cell + 18}" text-anchor="middle" '
            f'font-family="Helvetica,Arial,sans-serif" font-size="12.5" '
            f'letter-spacing="0.3" fill="{INK}">{name}</text>')
    backdrop = f'<rect width="{w}" height="{h}" fill="#fff"/>' if back else ""
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" '
            f'width="{w}" height="{h}" color="{INK}">'
            f'{backdrop}{"".join(parts)}</svg>\n')


def main():
    bodies = []
    for filename, _, render in MARKS:
        body = render()
        bodies.append(body)
        (OUT / filename).write_text(standalone(body))
        print("wrote", filename)
    (OUT / "ternary.svg").write_text(sheet(bodies))
    print("wrote ternary.svg")


if __name__ == "__main__":
    main()
