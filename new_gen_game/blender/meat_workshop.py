"""Meat workshop (Мясной цех): stone-and-plaster house with a red awning, hams hanging, chopping table."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lib  # noqa: E402
import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=111)
M["meat"] = lib.mat_noise("meat", [(0.3, (0.35, 0.10, 0.06)), (0.7, (0.55, 0.22, 0.14))], scale=10, bump=0.0)
W = dict(kind="window", w=0.3, z0=0.6, z1=0.95)
lp.house(b, 0.9, 0.7, top=1.35, wall="plaster_white", roof="roof_red", pitch_deg=40, ridge="x", plinth=0.45, chimney=(0.5, 0.25),
         openings={"front": [dict(kind="door", u=-0.35, w=0.4, z0=0.45, z1=1.15), dict(W, u=0.4)],
                   "right": [dict(W, u=0.0)], "back": [dict(W, u=0.0)], "left": [dict(W, u=0.0)]})
# awning over the front
lp.lean_to(b, -0.9, 0.9, -1.25, -0.7, 1.0, 1.25, key="cloth")
# hanging rail with hams and sausages
b.box("wood_dark", (0.3, -1.15, 0.92), (0.95, 0.04, 0.04))
for i in range(4):
    b.sphere("meat", (-0.05 + i * 0.22, -1.15, 0.72), 0.09, scale=(0.75, 0.75, 1.3), segments=6)
    b.box("rope", (-0.05 + i * 0.22, -1.15, 0.86), (0.015, 0.015, 0.1))
# chopping table
b.box("wood_light", (-0.45, -1.05, 0.4), (0.5, 0.35, 0.12))
for x in (-0.65, -0.25):
    b.box("wood_dark", (x, -1.05, 0.17), (0.07, 0.25, 0.34))
b.sphere("meat", (-0.45, -1.05, 0.5), 0.1, scale=(1.3, 0.8, 0.5), segments=6)
lp.barrel(b, 1.2, -0.2)
lp.crate(b, 1.2, 0.3, size=0.28)
lp.finish(b, "meat_workshop", OUT)
