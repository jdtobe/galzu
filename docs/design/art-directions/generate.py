"""Reproducible, original monochrome art-direction studies, not app assets."""
from pathlib import Path
import math
import random

OUT = Path(__file__).parent
INK = "#286450"


def path(d, **attrs):
    fields = " ".join(f'{k.replace("_", "-")}="{v}"' for k, v in attrs.items())
    return f'<path d="{d}" {fields}/>'


def stroke(d, width=1.5, **attrs):
    return path(d, fill="none", stroke="currentColor", stroke_width=width,
                stroke_linecap="round", stroke_linejoin="round", **attrs)


def pixel():
    """Hand-placed 40x40 sprite. Snapping a smooth profile to a grid reads as mush;
    pixel art has to be drawn on the grid, one square at a time."""
    grid = [['.']*40 for _ in range(40)]

    def dark(y, a, b=0):
        for x in range(a, max(a, b)+1):
            grid[y][x] = '#'

    # Crown of the hair.
    for y, a, b in [(3,14,25),(4,11,28),(5,9,30),(6,8,31),(7,7,32),(8,6,33),(9,5,34),(10,5,34)]:
        dark(y,a,b)
    # y, left edge of hair, first light face column, right edge of profile.
    for y, left, face, profile in [
        (11,4,24,31),(12,4,23,32),(13,4,23,33),(14,4,23,34),(15,4,23,32),
        (16,4,23,31),(17,4,23,31),(18,4,23,32),(19,3,23,34),(20,3,23,35),
        (21,3,23,32),(22,2,23,33),(23,2,23,31),(24,1,23,32),(25,1,23,30),
        (26,1,22,31),(27,2,22,29),(28,3,21,27),(29,4,21,26),(30,6,20,25),
        (31,7,19,22),(32,6,19,22)]:
        dark(y,left,face-1)
        for x in range(face,profile+1):
            grid[y][x] = '.'
    # Taper the last of the hair instead of cutting it off square.
    for y, a, b in [(33,6,15),(34,9,13)]:
        dark(y,a,b)
    # Glasses: rims, lens, temple arm reaching back into the hair, and a pupil.
    for x in range(25,32):
        dark(14,x); dark(18,x)
    for y in range(14,19):
        dark(y,25); dark(y,31)
    for y in (15,16):
        dark(y,22,24)
    dark(16,28)
    # Mouth.
    dark(23,29,30)
    # Two locks of hair, carved as gaps rather than drawn as lines.
    for y in range(16,31):
        grid[y][10+round(1.6*math.sin(y/2.6))] = '.'
        if 21 <= y <= 27:
            grid[y][14] = '.'
    cells = []
    for y in range(40):
        for x in range(40):
            if grid[y][x] == '#':
                cells.append(f'<rect x="{x*4}" y="{y*4}" width="4" height="4"/>')
    return '<g fill="currentColor" shape-rendering="crispEdges">'+''.join(cells)+'</g>'


def charcoal():
    rng = random.Random(18)
    scratches = []
    for _ in range(105):
        x, y = rng.uniform(18,90), rng.uniform(12,146)
        scratches.append(path(f'M{x:.1f} {y:.1f} l{rng.uniform(-5,5):.1f} {rng.uniform(1,10):.1f}',
                              stroke="black", stroke_width=rng.choice([.4,.6,.9]), fill="none"))
    mask = '<defs><mask id="charcoal-grain" maskUnits="userSpaceOnUse" x="0" y="0" width="160" height="160"><rect width="160" height="160" fill="white"/>'+''.join(scratches)+'</mask></defs>'
    hair = path('M82 9 L62 12 L45 25 L37 43 L34 69 L27 94 L19 123 L7 151 L34 149 L59 139 L78 119 L70 91 L75 64 L84 46 L100 50 L114 55 L118 44 L109 25 L96 13 Z', fill="currentColor", mask="url(#charcoal-grain)")
    edges = []
    for i in range(24):
        y = 30+i*4.5
        x = 38-(y-30)*.20+rng.uniform(-2,2)
        edges.append(stroke(f'M{x:.1f} {y:.1f} l-4 9',rng.uniform(.6,1.5)))
    face = stroke('M114 55 L111 66 L116 74 L127 81 L116 85 L119 92 L116 97 L120 102 L115 112 L108 119 L92 119 L79 110',3.2)
    glasses = stroke('M86 61 L111 62 L114 76 L107 83 L87 79 L82 69 Z M73 58 L84 65',4.5)
    ear = stroke('M76 74 C64 62 61 84 72 89 M70 75 L69 83',2.3)
    marks = ''.join(stroke(f'M{96+i*3} {102+i%3} l-3 4',.65) for i in range(4))
    return mask+hair+''.join(edges)+face+glasses+ear+marks


