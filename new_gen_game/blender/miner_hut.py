"""Miner hut (Хижина рудокопа): mine entrance in a rocky mound, timber portal, rails and an ore cart, small shed."""
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mathutils import Matrix  # noqa: E402

import lib  # noqa: E402
import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=81)
M["dark"] = lib.mat_flat("dark", (0.01, 0.008, 0.006), rough=1.0)
M["ore"] = lib.mat_noise("ore", [(0.3, (0.25, 0.12, 0.06)), (0.7, (0.45, 0.24, 0.12))], scale=12, bump=0.0)
# mound: a lumpy rock hill behind the entrance
for x, y, sz, sq in ((0.0, 0.6, 0.95, 0.85), (-0.7, 0.5, 0.7, 0.8), (0.7, 0.45, 0.75, 0.8), (0.35, 1.0, 0.8, 0.9),
                     (-0.4, 1.0, 0.75, 0.9), (-1.1, 0.2, 0.45, 0.6), (1.1, 0.15, 0.5, 0.6), (0.0, 1.25, 0.6, 0.7)):
    lp.rock(b, x, y, sz, squash=sq)
# portal
b.box("dark", (0.0, -0.05, 0.42), (0.6, 0.3, 0.84))
for x in (-0.33, 0.33):
    b.box("wood_dark", (x, -0.2, 0.45), (0.1, 0.1, 0.9))
b.box("wood_dark", (0.0, -0.2, 0.92), (0.85, 0.12, 0.12))
b.box("planks", (0.0, -0.22, 1.05), (0.5, 0.05, 0.16))
# rails
for x in (-0.15, 0.15):
    b.box("iron", (x, -0.75, 0.04), (0.03, 1.2, 0.03))
for k in range(6):
    b.box("wood_dark", (0, -0.25 - k * 0.2, 0.015), (0.45, 0.08, 0.03))
# ore cart
b.box("iron", (0, -0.9, 0.25), (0.4, 0.5, 0.24))
b.box("ore", (0, -0.9, 0.39), (0.34, 0.44, 0.06))
for x in (-0.22, 0.22):
    for y in (-1.05, -0.75):
        b.cylinder("iron", (x, y, 0.1), 0.08, 0.08, 0.04, segments=8, rot=Matrix.Rotation(math.pi / 2, 3, "Y"))
# shed + lantern
lp.house(b, 0.45, 0.4, top=0.95, wall="planks", roof="roof_brown", pitch_deg=35, ridge="y", timber=False, origin=(-1.05, -0.75),
         openings={"front": [dict(kind="door", u=0.0, w=0.32, z0=0.2, z1=0.8)]})
b.box("glow", (0.38, -0.3, 0.75), (0.08, 0.08, 0.1))
lp.finish(b, "miner_hut", OUT)
