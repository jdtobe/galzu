# Archive: the face direction

Superseded. These are the thirty-five profile studies produced before we
decided the portrait was carrying the concept rather than being it. Each one
illustrates a drawing medium or a pigment process using the same face from
`geometry.py`, so what varies between them is technique, not drawing.

Kept because the work is worth its history and the string-art studies in set 5
are what led to the network icons one level up. Nothing here is current.

| Script | Set | Studies |
|---|---|---|
| `generate.py` | Drawing media | pixel, charcoal, engraving, brush, nouveau |
| `generate2.py` | Pigment processes | wire, knife, scratch, stencil, glitch |
| `generate3.py` | Generated processes | loom, circuit, spectrum, topo, stars |
| `generate4.py` | Recording and print | halftone, knit, string, barcode, typewriter, blueprint, fingerprint, marble, papercut, origami |
| `generate5.py` | String art | one-thread, hub, delone, web, slack, pins, directed, traffic, radial, rounds |
| `gallery.py` | — | rebuilds `gallery.svg` from the SVGs beside it |

`geometry.py` is the shared profile: `generate.py` predates it and draws its own
hand-made face, which is why set 1 looks slightly off in the gallery.

Regenerate a set in place with `python3 generate2.py`, from this directory (the
scripts import `geometry.py` as a sibling). Then rerun `../preview.py` so the
white-backed copies in `../preview/` follow.

This whole directory can be deleted without touching anything current.
