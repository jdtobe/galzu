"""Art-direction studies, fifth set: ten incarnations of the string-art idea.

If a nail is an agent and a thread is a message, then the picture is not the
subject at all — it is the shape a conversation leaves behind. Each study takes
a different answer to "who talks to whom".

Every study renders the shared profile in geometry.py. Not app assets.

    python3 generate5.py
"""
from pathlib import Path
import math
import random

from geometry import (W, H, INK, LENS, TEMPLE, MOUTH, NOSTRIL, catmull, d_of,
                      ellipse, inside, edge_distance, outline_d, OUTLINE_POLY,
                      shade_at)

OUT = Path(__file__).parent


def nails(spacing=13, interior=14, margin=6):
    """Nails around the edge, then loosely through the middle."""
    rnd = random.Random(1234)
    out = [(round(x, 1), round(y, 1)) for x, y in OUTLINE_POLY[::spacing]]
    x = 46.0
    while x < 152:
        y = 38.0
        while y < 184:
            if inside(x, y) and edge_distance(x, y) > margin:
                out.append((round(x + rnd.uniform(-2, 2), 1),
                            round(y + rnd.uniform(-2, 2), 1)))
            y += interior
        x += interior
    return out


def pin(x, y, r=1.1):
    return f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{r}" fill="currentColor"/>'


def rim(opacity=0.22):
    return (f'<path d="{outline_d()}" fill="none" stroke="currentColor" '
            f'stroke-width="0.55" opacity="{opacity}"/>')


def thread(a, b, opacity, width=0.55):
    return (f'<path d="M{a[0]:.1f},{a[1]:.1f} L{b[0]:.1f},{b[1]:.1f}" '
            f'stroke="currentColor" stroke-width="{width}" opacity="{opacity:.2f}"/>')


# ── 1. one thread ──────────────────────────────────────────────────────
def one_thread():
    """A single unbroken thread visiting every nail once. The whole
    conversation is one path; nobody gets to start over."""
    pts = nails()
    cx, cy = 100.0, 106.0
    tour = sorted(pts, key=lambda p: (
        round(math.hypot(p[0] - cx, p[1] - cy) / 13.0),
        math.atan2(p[1] - cy, p[0] - cx)))
    path = "M" + " L".join(f"{x:.1f},{y:.1f}" for x, y in tour)
    return (rim()
            + f'<path d="{path}" fill="none" stroke="currentColor" stroke-width="0.8" '
              f'opacity="0.45" stroke-linejoin="round"/>'
            + "".join(pin(x, y, 0.95) for x, y in pts))


# ── 2. hub and spokes ──────────────────────────────────────────────────
def hub():
    """A few agents carry most of the traffic. Everyone else only reaches a
    hub, so the leaders are legible as knots of thread."""
    pts = nails()
    hubs = [(124.0, 86.0), (134.0, 119.0), (100.0, 50.0)]
    parts = []
    for p in pts:
        h = min(hubs, key=lambda q: (p[0] - q[0]) ** 2 + (p[1] - q[1]) ** 2)
        tone = shade_at((p[0] + h[0]) / 2, (p[1] + h[1]) / 2) or 0.2
        parts.append(thread(h, p, 0.20 + 0.45 * tone))
    for h in hubs:
        parts.append(f'<circle cx="{h[0]}" cy="{h[1]}" r="3.4" fill="none" '
                     f'stroke="currentColor" stroke-width="1.3"/>')
        parts.append(f'<circle cx="{h[0]}" cy="{h[1]}" r="1.2" fill="currentColor"/>')
    parts.append("".join(pin(x, y, 0.85) for x, y in pts))
    return "".join(parts)


# ── 3. everyone talks to their neighbours ──────────────────────────────
def _delaunay(pts):
    minx = min(p[0] for p in pts)
    maxx = max(p[0] for p in pts)
    miny = min(p[1] for p in pts)
    maxy = max(p[1] for p in pts)
    d = max(maxx - minx, maxy - miny) * 20
    midx, midy = (minx + maxx) / 2, (miny + maxy) / 2
    super_tri = ((midx - d, midy - d), (midx, midy + d), (midx + d, midy - d))
    tris = [super_tri]

    def inside_circle(p, t):
        (ax, ay), (bx, by), (cx, cy) = t
        ax, ay, bx, by, cx, cy = ax - p[0], ay - p[1], bx - p[0], by - p[1], cx - p[0], cy - p[1]
        det = ((ax * ax + ay * ay) * (bx * cy - by * cx)
               - (bx * bx + by * by) * (ax * cy - ay * cx)
               + (cx * cx + cy * cy) * (ax * by - ay * bx))
        area = (bx - ax) * (cy - ay) - (by - ay) * (cx - ax)
        return det * (1 if area > 0 else -1) > 0

    for p in pts:
        bad = [t for t in tris if inside_circle(p, t)]
        edges = {}
        for t in bad:
            for k in range(3):
                e = tuple(sorted([t[k], t[(k + 1) % 3]]))
                edges[e] = edges.get(e, 0) + 1
        tris = [t for t in tris if t not in bad]
        for (a, b), count in edges.items():
            if count == 1:
                tris.append((a, b, p))
    out = []
    for t in tris:
        if any(v in super_tri for v in t):
            continue
        for k in range(3):
            e = tuple(sorted([t[k], t[(k + 1) % 3]]))
            if e not in out:
                out.append(e)
    return out


