"""Winery (Винодельня): cellar house, barrel rack, wine press, grape baskets. The vineyard is drawn by the game."""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lib  # noqa: E402
import lowpoly as lp  # noqa: E402

b, M, OUT = lp.begin(seed=121)
M["grape"] = lib.mat_noise("grape", [(0.3, (0.12, 0.03, 0.14)), (0.7, (0.30, 0.08, 0.32))], scale=30, bump=0.0)
W = dict(kind="window", w=0.28, z0=0.6, z1=0.95)
lp.house(b, 0.95, 0.7, top=1.4, wall="plaster", roof="roof_red", pitch_deg=36, ridge="x", plinth=0.5,
         openings={"front": [dict(kind="door", u=-0.45, w=0.45, z0=0.5, z1=1.2), dict(W, u=0.45)],
                   "right": [dict(W, u=0.0)], "back": [dict(W, u=0.0)], "left": [dict(W, u=0.0)]})
# barrel rack on the right
b.box("wood_dark", (1.3, 0.0, 0.08), (0.45, 1.2, 0.08))
for y in (-0.38, 0.0, 0.38):
    lp.barrel(b, 1.3, y, z=0.08, h=0.38, r=0.17, lying=True, yaw=0.0)
# wine press
b.cylinder("wood_light", (-0.55, -1.05, 0.2), 0.22, 0.22, 0.4, segments=8)
b.cylinder("grape", (-0.55, -1.05, 0.41), 0.19, 0.19, 0.03, segments=8)
for x in (-0.8, -0.3):
    b.box("wood_dark", (x, -1.05, 0.45), (0.06, 0.06, 0.9))
b.box("wood_dark", (-0.55, -1.05, 0.88), (0.6, 0.08, 0.08))
b.cylinder("iron", (-0.55, -1.05, 0.65), 0.03, 0.03, 0.45, segments=6)
# grape baskets
for x in (0.15, 0.45):
    b.cylinder("wood_light", (x, -1.05, 0.12), 0.14, 0.11, 0.24, segments=8)
    b.sphere("grape", (x, -1.05, 0.24), 0.12, scale=(1, 1, 0.5), segments=6)
lp.finish(b, "winery", OUT)
