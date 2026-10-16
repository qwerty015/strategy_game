"""Fisher hut (Хижина рыбака): plank hut, net drying on poles, fish rack, upturned boat."""
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mathutils import Matrix  # noqa: E402

import lib  # noqa: E402
import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=91)
M["fish"] = lib.mat_flat("fish", (0.45, 0.50, 0.52), rough=0.4, metallic=0.3)
M["net"] = lib.mat_flat("net", (0.30, 0.26, 0.18), rough=1.0)
M["boat"] = lib.mat_wood("boat", (0.16, 0.08, 0.035), (0.36, 0.20, 0.09))
W = dict(kind="window", w=0.26, z0=0.5, z1=0.8)
lp.house(b, 0.75, 0.6, top=1.1, wall="planks", roof="roof_slate", pitch_deg=40, ridge="y", origin=(-0.35, 0.35),
         openings={"front": [dict(kind="door", u=0.0, w=0.38, z0=0.25, z1=0.95)], "right": [dict(W, u=0.0)],
                   "left": [dict(W, u=0.0)], "back": [dict(W, u=0.0)]})
# net between two poles
for y in (-0.15, 0.85):
    b.box("wood_dark", (0.9, y, 0.55), (0.06, 0.06, 1.1))
for k in range(6):                      # net: a grid of ropes, sagging a little
    y = -0.15 + k * 0.2
    b.box("net", (0.9, y, 0.62), (0.015, 0.015, 0.8))
for k in range(5):
    b.box("net", (0.9, 0.35, 0.3 + k * 0.17 - (0.03 if k in (1, 2, 3) else 0)), (0.015, 1.0, 0.015))
for i in range(3):
    b.sphere("fish", (0.92, 0.0 + i * 0.3, 0.55 + (i % 2) * 0.2), 0.09, scale=(0.3, 1.0, 0.45), segments=6)
# drying rack with fish
b.box("wood_dark", (-0.4, -0.75, 0.75), (0.9, 0.05, 0.05))
for x in (-0.82, 0.02):
    b.box("wood_dark", (x, -0.75, 0.38), (0.05, 0.05, 0.75))
for i in range(5):
    b.sphere("fish", (-0.7 + i * 0.15, -0.75, 0.58), 0.13, scale=(0.4, 0.15, 1.0), segments=6)
# upturned boat
lp.boat_hull(b, 0.75, -0.85, length=1.15, width=0.45, height=0.2, yaw=math.radians(20), key="boat", upside_down=True)
lp.barrel(b, -1.25, -0.2)
lp.finish(b, "fisher_hut", OUT)
