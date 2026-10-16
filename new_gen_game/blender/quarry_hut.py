"""Quarry (Каменоломня): stone hut, cut blocks stacked, a small wooden crane lifting a block."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mathutils import Matrix  # noqa: E402

import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=71)
W = dict(kind="window", w=0.26, z0=0.5, z1=0.8)
lp.house(b, 0.7, 0.65, top=1.1, wall="stone_grey", roof="roof_slate", pitch_deg=38, ridge="x", timber=False, origin=(-0.4, 0.3),
         openings={"front": [dict(kind="door", u=0.0, w=0.4, z0=0.2, z1=0.95)], "right": [dict(W, u=0.0)],
                   "back": [dict(W, u=0.0)], "left": [dict(W, u=0.0)]})
# stacked blocks
for x, y, z in ((0.75, -0.55, 0), (1.15, -0.55, 0), (0.95, -0.55, 0.3), (0.75, -0.15, 0), (1.15, -0.15, 0)):
    t = b.rng.uniform(0.9, 1.1)
    b.box("stone_grey", (x, y, z + 0.14), (0.36, 0.34, 0.28), tint=(t, t, t))
# crane: mast, boom, rope, hanging block
b.box("wood_dark", (0.9, 0.55, 0.85), (0.1, 0.1, 1.7))
b.beam("wood_dark", (0.9, 0.55, 1.6), (0.25, -0.35, 1.45), 0.07, 0.07, (0, 0, 1))
b.beam("wood_dark", (0.9, 0.55, 0.9), (0.6, 0.15, 1.5), 0.05, 0.05, (0, 0, 1))
b.box("rope", (0.27, -0.33, 1.05), (0.02, 0.02, 0.8))
b.box("stone_grey", (0.27, -0.33, 0.55), (0.3, 0.3, 0.24))
for y in (0.4, 0.7):
    b.box("wood_dark", (0.9, y, 0.06), (0.5, 0.08, 0.08))
# pick resting on a rock
lp.rock(b, -1.2, -0.75, 0.3)
b.box("wood_dark", (-1.0, -0.8, 0.25), (0.04, 0.04, 0.5), Matrix.Rotation(0.8, 3, "X"))
lp.finish(b, "quarry_hut", OUT)