def engraving():
    rng = random.Random(12)
    parts = [stroke('M111 50 C110 59 109 64 113 69 L126 78 Q128 81 115 84 L118 91 L115 95 L119 99 Q118 108 108 117 L94 118 L86 127 L105 148 M94 118 L82 108',1.25),
             stroke('M111 50 C117 28 103 10 82 10 C55 7 36 23 35 52 C35 79 29 114 12 145 C36 154 70 146 86 127',1.4)]
    for i in range(31):
        start_x = 45+i*1.7
        start_y = 21+8*math.sin(i/7)
        end_x = 15+i*2.1
        parts.append(stroke(f'M{start_x:.1f} {start_y:.1f} C{start_x-16:.1f} 57 {end_x+25:.1f} 102 {end_x:.1f} {145-i*.5:.1f}',.45+(i%4)*.12))
    for i in range(18):
        y = 38+i*5.3
        x = 37-(y-38)*.12
        parts.append(stroke(f'M{x:.1f} {y:.1f} l{18+i%5} -8',.45))
    parts += [stroke('M80 48 C86 43 95 46 105 51 M81 52 Q95 49 105 56',.9),
              stroke('M82 65 C72 57 68 68 74 81 Q77 85 80 79 M76 67 Q72 72 78 76',1),
              '<ellipse cx="101" cy="67" rx="13" ry="10" stroke="currentColor" stroke-width="1.6"/>',
              stroke('M75 60 L88 64 M115 66 L118 67',1.4),
              stroke('M99 65 Q104 63 107 67 M117 94 L111 94',.8),
              '<circle cx="104" cy="66" r="1.3" fill="currentColor"/>',
              stroke('M85 127 L68 148 L36 153 M85 129 L89 149 L108 153',1)]
    for i in range(17):
        x, y = 87+rng.random()*21, 89+rng.random()*22
        parts.append(stroke(f'M{x:.1f} {y:.1f} l-3 4',.4))
    return ''.join(parts)


def brush():
    hair = path('M111 47 C113 15 82 1 56 17 C31 32 43 63 29 95 C22 113 15 137 4 148 C36 149 57 136 70 111 C86 82 70 57 82 34 C65 62 77 86 55 112 C47 123 36 131 24 134 C54 91 37 59 52 33 C68 8 97 16 111 47 Z', fill="currentColor")
    profile = path('M111 47 C109 60 113 68 126 80 C133 86 122 89 115 89 L119 97 L115 101 L119 106 C116 118 104 124 91 120 L80 112 L82 108 L94 116 L105 116 L114 106 L110 101 L114 97 L110 87 L124 83 C113 72 105 61 111 47 Z',fill="currentColor")
    strands = [stroke('M92 17 C53 9 30 57 31 85',1.2),
               stroke('M45 79 C39 107 27 126 16 135',2.1),
               stroke('M68 125 Q49 148 19 153',.9),
               stroke('M81 30 Q86 48 106 50',3)]
    glasses = path('M88 57 C78 56 76 65 79 73 C81 83 94 86 105 81 C116 76 115 60 106 59 Z M88 62 C100 61 109 61 109 69 C109 78 94 82 85 76 C79 72 81 62 88 62 Z',fill="currentColor",fill_rule="evenodd")
    for x,y,r in [(21,104,1),(12,139,1.4),(33,20,.8),(24,126,.6),(17,146,.5),(45,18,.7)]:
        strands.append(f'<circle cx="{x}" cy="{y}" r="{r}" fill="currentColor"/>')
    return hair+profile+''.join(strands)+glasses+stroke('M70 56 L80 62 M71 71 C61 63 62 83 72 85',2.4)


