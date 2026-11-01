"""Art-direction studies, fourth set: ten more processes that make an image
without being a style — halftone, knit, string art, barcode, typewriter,
blueprint, fingerprint, marbling, papercut, and an origami crease pattern.

Every study renders the shared profile in geometry.py, so what differs is the
medium. Not app assets.

    python3 generate4.py
"""
from pathlib import Path
import math
import random

from geometry import (W, H, INK, LENS, TEMPLE, BRIDGE, MOUTH, NOSTRIL,
                      HAIR_STRANDS, catmull, d_of, ellipse, inside, in_hair,
                      edge_distance, outline_d, OUTLINE_POLY, shade_at,
                      glasses_tone, marching)

OUT = Path(__file__).parent


# ── 1. halftone screen ─────────────────────────────────────────────────
def halftone():
    """A rotated dot screen, the way a press separates a photograph into ink.
    Dot area carries the tone, so the picture is only as sharp as the screen."""
    parts = []
    pitch = 3.4
    ang = math.radians(45)
    n = 56
    for i in range(-n, n + 1):
        for j in range(-n, n + 1):
            x = 100 + (i * math.cos(ang) - j * math.sin(ang)) * pitch
            y = 100 + (i * math.sin(ang) + j * math.cos(ang)) * pitch
            if not (0.5 <= x < W - 0.5 and 0.5 <= y < H - 0.5):
                continue
            tone = shade_at(x, y)
            if not tone:
                continue
            r = 0.35 + 1.45 * math.sqrt(tone)
            parts.append(f'<circle cx="{x:.1f}" cy="{y:.1f}" r="{r:.2f}"/>')
    return f'<g fill="currentColor">{"".join(parts)}</g>'


# ── 2. knit ────────────────────────────────────────────────────────────
def knit():
    """Stockinette: every stitch is a V pulled through the one below it, so the
    surface is a stack of little chevrons, not a grid of squares."""
    parts = []
    w, h = 4.5, 3.4
    y = h
    while y < H:
        # No row offset: stitches sit in columns and each V nests into the one
        # below, with short legs running up to the row above.
        x = w / 2
        while x < W:
            tone = shade_at(x, y)
            if tone:
                weight = 0.85 + 1.5 * tone
                opacity = 0.42 + 0.55 * tone
                parts.append(
                    f'<path d="M{x - w/2:.1f},{y - h*1.35:.1f} '
                    f'L{x - w/2:.1f},{y - h*0.55:.1f} '
                    f'L{x:.1f},{y:.1f} '
                    f'L{x + w/2:.1f},{y - h*0.55:.1f} '
                    f'L{x + w/2:.1f},{y - h*1.35:.1f}" fill="none" '
                    f'stroke="currentColor" stroke-width="{weight:.2f}" '
                    f'stroke-linecap="round" stroke-linejoin="round" '
                    f'opacity="{opacity:.2f}"/>')
            x += w
        y += h
    return (f'<clipPath id="knit-clip"><path d="{outline_d()}"/></clipPath>'
            f'<g clip-path="url(#knit-clip)">{"".join(parts)}</g>')


