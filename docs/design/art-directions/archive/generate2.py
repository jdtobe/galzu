"""Art-direction studies, second set: five mediums that are not usually called
art styles — wire sculpture, palette knife, scratchboard, stencil, datamosh.

All five render one shared outline, so what differs is the medium, not the
drawing. Not app assets.

    python3 generate2.py
"""
from pathlib import Path
import math
import random

# The subject lives in geometry.py so every study renders the same profile.
from geometry import (W, H, INK, OUTLINE, HAIR_MASS, LENS, TEMPLE, BRIDGE,
                      MOUTH, NOSTRIL, BROW, HAIR_STRANDS, HAIR_POLY,
                      catmull, d_of, ellipse, outline_d, hair_d, inside_poly)

OUT = Path(__file__).parent


# ── 1. wire sculpture ──────────────────────────────────────────────────
def wire():
    rnd = random.Random(7)

    def jitter(pts, amt):
        return [(x + rnd.uniform(-amt, amt), y + rnd.uniform(-amt, amt)) for x, y in pts]

    loop = jitter(catmull(OUTLINE, True, samples=20), 0.45)
    # Spectacles are a separate piece of wire, bent and joined afterwards.
    spec = (jitter(ellipse(*LENS), 0.25)
            + jitter(catmull(BRIDGE), 0.2)
            + jitter(catmull(TEMPLE), 0.2))
    strands = "".join(
        f'<path d="{d_of(jitter(catmull(s), 0.5))}" fill="none" stroke="currentColor" '
        f'stroke-width="1.3" stroke-linecap="round" opacity="0.8"/>'
        for s in HAIR_STRANDS)

    body = (
        f'{strands}'
        f'<path d="{d_of(loop, True)}" fill="none" stroke="currentColor" '
        f'stroke-width="2.8" stroke-linejoin="round" stroke-linecap="round"/>'
        f'<path d="{d_of(spec)}" fill="none" stroke="currentColor" stroke-width="2.4" '
        f'stroke-linejoin="round" stroke-linecap="round"/>'
        f'<circle cx="120" cy="86" r="1.9" fill="currentColor"/>'
        f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="currentColor" '
        f'stroke-width="2" stroke-linecap="round"/>'
        f'<path d="{d_of(catmull(BROW))}" fill="none" stroke="currentColor" '
        f'stroke-width="1.8" stroke-linecap="round"/>'
    )
    return body


# ── 2. palette knife ───────────────────────────────────────────────────
def knife():
    rnd = random.Random(11)
    strokes = []

    def blade(cx, cy, length, width, angle, op):
        hx, hy = math.cos(angle) * length / 2, math.sin(angle) * length / 2
        nx, ny = -math.sin(angle) * width / 2, math.cos(angle) * width / 2
        pts = [(cx - hx, cy - hy),
               (cx + nx * 0.45, cy + ny * 0.45),
               (cx + hx + nx * 0.85, cy + hy + ny * 0.85),
               (cx + hx - nx * 0.85, cy + hy - ny * 0.85),
               (cx - nx * 0.45, cy - ny * 0.45)]
        strokes.append(f'<path d="{d_of(pts, True)}" fill="currentColor" '
                       f'opacity="{op:.2f}" stroke-linejoin="round"/>')
        # The ridge of paint catching the light.
        rx, ry = -math.sin(angle) * width * 0.22, math.cos(angle) * width * 0.22
        strokes.append(
            f'<path d="M{cx-hx*0.65+rx:.2f},{cy-hy*0.65+ry:.2f} '
            f'L{cx+hx*0.65+rx:.2f},{cy+hy*0.65+ry:.2f}" stroke="#fff" '
            f'stroke-width="{max(0.6, width*0.12):.1f}" stroke-linecap="round" '
            f'opacity="{min(0.32, op*0.34):.2f}"/>')

    # Long blades down the hair: dense, dark, following its fall.
    for _ in range(150):
        cx = rnd.uniform(44, 132)
        cy = rnd.uniform(36, 188)
        if not inside_poly(cx, cy, HAIR_POLY):
            continue
        angle = math.radians(rnd.uniform(76, 104))
        blade(cx, cy, rnd.uniform(22, 46), rnd.uniform(9, 18), angle,
              rnd.uniform(0.55, 0.95))

    # Face planes stay light and broad so the profile edge survives the paint.
    for _ in range(22):
        cx = rnd.uniform(108, 143)
        cy = rnd.uniform(70, 136)
        angle = math.radians(rnd.uniform(62, 112))
        blade(cx, cy, rnd.uniform(14, 26), rnd.uniform(10, 16), angle,
              rnd.uniform(0.18, 0.34))

    # Neck.
    for _ in range(6):
        blade(rnd.uniform(95, 103), rnd.uniform(146, 158), rnd.uniform(12, 18),
              rnd.uniform(8, 12), math.radians(rnd.uniform(82, 98)), 0.5)

    body = (f'<clipPath id="knife-clip"><path d="{outline_d()}"/></clipPath>'
            f'<g clip-path="url(#knife-clip)">{"".join(strokes)}</g>')
    # The profile is where the knife stopped, so the edge stays crisp.
    body += (f'<path d="{outline_d()}" fill="none" stroke="currentColor" '
             f'stroke-width="1.6" stroke-linejoin="round"/>')
    # Scraped-back ground reveals the features.
    body += (
        f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" stroke="#fff" stroke-width="3.4"/>'
        f'<path d="{d_of(catmull(BRIDGE))}" fill="none" stroke="#fff" stroke-width="3"/>'
        f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="#fff" stroke-width="3.2" '
        f'stroke-linecap="round"/>'
        f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="#fff" stroke-width="2.6" '
        f'stroke-linecap="round"/>'
        f'<circle cx="{NOSTRIL[0]}" cy="{NOSTRIL[1]}" r="1.6" fill="#fff"/>'
    )
    return body


