"""Contact sheet for every art-direction study, one row per set.

Reads the generated SVGs rather than re-rendering, so the gallery cannot drift
from the studies it claims to show.

    python3 gallery.py
"""
from pathlib import Path
import re

OUT = Path(__file__).parent
INK = "#141414"

SETS = [
    ("Set 1 · drawing media", [
        "pixel.svg", "charcoal.svg", "engraving.svg", "brush.svg", "nouveau.svg"]),
    ("Set 2 · pigment processes", [
        "wire.svg", "knife.svg", "scratch.svg", "stencil.svg", "glitch.svg"]),
    ("Set 3 · generated processes", [
        "loom.svg", "circuit.svg", "spectrum.svg", "topo.svg", "stars.svg"]),
    ("Set 4 · recording and print", [
        "halftone.svg", "knit.svg", "string.svg", "barcode.svg", "typewriter.svg"]),
    ("Set 4 · engineering and fluid", [
        "blueprint.svg", "fingerprint.svg", "marble.svg", "papercut.svg", "origami.svg"]),
]


def inner(path):
    """The drawing inside a study, without its <svg> wrapper."""
    text = path.read_text()
    text = re.sub(r"^<\?xml[^>]*\?>\s*", "", text)
    return re.sub(r"^.*?<svg[^>]*>", "", text, count=1, flags=re.S).replace("</svg>", "")


def main():
    cell, pad, label, rowgap = 200, 18, 24, 28
    cols = max(len(files) for _, files in SETS)
    width = pad * 2 + cols * cell + (cols - 1) * pad
    height = pad * 2 + len(SETS) * (cell + label + rowgap) - rowgap

    parts = []
    y = pad
    for title, files in SETS:
        parts.append(
            f'<text x="{pad}" y="{y - 6}" font-family="Helvetica,Arial,sans-serif" '
            f'font-size="11" letter-spacing="1.2" fill="{INK}" opacity="0.55">'
            f'{title.upper()}</text>')
        for i, name in enumerate(files):
            path = OUT / name
            x = pad + i * (cell + pad)
            body = inner(path)
            parts.append(
                f'<g transform="translate({x},{y})">'
                f'<rect width="{cell}" height="{cell}" fill="#fff" stroke="#e3e3e3"/>'
                f'{body}</g>')
            caption = name.replace(".svg", "")
            parts.append(
                f'<text x="{x + cell/2:.0f}" y="{y + cell + 16}" text-anchor="middle" '
                f'font-family="Helvetica,Arial,sans-serif" font-size="11.5" '
                f'letter-spacing="0.3" fill="{INK}">{caption}</text>')
        y += cell + label + rowgap

    svg = (f'<?xml version="1.0" encoding="UTF-8"?>\n'
           f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" '
           f'width="{width}" height="{height}" color="{INK}">'
           f'<rect width="{width}" height="{height}" fill="#fff"/>'
           f'{"".join(parts)}</svg>\n')
    (OUT / "gallery.svg").write_text(svg)
    print("wrote gallery.svg", width, "x", height)


if __name__ == "__main__":
    main()
