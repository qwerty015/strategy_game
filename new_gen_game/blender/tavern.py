"""Tavern (Харчевня): two-storey inn, red tile roof, lit windows, sign, benches and barrels out front.

    blender -b --factory-startup --python tavern.py -- <out.png> [resolution] [samples]
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import lowpoly as lp  # noqa: E402
from mathutils import Matrix  # noqa: E402

b, M, OUT = lp.begin(seed=31)

HX, HY = 1.05, 0.85
W = dict(kind="window", w=0.32, z0=0.55, z1=0.95)
W2 = dict(kind="window", w=0.3, z0=1.45, z1=1.8)
lp.house(b, HX, HY, top=2.15, ridge="x", roof="roof_red", pitch_deg=38, chimney=(0.6, 0.35), openings={
    "front": [dict(kind="door", u=0.0, w=0.42, z0=0.25, z1=1.05), dict(W, u=-0.62, glow=True), dict(W, u=0.62, glow=True),
              dict(W2, u=-0.55), dict(W2, u=0.55, glow=True)],
    "right": [dict(W, u=0.0, glow=True), dict(W2, u=0.0)],
    "back": [dict(W, u=-0.4), dict(W2, u=0.4, glow=True)],
    "left": [dict(W, u=0.2), dict(W2, u=-0.2)],
})
# floor divider beam all round (the second storey)
for y, sx, sy in ((-HY - 0.012, 2 * HX, 0.05), (HY + 0.012, 2 * HX, 0.05)):
    b.box("wood_dark", (0, y, 1.25), (sx, sy, 0.08))
for x in (-HX - 0.012, HX + 0.012):
    b.box("wood_dark", (x, 0, 1.25), (0.05, 2 * HY, 0.08))

# hanging sign on a bracket by the door
b.box("wood_dark", (0.45, -HY - 0.25, 1.15), (0.05, 0.5, 0.05))
b.box("planks", (0.45, -HY - 0.38, 0.92), (0.04, 0.32, 0.24))
b.cylinder("glow", (0.45, -HY - 0.38, 0.92), 0.05, 0.05, 0.05, segments=6, rot=Matrix.Rotation(1.5708, 3, "Y"))

# yard: table with benches, barrels, a lantern post
b.box("planks", (-0.55, -HY - 0.6, 0.38), (0.6, 0.3, 0.05))
for x in (-0.78, -0.32):
    b.box("wood_dark", (x, -HY - 0.6, 0.18), (0.05, 0.24, 0.36))
for dy in (-0.25, 0.25):
    b.box("planks", (-0.55, -HY - 0.6 + dy, 0.22), (0.6, 0.12, 0.04))
lp.barrel(b, HX + 0.25, -0.35)
lp.barrel(b, HX + 0.25, 0.05)
lp.barrel(b, HX + 0.22, -0.15, z=0.42, lying=True, yaw=1.57)
lp.crate(b, -HX - 0.25, 0.3, size=0.3)
b.box("wood_dark", (0.85, -HY - 0.55, 0.5), (0.06, 0.06, 1.0))
b.box("glow", (0.85, -HY - 0.55, 1.05), (0.12, 0.12, 0.14))

lp.finish(b, "tavern", OUT)