# ── 3. scratchboard ────────────────────────────────────────────────────
def scratch():
    rnd = random.Random(23)
    marks = []

    # Hatch, clipping-free but confined by the outline clip. The angle turns
    # with the form instead of ringing around it.
    for _ in range(900):
        x = rnd.uniform(38, 154)
        y = rnd.uniform(32, 192)
        hair = inside_poly(x, y, HAIR_POLY)
        if hair:
            angle = math.radians(rnd.uniform(72, 108))
            length = rnd.uniform(8, 22)
            op = rnd.uniform(0.25, 0.6)
        else:
            angle = math.radians(rnd.uniform(-30, 30))
            length = rnd.uniform(6, 18)
            op = rnd.uniform(0.35, 0.85)
        marks.append(f'<path d="M{x:.1f},{y:.1f} l{length*math.cos(angle):.1f},'
                     f'{length*math.sin(angle):.1f}" stroke="#fff" stroke-width="0.7" '
                     f'opacity="{op:.2f}"/>')

    # A few long scratches that follow the profile rather than circling it.
    for d in ("M136,76 C139,88 146,98 149,104",
              "M138,112 C137,118 136,124 134,130",
              "M108,142 C104,150 103,156 103,161"):
        marks.append(f'<path d="{d}" fill="none" stroke="#fff" stroke-width="1" '
                     f'opacity="0.8"/>')
    for s in HAIR_STRANDS:
        marks.append(f'<path d="{d_of(catmull(s))}" fill="none" stroke="#fff" '
                     f'stroke-width="0.9" opacity="0.55"/>')

    marks += [
        f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" stroke="#fff" stroke-width="1.7"/>',
        f'<path d="{d_of(catmull(BRIDGE))}" fill="none" stroke="#fff" stroke-width="1.5"/>',
        f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="#fff" stroke-width="1.5"/>',
        f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="#fff" stroke-width="1.5" '
        f'stroke-linecap="round"/>',
        f'<circle cx="{NOSTRIL[0]}" cy="{NOSTRIL[1]}" r="1.1" fill="#fff"/>',
    ]
    # A black rim keeps the scratches from floating as a white cutout.
    return (f'<rect width="{W}" height="{H}" fill="currentColor"/>'
            f'<clipPath id="scratch-clip"><path d="{outline_d()}"/></clipPath>'
            f'<g clip-path="url(#scratch-clip)">{"".join(marks)}</g>'
            f'<path d="{outline_d()}" fill="none" stroke="currentColor" stroke-width="2.2" '
            f'stroke-linejoin="round"/>')