def delone():
    """The cheapest network that keeps every agent in touch with its closest
    peers. Strip the redundancy and this is what is left."""
    pts = nails()
    edges = _delaunay(pts)
    parts = []
    for a, b in edges:
        tone = shade_at((a[0] + b[0]) / 2, (a[1] + b[1]) / 2) or 0.2
        parts.append(thread(a, b, 0.25 + 0.5 * tone, 0.6))
    parts.append("".join(pin(x, y, 0.9) for x, y in pts))
    return "".join(parts)


# ── 4. wound web ───────────────────────────────────────────────────────
def web():
    """Thread wound around the rim and tied to a centre, the way people
    actually make these. Structure before content."""
    parts = []
    cx, cy = 108.0, 104.0
    rings = 7
    for k in range(1, rings + 1):
        s = 1.0 - k * 0.105
        ring = [(cx + (x - cx) * s, cy + (y - cy) * s) for x, y in OUTLINE_POLY]
        parts.append(f'<path d="{d_of(ring, True)}" fill="none" stroke="currentColor" '
                     f'stroke-width="0.5" opacity="{0.55 - k*0.03:.2f}"/>')
    for k in range(0, 72):
        a = 2 * math.pi * k / 72
        step = 1.0
        r = 1.0
        while r < 130 and inside(cx + math.cos(a) * r, cy + math.sin(a) * r):
            r += step
        r -= step
        if r < 6:
            continue
        edge = (cx + math.cos(a) * r, cy + math.sin(a) * r)
        parts.append(thread((cx, cy), edge, 0.35, 0.5))
    parts.append(f'<circle cx="{cx}" cy="{cy}" r="2.4" fill="currentColor"/>')
    return "".join(parts)


