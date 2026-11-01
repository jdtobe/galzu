"""White-backed viewing copies of every study, plus PNG exports.

The studies keep a transparent ground because that is what a mark needs. But a
transparent SVG is invisible in an editor that paints transparency black, so
this writes a viewing copy of each one with a white backing rect, and renders
each copy to PNG for viewers with no SVG support at all.

Nothing here is a source of truth: regenerate after changing any study.

    python3 preview.py            # white-backed SVGs and PNGs
    python3 preview.py --svg      # white-backed SVGs only, no Chrome needed
"""
from pathlib import Path
import re
import shutil
import subprocess
import sys

BASE = Path(__file__).parent
PREVIEW = BASE / "preview"

CHROME_CANDIDATES = (
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "/Applications/Chromium.app/Contents/MacOS/Chromium",
    "/usr/bin/google-chrome",
    "/usr/bin/chromium",
)


def studies():
    """Every study SVG, including the archived face direction, by source path.

    preview/ is skipped so the copies never feed back into themselves. Names are
    flat because they are unique across the whole directory.
    """
    out = {}
    for path in sorted(BASE.rglob("*.svg")):
        rel = path.relative_to(BASE)
        if rel.parts[0] == PREVIEW.name:
            continue
        if path.name in out:
            raise SystemExit(f"duplicate study name: {path.name}")
        out[path.name] = path
    return out


def viewbox(text):
    m = re.search(r'viewBox="([\d.\- ]+)"', text)
    if not m:
        return None
    _, _, w, h = (float(v) for v in m.group(1).split())
    return w, h


def whiten(text):
    """Insert a white rect as the first child of the root <svg> element."""
    box = viewbox(text)
    if box is None:
        return text, None
    w, h = box
    i = text.index("<svg")
    j = text.index(">", i) + 1
    rect = f'<rect width="{w:g}" height="{h:g}" fill="#ffffff"/>'
    return text[:j] + rect + text[j:], (w, h)


def chrome():
    for candidate in CHROME_CANDIDATES:
        if Path(candidate).exists():
            return candidate
    return shutil.which("google-chrome") or shutil.which("chromium")


def render(browser, svg, png, size):
    w, h = size
    subprocess.run(
        [browser, "--headless=new", "--no-sandbox", "--disable-gpu",
         "--force-device-scale-factor=2", f"--screenshot={png}",
         f"--window-size={int(w)},{int(h)}", f"file://{svg}"],
        check=True, capture_output=True)


def main(argv):
    PREVIEW.mkdir(exist_ok=True)
    found = studies()
    for name, src in found.items():
        text, _ = whiten(src.read_text())
        (PREVIEW / name).write_text(text)
    print(f"wrote {len(found)} white-backed copies to preview/")

    if "--svg" in argv:
        return 0
    browser = chrome()
    if browser is None:
        print("no Chrome found; skipped PNG export", file=sys.stderr)
        return 0
    sizes = {}
    for name in found:
        _, box = whiten((PREVIEW / name).read_text())
        if box:
            sizes[name] = box
    failed = []
    for name, size in sizes.items():
        png = PREVIEW / f"{name[:-4]}.png"
        try:
            render(browser, (PREVIEW / name).resolve(), png.resolve(), size)
        except subprocess.CalledProcessError:
            failed.append(name)
    print(f"rendered {len(sizes) - len(failed)} PNGs beside them")
    for name in failed:
        print(f"  failed: {name}", file=sys.stderr)
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
