"""Lumberjack hut (Хижина лесоруба): log cabin, log pile, chopping block with an axe, firewood."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mathutils import Matrix  # noqa: E402

import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=51)
W = dict(kind="window", w=0.26, z0=0.5, z1=0.8)
lp.house(b, 0.85, 0.7, top=1.15, wall="logs", roof="roof_brown", pitch_deg=40, ridge="y", timber=False, chimney=(0.0, 0.45),
         openings={"front": [dict(kind="door", u=0.0, w=0.4, z0=0.25, z1=1.0)], "right": [dict(W, u=0.0)],
                   "left": [dict(W, u=0.0)], "back": [dict(W, u=0.0)]})
# log ends crossing at the corners
for x in (-0.85, 0.85):
    for y in (-0.7, 0.7):
        for k in range(4):
            b.cylinder("logs", (x, y, 0.35 + k * 0.22), 0.08, 0.08, 0.2, segments=6)
lp.log_pile(b, 1.3, -0.1, length=1.1, rows=3, yaw=1.5708)
# chopping block + axe
b.cylinder("wood_light", (-0.75, -1.05, 0.15), 0.18, 0.18, 0.3, segments=8)
b.box("wood_dark", (-0.75, -1.05, 0.48), (0.04, 0.04, 0.5), Matrix.Rotation(0.5, 3, "Y"))
b.box("iron", (-0.86, -1.05, 0.3), (0.16, 0.03, 0.12), Matrix.Rotation(0.5, 3, "Y"))
for i in range(5):  # split firewood
    b.box("wood_light", (-1.2 + (i % 3) * 0.12, -0.65 + (i // 3) * 0.13, 0.06 + (i // 3) * 0.1), (0.1, 0.32, 0.1))
lp.finish(b, "lumberjack_hut", OUT)