# ── 5. slack thread ────────────────────────────────────────────────────
def catenary():
    """Thread under its own weight. Long runs sag, short runs stay taut, so the
    network hangs rather than points."""
    pts = nails()
    rnd = random.Random(77)
    parts = []
    for i, a in enumerate(pts):
        near = sorted(((a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2, j)
                      for j, b in enumerate(pts) if j != i)[:3]
        for d2, j in near:
            if d2 > 40 ** 2:
                continue
            b = pts[j]
            length = math.sqrt(d2)
            sag = length * 0.16
            mx, my = (a[0] + b[0]) / 2, (a[1] + b[1]) / 2
            nx, ny = -(b[1] - a[1]) / max(length, 1e-9), (b[0] - a[0]) / max(length, 1e-9)
            cxp, cyp = mx + nx * sag, my + ny * sag
            tone = shade_at(mx, my) or 0.2
            parts.append(
                f'<path d="M{a[0]:.1f},{a[1]:.1f} Q{cxp:.1f},{cyp:.1f} '
                f'{b[0]:.1f},{b[1]:.1f}" fill="none" stroke="currentColor" '
                f'stroke-width="0.6" opacity="{0.22 + 0.5 * tone:.2f}"/>')
    parts.append("".join(pin(x, y, 0.85) for x, y in pts))
    return "".join(parts)


# ── 6. nails only ──────────────────────────────────────────────────────
def pins():
    """No thread at all: just the hardware, placed where the image wants it.
    The network is implied by the pins waiting to be strung."""
    parts = []
    step = 4.6
    y = 2.0
    while y < H:
        x = 2.0
        while x < W:
            tone = shade_at(x, y)
            if tone:
                r = 0.55 + 1.45 * tone
                parts.append(pin(x, y, round(r, 2)))
            x += step
        y += step
    return "".join(parts)


# ── 7. directed traffic ────────────────────────────────────────────────
def directed():
    """Messages have a sender. Arrows make the asymmetry visible; the quiet
    agents are the ones nothing points away from."""
    pts = nails()
    rnd = random.Random(88)
    parts = [rim(0.18)]
    for i, a in enumerate(pts):
        near = sorted(((a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2, j)
                      for j, b in enumerate(pts) if j != i)[:1]
        for d2, j in near:
            if d2 > 30 ** 2:
                continue
            b = pts[j]
            ang = math.atan2(b[1] - a[1], b[0] - a[0])
            hx, hy = math.cos(ang) * 4.2, math.sin(ang) * 4.2
            tone = shade_at(a[0], a[1]) or 0.2
            parts.append(thread(a, b, 0.25 + 0.45 * tone, 0.55))
            parts.append(
                f'<path d="M{b[0]:.1f},{b[1]:.1f} '
                f'L{b[0] - hx + hy * 0.45:.1f},{b[1] - hy - hx * 0.45:.1f} '
                f'M{b[0]:.1f},{b[1]:.1f} '
                f'L{b[0] - hx - hy * 0.45:.1f},{b[1] - hy + hx * 0.45:.1f}" '
                f'stroke="currentColor" stroke-width="0.55" '
                f'opacity="{0.3 + 0.5 * tone:.2f}"/>')
    parts.append("".join(pin(x, y, 0.8) for x, y in pts))
    return "".join(parts)


# ── 8. density as tone ─────────────────────────────────────────────────
def weighted():
    """Chords thrown at random, each as faint as the place it lands is busy.
    Nothing is drawn deliberately; the face is an artefact of where traffic
    happens to concentrate."""
    pts = nails(spacing=11, interior=12, margin=5)
    rnd = random.Random(99)
    parts = []
    for _ in range(1400):
        a = rnd.choice(pts)
        b = rnd.choice(pts)
        d2 = (a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2
        if not (6 ** 2 < d2 < 46 ** 2):
            continue
        tone = shade_at((a[0] + b[0]) / 2, (a[1] + b[1]) / 2)
        if tone is None:
            continue
        parts.append(thread(a, b, 0.05 + 0.30 * tone, 0.5))
    parts.append("".join(pin(x, y, 0.8) for x, y in pts))
    return "".join(parts)


# ── 9. one origin ──────────────────────────────────────────────────────
def radial():
    """Every thread leaves the same point. Reads as a single voice addressing
    the whole room, and the subject is only what the far ends outline."""
    pts = nails(spacing=10, interior=12, margin=5)
    origin = (120.0, 87.0)
    parts = [rim(0.25)]
    for p in pts:
        tone = shade_at(p[0], p[1]) or 0.2
        parts.append(thread(origin, p, 0.06 + 0.34 * tone, 0.5))
    parts.append(f'<circle cx="{origin[0]}" cy="{origin[1]}" r="2.8" fill="currentColor"/>')
    return "".join(parts)


# ── 10. rounds ─────────────────────────────────────────────────────────
def rounds():
    """Four sessions stacked. Each round only strings the nails it touched, and
    later rounds sit slightly proud of earlier ones, so the history shows."""
    pts = nails(spacing=12, interior=13, margin=6)
    parts = []
    parts.append(rim(0.18))
    for r in range(4):
        drift = r * 0.45
        reach = 24 + r * 9
        keep = [p for p in pts
                if (shade_at(p[0], p[1]) or 0) * 4 >= r
                and (hash((round(p[0] / 6), r)) % 3) != 0]
        drawn = set()
        for i, a in enumerate(keep):
            near = sorted(((a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2, j)
                          for j, b in enumerate(keep) if j != i)[:2]
            for d2, j in near:
                key = (min(i, j), max(i, j))
                if key in drawn or d2 > reach ** 2:
                    continue
                drawn.add(key)
                b = keep[j]
                parts.append(
                    f'<path d="M{a[0]+drift:.1f},{a[1]+drift:.1f} '
                    f'L{b[0]+drift:.1f},{b[1]+drift:.1f}" stroke="currentColor" '
                    f'stroke-width="0.55" opacity="{0.42 - r*0.08:.2f}"/>')
        for x, y in keep[::3]:
            parts.append(pin(x + drift, y + drift, 0.8))
    return "".join(parts)


STUDIES = [
    ("one-thread.svg", "One thread", one_thread),
    ("hub.svg", "Hub and spokes", hub),
    ("delone.svg", "Nearest peers", delone),
    ("web.svg", "Wound web", web),
    ("slack.svg", "Slack thread", catenary),
    ("pins.svg", "Nails only", pins),
    ("directed.svg", "Directed", directed),
    ("traffic.svg", "Traffic density", weighted),
    ("radial.svg", "One origin", radial),
    ("rounds.svg", "Rounds", rounds),
]


def standalone(body):
    return (f'<?xml version="1.0" encoding="UTF-8"?>\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" '
            f'width="{W}" height="{H}" color="{INK}">\n{body}\n</svg>\n')


def contact_sheet(bodies, columns=5):
    cell, pad, label = 200, 16, 26
    rows = (len(bodies) + columns - 1) // columns
    w = columns * (cell + pad) + pad
    h = rows * (cell + pad + label) + pad
    cells = []
    for i, ((_, name, _), body) in enumerate(zip(STUDIES, bodies)):
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


def main():
    bodies = []
    for filename, _, render in STUDIES:
        body = render()
        bodies.append(body)
        (OUT / filename).write_text(standalone(body))
        print("wrote", filename)
    (OUT / "comparison5.svg").write_text(contact_sheet(bodies))
    print("wrote comparison5.svg")


if __name__ == "__main__":
    main()
