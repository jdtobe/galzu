"""Shared subject geometry for the art-direction studies.

One side profile, facing right, with long hair and spectacles. Every study
renders this same outline, so what differs between them is the medium and not
the drawing. Not an app asset.
"""
import math

W = H = 200
INK = "#141414"

# One closed outline: crown, hairline, forehead, brow, nose, lips, chin, jaw,
# neck, then the long hair falling behind the shoulder and back up over the
# skull. Hair covers the back of the head, so the skull never appears alone.
OUTLINE = [
    (100, 36), (118, 40), (128, 50), (133, 62),      # crown to hairline
    (135, 74), (137, 81),                            # forehead, brow
    (141, 87), (147, 96), (150, 102),                # bridge, nose slope, tip
    (146, 107), (140, 109), (137, 113),              # under nose, philtrum, lip
    (139, 118), (136, 123), (135, 128),              # mouth, lower lip, chin
    (130, 133), (121, 136), (112, 138),              # chin, jaw
    (107, 145), (106, 160),                          # neck front to base
    (92, 162), (89, 148), (86, 136),                 # neck back, nape
    (81, 152), (76, 168), (70, 182),                 # hair falling behind
    (58, 187), (46, 179),                            # hair bottom
    (43, 158), (45, 132), (50, 106), (56, 80),       # hair outer edge
    (65, 56), (81, 42),
]

# Rough hair mass, used to steer stroke direction and density.
HAIR_MASS = [
    (100, 36), (118, 40), (128, 50), (133, 62), (120, 72), (106, 84),
    (98, 100), (94, 120), (90, 134), (86, 136), (81, 152), (76, 168),
    (70, 182), (58, 187), (46, 179), (43, 158), (45, 132), (50, 106),
    (56, 80), (65, 56), (81, 42),
]

LENS = (124.0, 86.0, 13.0, 9.5)          # cx, cy, rx, ry
TEMPLE = [(111, 84), (98, 80), (86, 74), (76, 68)]
BRIDGE = [(135, 83), (142, 87)]
MOUTH = [(139, 118), (133, 117.5), (129, 117)]
NOSTRIL = (145, 106)
BROW = [(133, 78), (127, 76.5), (121, 77.5)]

# Strands falling with the hair. Without these the mass reads as a swim cap.
HAIR_STRANDS = [
    [(112, 44), (98, 58), (82, 74), (70, 92), (61, 114), (56, 138), (52, 160), (50, 180)],
    [(95, 40), (84, 62), (74, 86), (66, 112), (60, 140), (56, 166), (53, 184)],
    [(78, 47), (69, 70), (61, 96), (55, 124), (51, 152), (49, 176)],
    [(118, 50), (126, 58), (131, 68)],
]


def catmull(pts, closed=False, samples=16):
    """Sample a Catmull-Rom spline through pts as a polyline."""
    p = list(pts)
    ext = ([p[-1]] + p + [p[0], p[1]]) if closed else ([p[0]] + p + [p[-1]])
    out = []
    for i in range(len(ext) - 3):
        p0, p1, p2, p3 = ext[i:i + 4]
        for s in range(samples):
            t = s / samples
            t2, t3 = t * t, t * t * t
            x = 0.5 * (2 * p1[0] + (-p0[0] + p2[0]) * t
                       + (2 * p0[0] - 5 * p1[0] + 4 * p2[0] - p3[0]) * t2
                       + (-p0[0] + 3 * p1[0] - 3 * p2[0] + p3[0]) * t3)
            y = 0.5 * (2 * p1[1] + (-p0[1] + p2[1]) * t
                       + (2 * p0[1] - 5 * p1[1] + 4 * p2[1] - p3[1]) * t2
                       + (-p0[1] + 3 * p1[1] - 3 * p2[1] + p3[1]) * t3)
            out.append((x, y))
    if not closed:
        out.append((p[-1][0], p[-1][1]))
    return out


def d_of(pts, closed=False):
    return "M" + " L".join(f"{x:.2f},{y:.2f}" for x, y in pts) + (" Z" if closed else "")


def ellipse(cx, cy, rx, ry, n=48):
    return [(cx + rx * math.cos(2 * math.pi * i / n),
             cy + ry * math.sin(2 * math.pi * i / n)) for i in range(n)]


def inside_poly(x, y, poly):
    hit = False
    j = len(poly) - 1
    for i in range(len(poly)):
        xi, yi = poly[i]
        xj, yj = poly[j]
        if (yi > y) != (yj > y) and x < (xj - xi) * (y - yi) / (yj - yi + 1e-12) + xi:
            hit = not hit
        j = i
    return hit


OUTLINE_POLY = catmull(OUTLINE, True, samples=16)
HAIR_POLY = catmull(HAIR_MASS, True)


def outline_d():
    return d_of(OUTLINE_POLY, True)


def hair_d():
    return d_of(HAIR_POLY, True)


def inside(x, y):
    return inside_poly(x, y, OUTLINE_POLY)


def in_hair(x, y):
    return inside_poly(x, y, HAIR_POLY)


_EDGE = OUTLINE_POLY[::2]


def edge_distance(x, y):
    """Distance from a point to the silhouette edge, cheapest form."""
    best = 1e9
    for ex, ey in _EDGE:
        dx, dy = x - ex, y - ey
        d = dx * dx + dy * dy
        if d < best:
            best = d
    return math.sqrt(best)
