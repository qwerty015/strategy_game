"""Carpentry workshop (Столярная мастерская): plank house with an open shed: workbench, sawhorse, plank stacks."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=61)
W = dict(kind="window", w=0.3, z0=0.55, z1=0.9)
lp.house(b, 0.75, 0.75, top=1.3, wall="planks", roof="roof_brown", pitch_deg=42, ridge="y", origin=(-0.35, 0.1),
         openings={"front": [dict(kind="door", u=0.0, w=0.42, z0=0.25, z1=1.05)], "left": [dict(W, u=0.0)],
                   "back": [dict(W, u=0.0)], "right": [dict(W, u=0.3)]})
# open shed on the right
lp.lean_to(b, 0.42, 1.35, -0.75, 0.85, 0.95, 1.25, key="roof_brown")
b.box("wood_dark", (1.3, 0.8, 0.62), (0.08, 0.08, 1.25))
# workbench
b.box("planks", (0.88, -0.1, 0.42), (0.75, 0.32, 0.06))
for x in (0.58, 1.18):
    for y in (-0.22, 0.02):
        b.box("wood_dark", (x, y, 0.2), (0.05, 0.05, 0.4))
b.box("iron", (0.75, -0.12, 0.47), (0.25, 0.04, 0.03))      # saw blade
# plank stacks
for k in range(5):
    b.box("wood_light", (0.9, 0.5, 0.04 + k * 0.05), (0.85, 0.24, 0.045))
for k in range(3):
    b.box("wood_light", (0.9, -0.62, 0.04 + k * 0.05), (0.8, 0.2, 0.045))
# sawhorse with a log
for x in (-0.95, -0.55):
    b.box("wood_dark", (x, -1.05, 0.2), (0.05, 0.3, 0.05))
    b.box("wood_dark", (x, -1.05, 0.12), (0.05, 0.05, 0.25))
b.box("wood_dark", (-0.75, -1.05, 0.3), (0.55, 0.06, 0.06))
lp.finish(b, "carpentry", OUT)
