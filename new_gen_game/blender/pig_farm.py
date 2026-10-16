"""Pig farm (Свиноферма): barn, fenced mud pen with a trough and pigs."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mathutils import Matrix, Vector  # noqa: E402

import lib  # noqa: E402
import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=101)
M["pink"] = lib.mat_flat("pink", (0.85, 0.5, 0.45), rough=0.8)
M["mud"] = lib.mat_noise("mud", [(0.3, (0.12, 0.07, 0.03)), (0.7, (0.25, 0.15, 0.07))], scale=4, rough=0.5, bump=0.0)
lp.house(b, 0.7, 0.85, top=1.15, wall="planks", roof="roof_brown", pitch_deg=40, ridge="y", origin=(-0.75, 0.2),
         openings={"right": [dict(kind="door", u=-0.2, w=0.45, z0=0.25, z1=0.95)],
                   "front": [dict(kind="window", u=0.0, w=0.26, z0=0.55, z1=0.8)]})
# pen
b.box("mud", (0.65, -0.1, 0.01), (1.2, 1.5, 0.02))
lp.fence(b, (0.05, -0.85), (1.3, -0.85))
lp.fence(b, (1.3, -0.85), (1.3, 0.65))
lp.fence(b, (1.3, 0.65), (0.05, 0.65))
b.box("wood_dark", (0.3, 0.45, 0.1), (0.5, 0.18, 0.18))
b.box("mud", (0.3, 0.45, 0.19), (0.42, 0.1, 0.02))


def pig(x, y, yaw):
    r = Matrix.Rotation(yaw, 3, "Z")
    o = Vector((x, y, 0))
    b.sphere("pink", o + Vector((0, 0, 0.2)), 0.2, scale=(1.0, 0.7, 0.65), segments=8, rot=r)
    b.cylinder("pink", o + r @ Vector((0.22, 0, 0.21)), 0.06, 0.06, 0.06, segments=6, rot=r @ Matrix.Rotation(1.5708, 3, "Y"))
    for ex in (-1, 1):
        b.box("pink", o + r @ Vector((0.13, ex * 0.07, 0.31)), (0.05, 0.06, 0.06))
    for lx in (-0.1, 0.1):
        for ly in (-0.07, 0.07):
            b.box("pink", o + r @ Vector((lx, ly, 0.05)), (0.05, 0.05, 0.1))


pig(0.75, -0.35, 0.4)
pig(0.95, 0.25, 2.5)
pig(0.45, -0.1, -1.2)
lp.sack(b, -1.55, -0.55)
lp.finish(b, "pig_farm", OUT)
