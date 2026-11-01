# Art directions

Monochrome studies for the app mark. None of this is a shipped asset; branding
is provisional and the styling should stay easy to re-theme.

Two live sets, one archived set:

| Files | What it is |
|---|---|
| `generate6.py`, `icons.svg`, `icons-compare.svg`, `icons-small.svg`, and the ten icon SVGs | Network icons: mesh, nesting and circular structure with no profile in them. |
| `generate7.py`, `ternary.svg`, and the nine mark SVGs | Ternary hierarchy: "three groups of three groups of three", arranged three by three. |
| `generate8.py`, `germ.svg`, `germ-small.svg`, and the ten germ SVGs | The name: a seed that branches, in the tree and gasket idioms. |
| `archive/` | The rejected face direction — five sets of profile studies, their generators, `geometry.py` and the contact sheets. Kept for provenance, not for use. |

## Looking at them

Every study is drawn with a **transparent ground**, which is what a mark needs
but also what makes it invisible in an editor that paints transparency black.

So `preview/` holds a white-backed copy of every study under the same filename,
plus a PNG of each one. Open anything in there:

```
preview/ternary-tree-27.svg
preview/ternary-tree-27.png
preview/ternary.svg            the whole 3x3 sheet
preview/icons-compare.svg      the ten network icons, white, 240 px a cell
preview/icons-small.svg        those icons at 48, 24 and 16 px
```

The files one level up are the production ones and stay transparent.

## Rebuilding

```sh
./rebuild.sh                 # generators, then previews and PNGs
python3 preview.py --svg     # previews only, no Chrome needed
python3 generate7.py         # just the ternary marks
```

`preview/` is derived — never edit it by hand, and its PNGs are ignored by git.

## Where the direction stands

The face studies were rejected: the portrait carried the concept, and the
concept was a network. Set `generate6.py` draws that network directly, and
`generate7.py` gives it hierarchy. Nothing is decided yet; the shortlist is
Tree · 27, Sierpinski, Cluster mesh, Small world and Nested hex.