def nouveau():
    parts = [stroke('M115 49 C115 23 101 10 80 10 C41 10 24 35 22 70 C21 108 15 132 7 150 M7 150 C35 160 59 146 77 126 M115 49 Q111 64 119 74 L129 82 L117 87 L120 95 L117 99 L120 104 Q116 116 106 120 H94 L87 130',1.6),
             stroke('M106 31 C80 7 46 32 43 58 C38 94 65 112 35 147 M92 24 C56 23 47 53 53 76 C62 109 63 132 34 151 M82 33 C64 45 69 71 67 91 C64 117 89 128 74 146 M30 68 C29 97 42 119 15 148',1.1),
             stroke('M82 44 C90 52 102 55 114 53 M87 58 Q101 55 110 61 M81 72 C69 62 68 83 81 85 M76 73 Q72 78 79 79',1.1),
             '<ellipse cx="103" cy="71" rx="12" ry="9" stroke="currentColor" stroke-width="1.8"/>',
             stroke('M79 64 L91 68 M116 96 H111 M95 120 L94 136 C105 139 116 145 126 154',1.4),
             stroke('M141 151 V57 C141 7 93 -1 62 6 M139 89 C151 62 149 38 135 22 M141 118 C129 135 124 143 126 154 M85 150 C93 129 120 127 126 154',1),
             stroke('M31 43 C8 24 13 3 37 5 C48 7 47 21 37 24 C29 27 24 16 32 13',1.3)]
    for x,y,angle in [(141,91,-20),(137,109,20),(132,128,40),(124,14,-35),(24,39,-40),(85,147,30)]:
        parts.append(f'<path d="M{x} {y} q-12 -12 -6 -23 q14 10 6 23 Z" fill="currentColor" transform="rotate({angle} {x} {y})"/>')
    parts.append(stroke('M68 153 C73 146 71 135 62 136 C54 137 56 148 62 146',1.2))
    return ''.join(parts)


STUDIES = [
    ('pixel', '1 / 8-bit pixel art', 'A game-sprite character, built from square pixels.', pixel),
    ('charcoal', '2 / Scratchy charcoal', 'Rough edges, scraped highlights, restless marks.', charcoal),
    ('engraving', '3 / Copperplate engraving', 'Fine hair strands and crosshatched portrait detail.', engraving),
    ('brush', '4 / Brush & ink', 'Broad, tapered strokes and an expressive gesture.', brush),
    ('nouveau', '5 / Art Nouveau', 'Ornamental curves, botanical details, flowing hair.', nouveau),
]


def generate():
    bodies = {}
    for key,title,description,make in STUDIES:
        body = make()
        bodies[key] = body
        svg = f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 160" fill="none" color="{INK}"><title>{title.replace("&","&amp;")}</title>{body}</svg>\n'
        (OUT/f'{key}.svg').write_text(svg)
    symbols = ''.join(f'<symbol id="{key}" viewBox="0 0 160 160" fill="none">{body}</symbol>' for key,body in bodies.items())
    sheet = [f'<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="1600" height="1600" viewBox="0 0 1600 1600" color="{INK}"><title>Garth · five different art media</title><defs>{symbols}</defs>',
             '<rect width="1600" height="1600" fill="#f1f3ef"/><g font-family="Arial, sans-serif">',
             '<text x="30" y="48" fill="#66756c" font-size="12" letter-spacing="2">GARTH / ART DIRECTION, NOT ICON VARIATIONS</text>',
             '<text x="30" y="103" fill="#243630" font-size="39" font-weight="600">Change the medium. Then refine the form.</text>',
             '<text x="30" y="142" fill="#66756c" font-size="16">Five deliberately different treatments. All use one ink color and transparent negative space.</text>']
    positions = [(30,185),(550,185),(1070,185),(290,835),(810,835)]
    for (key,title,description,_),(x,y) in zip(STUDIES,positions):
        sheet += [f'<g transform="translate({x} {y})"><rect width="500" height="610" rx="15" fill="#fff" stroke="#dce4db"/>',
                  f'<text x="25" y="45" fill="#243630" font-size="24" font-weight="600">{title.replace("&","&amp;")}</text>',
                  f'<text x="25" y="78" fill="#66756c" font-size="14">{description}</text>',
                  f'<use xlink:href="#{key}" x="55" y="110" width="390" height="390"/>',
                  '<path d="M25 522 H475" stroke="#e3e9e2"/>',
                  '<text x="25" y="560" fill="#66756c" font-size="12">STYLE STUDY · NOT A FINAL APP ICON</text></g>']
    sheet += ['<text x="30" y="1510" fill="#66756c" font-size="15">Pick the visual language first. Face shape, hair, glasses, and small-size readability come next.</text>',
              '<text x="30" y="1545" fill="#66756c" font-size="12">Original vector studies. No changes to the app’s current icon.</text></g></svg>\n']
    (OUT/'comparison.svg').write_text(''.join(sheet))


if __name__ == '__main__':
    generate()