# ── 4. stencil ─────────────────────────────────────────────────────────
def stencil():
    gap = "#fff"
    bridges = [
        "M100,38 C100,58 101,76 100,92",          # crown split
        "M124,52 C128,70 129,84 128,98",          # temple split
        "M126,120 C122,132 117,141 110,150",      # cheek to jaw
        "M60,80 C54,110 50,140 47,172",           # hair split
        "M78,120 C72,146 67,168 62,186",          # hair split
        "M106,150 L92,152",                       # neck cut
    ]

    body = f'<path d="{outline_d()}" fill="currentColor"/>'
    for s in HAIR_STRANDS:
        body += (f'<path d="{d_of(catmull(s))}" fill="none" stroke="{gap}" '
                 f'stroke-width="1.6" stroke-linecap="round"/>')
    for b in bridges:
        body += (f'<path d="{b}" fill="none" stroke="{gap}" stroke-width="2.2" '
                 f'stroke-linecap="round"/>')

    body += (f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" stroke="{gap}" '
             f'stroke-width="3.4"/>'
             f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" stroke="currentColor" '
             f'stroke-width="1"/>'
             f'<path d="{d_of(catmull(BRIDGE))}" fill="none" stroke="{gap}" '
             f'stroke-width="2.8"/>'
             f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="{gap}" '
             f'stroke-width="3.4" stroke-linecap="round"/>'
             f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="{gap}" '
             f'stroke-width="2.4" stroke-linecap="round"/>')

    # Overspray drifting off the edge.
    rnd = random.Random(31)
    edge = catmull(OUTLINE, True, samples=20)
    for _ in range(110):
        x, y = rnd.choice(edge)
        a = rnd.uniform(0, 2 * math.pi)
        r = rnd.uniform(2.5, 13)
        body += (f'<circle cx="{x + math.cos(a)*r:.1f}" cy="{y + math.sin(a)*r:.1f}" '
                 f'r="{rnd.uniform(0.35, 1.2):.2f}" fill="currentColor" '
                 f'opacity="{rnd.uniform(0.12, 0.45):.2f}"/>')
    return body


# ── 5. datamosh ────────────────────────────────────────────────────────
def glitch():
    rnd = random.Random(47)
    sil = outline_d()

    smear = "".join(
        f'<path d="{sil}" fill="currentColor" opacity="{0.12 - i*0.03:.2f}" '
        f'transform="translate({-6*(i+1)},0)"/>' for i in range(3))

    lens_d = d_of(ellipse(*LENS), True)
    clean = (f'<path d="{lens_d}" fill="none" stroke="currentColor" stroke-width="2.6"/>'
             f'<path d="{d_of(catmull(BRIDGE))}" fill="none" stroke="currentColor" '
             f'stroke-width="2.2"/>'
             f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="currentColor" '
             f'stroke-width="2.2"/>'
             f'<circle cx="120" cy="86" r="1.9" fill="currentColor"/>'
             f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="currentColor" '
             f'stroke-width="2" stroke-linecap="round"/>')

    # Horizontal bands of a later frame, displaced over the current one.
    bands, y, i = [], 38.0, 0
    while y < 190:
        h = rnd.uniform(2.0, 8.0)
        dx = rnd.choice([-1, 1]) * rnd.uniform(2.0, 6.5)
        bid = f"band{i}"
        bands.append(f'<clipPath id="{bid}"><rect x="0" y="{y:.1f}" width="{W}" '
                     f'height="{h:.1f}"/></clipPath>'
                     f'<g clip-path="url(#{bid})"><path d="{sil}" fill="currentColor" '
                     f'transform="translate({dx:.1f},0)" opacity="{rnd.uniform(0.6,1.0):.2f}"/>'
                     f'</g>')
        y += h + rnd.uniform(2.5, 8)
        i += 1

    blocks = []
    for _ in range(10):
        bx, by = rnd.uniform(44, 148), rnd.uniform(40, 182)
        bw, bh = rnd.uniform(7, 24), rnd.uniform(2.5, 9)
        blocks.append(f'<rect x="{bx:.1f}" y="{by:.1f}" width="{bw:.1f}" height="{bh:.1f}" '
                      f'fill="#fff" opacity="{rnd.uniform(0.75,1):.2f}"/>')

    return (f'<rect width="{W}" height="{H}" fill="#fff"/>'
            f'{smear}<path d="{sil}" fill="currentColor"/>{clean}'
            f'{"".join(bands)}{"".join(blocks)}{clean}')


STUDIES = [
    ("wire.svg", "Wire", wire),
    ("knife.svg", "Palette knife", knife),
    ("scratch.svg", "Scratchboard", scratch),
    ("stencil.svg", "Stencil", stencil),
    ("glitch.svg", "Datamosh", glitch),
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
    (OUT / "comparison2.svg").write_text(contact_sheet(bodies))
    print("wrote comparison2.svg")


if __name__ == "__main__":
    main()