# ── 3. string art ──────────────────────────────────────────────────────
def string_art():
    """Nails around a board, thread wound between them. The threads never touch
    the picture directly; the tone is only where strands happen to accumulate."""
    rnd = random.Random(404)
    nails = [(round(x, 1), round(y, 1)) for x, y in OUTLINE_POLY[::13]]
    gx = 48.0
    while gx < 150:
        gy = 40.0
        while gy < 182:
            if inside(gx, gy) and edge_distance(gx, gy) > 5:
                nails.append((round(gx + rnd.uniform(-3, 3), 1),
                              round(gy + rnd.uniform(-3, 3), 1)))
            gy += 11
        gx += 11

    threads, seen = [], set()
    for i, a in enumerate(nails):
        near = sorted(
            ((a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2, j)
            for j, b in enumerate(nails) if j != i)[:6]
        for d2, j in near:
            key = (min(i, j), max(i, j))
            if key in seen or d2 > 62 ** 2:
                continue
            seen.add(key)
            b = nails[j]
            tone = shade_at((a[0] + b[0]) / 2, (a[1] + b[1]) / 2) or 0.15
            threads.append(
                f'<path d="M{a[0]},{a[1]} L{b[0]},{b[1]}" stroke="currentColor" '
                f'stroke-width="0.55" opacity="{0.22 + 0.55 * tone:.2f}"/>')
    dots = "".join(f'<circle cx="{x}" cy="{y}" r="1.05" fill="currentColor"/>'
                   for x, y in nails)
    return "".join(threads) + dots


# ── 4. barcode ─────────────────────────────────────────────────────────
def barcode():
    """Bars of varying width, each cut to the height the subject occupies at
    that column. Read as data, then read as a face."""
    rnd = random.Random(505)
    parts = []
    x = 1.0
    while x < W:
        width = rnd.choice([0.7, 0.7, 0.7, 1.1, 1.1, 1.6, 2.2])
        cx = x + width / 2
        span = [y for y in range(2, H - 2) if shade_at(cx, y) is not None]
        if span:
            top, bottom = min(span), max(span)
            parts.append(f'<rect x="{x:.2f}" y="{top}" width="{width:.2f}" '
                         f'height="{bottom - top + 2}" fill="currentColor"/>')
        x += width + rnd.choice([0.6, 0.8, 0.8, 1.2, 1.8])
    return "".join(parts)


# ── 5. typewriter ──────────────────────────────────────────────────────
def typewriter():
    """A monospaced type frame. The letters are chosen by how much ink each
    glyph covers, so the face is a consequence of the alphabet."""
    parts = []
    cw, ch = 4.0, 5.0
    ramp = " .:-=+*#%@"
    y = ch
    while y < H:
        x = cw / 2
        while x < W:
            tone = shade_at(x, y)
            if tone:
                ch_ = ramp[min(len(ramp) - 1, int(tone * (len(ramp) - 1)))]
                if ch_ != " ":
                    parts.append(
                        f'<text x="{x:.1f}" y="{y:.1f}" font-family="monospace" '
                        f'font-size="5.6" fill="currentColor" text-anchor="middle">'
                        f'{ch_}</text>')
            x += cw
        y += ch
    return "".join(parts)


# ── 6. blueprint ───────────────────────────────────────────────────────
def blueprint():
    """A working drawing: construction grid, dimension arrows, and leaders to
    the parts that matter. Sparse by rule, because every mark has to be
    readable at print scale."""
    parts = []

    grid = []
    g = 0.0
    while g <= W:
        strong = abs(g % 50.0) < 0.01
        grid.append(f'<path d="M{g:.1f},0 L{g:.1f},{H}" stroke="currentColor" '
                    f'stroke-width="{0.5 if strong else 0.3:.1f}" '
                    f'opacity="{0.22 if strong else 0.12:.2f}"/>')
        grid.append(f'<path d="M0,{g:.1f} L{W},{g:.1f}" stroke="currentColor" '
                    f'stroke-width="{0.5 if strong else 0.3:.1f}" '
                    f'opacity="{0.22 if strong else 0.12:.2f}"/>')
        g += 10.0
    parts.append("".join(grid))

    # The part itself, drawn twice: outline and a 2px construction offset.
    parts.append(f'<path d="{outline_d()}" fill="none" stroke="currentColor" '
                 f'stroke-width="1.4" stroke-linejoin="round"/>')
    inner = [(x - 2, y - 2) for x, y in OUTLINE_POLY]
    parts.append(f'<path d="{d_of(inner, True)}" fill="none" stroke="currentColor" '
                 f'stroke-width="0.45" stroke-dasharray="3 2" opacity="0.7"/>')

    def arrow(x1, y1, x2, y2, head=3.0):
        a = math.atan2(y2 - y1, x2 - x1)
        hx, hy = math.cos(a) * head, math.sin(a) * head
        return (
            f'<path d="M{x1:.1f},{y1:.1f} L{x2:.1f},{y2:.1f}" stroke="currentColor" '
            f'stroke-width="0.6"/>'
            f'<path d="M{x2:.1f},{y2:.1f} L{x2 - hx + hy*0.45:.1f},{y2 - hy - hx*0.45:.1f} '
            f'M{x2:.1f},{y2:.1f} L{x2 - hx - hy*0.45:.1f},{y2 - hy + hx*0.45:.1f}" '
            f'stroke="currentColor" stroke-width="0.6"/>')

    # Vertical and horizontal dimensions of the part.
    parts.append(arrow(14, 36, 14, 187))
    parts.append(arrow(14, 36, 19, 36))
    parts.append(arrow(14, 187, 19, 187))
    parts.append(arrow(43, 194, 150, 194))
    parts.append(arrow(43, 194, 43, 189))
    parts.append(arrow(150, 194, 150, 189))

    # Leaders to the spectacles and the mouth.
    for lx, ly, tx, ty in ((150, 60, 168, 52), (139, 118, 168, 126)):
        parts.append(f'<path d="M{lx},{ly} L{tx},{ty} L{tx + 12},{ty}" '
                     f'stroke="currentColor" stroke-width="0.5" opacity="0.85"/>')
        parts.append(f'<circle cx="{lx}" cy="{ly}" r="1.4" fill="none" '
                     f'stroke="currentColor" stroke-width="0.5"/>')

    # Spectacles drawn as a dimensioned detail.
    parts.append(f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" '
                 f'stroke="currentColor" stroke-width="1.1"/>')
    parts.append(f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="currentColor" '
                 f'stroke-width="0.9"/>')
    parts.append(f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="currentColor" '
                 f'stroke-width="0.8"/>')
    return "".join(parts)


# ── 7. fingerprint ─────────────────────────────────────────────────────
def fingerprint():
    """Ridge flow from two interference centres. Where the two distance fields
    meet, the ridges have to loop and bifurcate, which is what makes a print."""
    step = 2.0
    xs = [i * step for i in range(int(W / step) + 1)]
    ys = [j * step for j in range(int(H / step) + 1)]
    core, delta = (120.0, 84.0), (100.0, 136.0)
    k = 0.78
    values = []
    for y in ys:
        row = []
        for x in xs:
            r1 = math.hypot(x - core[0], y - core[1])
            r2 = math.hypot(x - delta[0], y - delta[1])
            row.append(math.sin(k * (r1 + r2) + 0.55 * math.sin(0.09 * r1)))
        values.append(row)

    parts = []
    for level in (0.0, 0.45, -0.45, 0.85, -0.85):
        segs = marching(xs, ys, values, level)
        if not segs:
            continue
        d = " ".join(f"M{a[0]:.1f},{a[1]:.1f} L{b[0]:.1f},{b[1]:.1f}" for a, b in segs)
        parts.append(f'<path d="{d}" fill="none" stroke="currentColor" '
                     f'stroke-width="{1.0 if level == 0 else 0.55:.2f}" '
                     f'opacity="{1.0 if level == 0 else 0.55:.2f}" '
                     f'stroke-linecap="round"/>')
    return (f'<clipPath id="print-clip"><path d="{outline_d()}"/></clipPath>'
            f'<g clip-path="url(#print-clip)">{"".join(parts)}</g>')


# ── 8. marbling ────────────────────────────────────────────────────────
def marbling():
    """Paper marbling: ink floated on a size bath, then combed through. The
    pattern belongs to the fluid, so it ignores the subject entirely."""
    step = 2.0
    xs = [i * step for i in range(int(W / step) + 1)]
    ys = [j * step for j in range(int(H / step) + 1)]
    values = []
    for y in ys:
        row = []
        for x in xs:
            warp = (0.95 * math.sin(0.055 * y + 0.8)
                    + 0.70 * math.sin(0.040 * x + 1.6 * math.sin(0.030 * y))
                    + 0.45 * math.cos(0.025 * (x + y)))
            row.append(math.sin(0.205 * x + 0.145 * y + warp))
        values.append(row)

    parts = []
    for n in range(-4, 5):
        segs = marching(xs, ys, values, n * 0.5)
        if not segs:
            continue
        thick = n in (-1, 1)
        d = " ".join(f"M{a[0]:.1f},{a[1]:.1f} L{b[0]:.1f},{b[1]:.1f}" for a, b in segs)
        parts.append(f'<path d="{d}" fill="none" stroke="currentColor" '
                     f'stroke-width="{1.5 if thick else 0.7:.2f}" '
                     f'opacity="{0.95 if thick else 0.6:.2f}" '
                     f'stroke-linecap="round"/>')
    return (f'<clipPath id="marble-clip"><path d="{outline_d()}"/></clipPath>'
            f'<g clip-path="url(#marble-clip)">{"".join(parts)}</g>')


# ── 9. papercut ────────────────────────────────────────────────────────
def papercut():
    """Two sheets, cut and stacked. The back sheet is offset so the shadow does
    the modelling, and the features are holes cut clean through the top sheet."""
    parts = []
    back = [(x - 2.2, y - 2.2) for x, y in OUTLINE_POLY]
    parts.append(f'<path d="{d_of(back, True)}" fill="currentColor" opacity="0.3"/>')
    parts.append(f'<path d="{outline_d()}" fill="currentColor"/>')

    # Holes cut through the top sheet.
    parts.append(f'<path d="{d_of(ellipse(*LENS), True)}" fill="#fff"/>')
    parts.append(f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" '
                 f'stroke="currentColor" stroke-width="1.4"/>')
    parts.append(f'<path d="{d_of(catmull(MOUTH))}" fill="none" stroke="#fff" '
                 f'stroke-width="2.4" stroke-linecap="round"/>')
    parts.append(f'<path d="{d_of(catmull(BRIDGE))}" fill="none" stroke="#fff" '
                 f'stroke-width="2"/>')

    # Slits cut through the hair to separate the locks.
    for strand in HAIR_STRANDS[:3]:
        parts.append(f'<path d="{d_of(catmull(strand))}" fill="none" stroke="#fff" '
                     f'stroke-width="1.3" stroke-linecap="round"/>')
    parts.append(f'<circle cx="{NOSTRIL[0]}" cy="{NOSTRIL[1]}" r="1.1" fill="#fff"/>')
    return "".join(parts)


# ── 10. origami crease pattern ─────────────────────────────────────────
def origami():
    """A crease pattern: flat facets meeting along folds. Solid lines fold one
    way, dashed the other, which is all the depth the sheet can record."""
    rnd = random.Random(606)
    step = 15.0
    pts = {}
    rows = int(H / step) + 2
    cols = int(W / step) + 2
    for j in range(rows):
        for i in range(cols):
            x = i * step + (step / 2 if j % 2 else 0.0) + rnd.uniform(-2.5, 2.5)
            y = j * step + rnd.uniform(-2.5, 2.5)
            pts[(i, j)] = (x, y)

    mountain, valley = [], []
    for (i, j), a in pts.items():
        for di, dj in ((1, 0), (0, 1), (1, -1)):
            b = pts.get((i + di, j + dj))
            if b is None:
                continue
            mx, my = (a[0] + b[0]) / 2, (a[1] + b[1]) / 2
            if not inside(mx, my):
                continue
            seg = f"M{a[0]:.1f},{a[1]:.1f} L{b[0]:.1f},{b[1]:.1f}"
            (mountain if (i + j) % 2 == 0 else valley).append(seg)

    body = (f'<clipPath id="fold-clip"><path d="{outline_d()}"/></clipPath>'
            f'<g clip-path="url(#fold-clip)">'
            f'<path d="{" ".join(mountain)}" fill="none" stroke="currentColor" '
            f'stroke-width="0.65" opacity="0.9"/>'
            f'<path d="{" ".join(valley)}" fill="none" stroke="currentColor" '
            f'stroke-width="0.55" stroke-dasharray="2.4 1.8" opacity="0.75"/></g>')
    # The sheet edge itself folds, so it stays solid.
    body += (f'<path d="{outline_d()}" fill="none" stroke="currentColor" '
             f'stroke-width="1.5" stroke-linejoin="round"/>')
    body += (f'<path d="{d_of(ellipse(*LENS), True)}" fill="none" '
             f'stroke="currentColor" stroke-width="1.1"/>'
             f'<path d="{d_of(catmull(TEMPLE))}" fill="none" stroke="currentColor" '
             f'stroke-width="0.9" stroke-dasharray="2.4 1.8"/>')
    return body


STUDIES = [
    ("halftone.svg", "Halftone", halftone),
    ("knit.svg", "Knit", knit),
    ("string.svg", "String art", string_art),
    ("barcode.svg", "Barcode", barcode),
    ("typewriter.svg", "Typewriter", typewriter),
    ("blueprint.svg", "Blueprint", blueprint),
    ("fingerprint.svg", "Fingerprint", fingerprint),
    ("marble.svg", "Marbling", marbling),
    ("papercut.svg", "Papercut", papercut),
    ("origami.svg", "Origami", origami),
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
    (OUT / "comparison4.svg").write_text(contact_sheet(bodies))
    print("wrote comparison4.svg")


if __name__ == "__main__":
    main()
