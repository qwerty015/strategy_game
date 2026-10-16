"""Farm (Ферма) farmhouse: thatched cottage, haystack, hay bales. The 3x3 field is drawn by the game around it."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mathutils import Matrix  # noqa: E402

import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=41)
W = dict(kind="window", w=0.28, z0=0.5, z1=0.85)
lp.house(b, 0.95, 0.7, top=1.2, wall="plaster_white", roof="thatch", pitch_deg=48, ridge="x", overhang=0.3, chimney=(-0.5, 0.2),
         openings={"front": [dict(kind="door", u=-0.2, w=0.38, z0=0.25, z1=1.0), dict(W, u=0.45)],
                   "right": [dict(W, u=0.0)], "back": [dict(W, u=0.0)], "left": [dict(W, u=0.0)]})
# haystack and bales
b.cylinder("straw", (1.35, 0.35, 0.3), 0.42, 0.38, 0.6, segments=8)
b.cylinder("straw", (1.35, 0.35, 0.85), 0.38, 0.05, 0.5, segments=8)
for x, y, z, yaw in ((1.2, -0.55, 0.13, 0.2), (1.55, -0.3, 0.13, 1.4), (1.35, -0.45, 0.38, 0.8)):
    b.box("straw", (x, y, z), (0.42, 0.26, 0.26), Matrix.Rotation(yaw, 3, "Z"))
lp.fence(b, (-1.3, -1.05), (0.5, -1.05))
lp.sack(b, -0.85, -0.9)
lp.barrel(b, -1.2, 0.2)
lp.finish(b, "farm", OUT)
