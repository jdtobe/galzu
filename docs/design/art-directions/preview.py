"""White-backed copies of every study, for viewing.

The studies keep a transparent ground because that is what a mark needs. But a
transparent SVG is invisible in an editor that paints transparency black, so
this writes a viewing copy of each one with a white backing rect.

Nothing here is a source of truth: regenerate after changing any study.

    python3 preview.py
"""
from pathlib import Path
import re

OUT = Path(__file__).parent
PREVIEW = OUT / "preview"


def viewbox(text):
    m = re.search(r'viewBox="([\d.\- ]+)"', text)
    if not m:
        return None
    x, y, w, h = (float(v) for v in m.group(1).split())
    return w, h


def whiten(text):
    """Insert a white rect as the first child of the root <svg> element."""
    box = viewbox(text)
    if box is None:
        return text
    w, h = box
    i = text.index("<svg")
    j = text.index(">", i) + 1
    rect = f'<rect width="{w:g}" height="{h:g}" fill="#ffffff"/>'
    return text[:j] + rect + text[j:]


def main():
    PREVIEW.mkdir(exist_ok=True)
    names = sorted(p.name for p in OUT.glob("*.svg"))
    for name in names:
        src = OUT / name
        (PREVIEW / name).write_text(whiten(src.read_text()))
    print(f"wrote {len(names)} white-backed copies to preview/")


if __name__ == "__main__":
    main()
