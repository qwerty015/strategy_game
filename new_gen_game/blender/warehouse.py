"""Warehouse (Склад): timber-framed barn, gable end with open doors facing the viewer.

    blender -b --factory-startup --python warehouse.py -- <out.png> [resolution] [samples] [outline 0/1]

Layout (camera looks from +X/-Y): the -Y gable is the main facade with a real
door recess full of goods and a hoist above; the +X long side carries a
lean-to shed and a dormer on its roof slope; the ridge runs along Y.
"""

import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from mathutils import Matrix, Vector  # noqa: E402

import lib  # noqa: E402
import props  # noqa: E402

args = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []
HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.abspath(args[0] if args else os.path.join(HERE, "..", "renders", "warehouse.png"))
RES = int(args[1]) if len(args) > 1 else 1024
SAMPLES = int(args[2]) if len(args) > 2 else 192
OUTLINE = 0.011 if len(args) <= 3 or args[3] != "0" else 0.0

lib.clear_scene()
lib.setup_render(RES, SAMPLES)
lib.setup_world()
lib.add_sun()

M = {
    "plaster": lib.mat_noise("plaster", [(0.3, (0.56, 0.44, 0.27)), (0.75, (0.76, 0.63, 0.42))], scale=3, bump=0.18, rough=0.95),
    "wood_dark": lib.mat_wood("wood_dark", (0.06, 0.028, 0.012), (0.17, 0.08, 0.032)),
    "wood_light": lib.mat_wood("wood_light", (0.24, 0.12, 0.045), (0.50, 0.29, 0.11)),
    "stone": lib.mat_stone("stone", (0.24, 0.21, 0.17), (0.38, 0.34, 0.28), (0.08, 0.07, 0.055), scale=2.6),
    "slate": lib.mat_noise(
        "slate",
        [(0.0, (0.02, 0.024, 0.04)), (0.5, (0.045, 0.052, 0.08)), (0.66, (0.05, 0.056, 0.07)), (0.74, (0.08, 0.11, 0.02))],
        scale=2.2, rough=0.55, bump=0.1,
    ),
    "iron": lib.mat_flat("iron", (0.04, 0.04, 0.045), rough=0.45, metallic=0.85),
    "dirt": lib.mat_noise("dirt", [(0.3, (0.22, 0.13, 0.05)), (0.72, (0.42, 0.28, 0.12))], scale=3.5, rough=1.0, bump=0.3),
    "pebble": lib.mat_noise("pebble", [(0.3, (0.17, 0.155, 0.13)), (0.7, (0.33, 0.30, 0.25))], scale=12, rough=0.9, bump=0.2),
    "grass": lib.mat_flat("grass", (0.09, 0.24, 0.025), rough=0.8),
    "leaf": lib.mat_noise("leaf", [(0.3, (0.04, 0.12, 0.015)), (0.7, (0.10, 0.26, 0.03))], scale=40, rough=0.6, bump=0.0),
    "flower": lib.mat_flat("flower", (0.9, 0.9, 0.9), rough=0.5),
    "burlap": lib.mat_noise("burlap", [(0.3, (0.36, 0.26, 0.13)), (0.7, (0.55, 0.43, 0.24))], scale=45, rough=1.0, bump=0.35),
    "grain": lib.mat_noise("grain", [(0.35, (0.45, 0.28, 0.06)), (0.65, (0.75, 0.52, 0.14))], scale=70, rough=0.8, bump=0.3),
    "glow": lib.mat_flat("glow", (1.0, 0.7, 0.3), emission=(1.0, 0.55, 0.18), emission_strength=14),
    "cloth": lib.mat_noise("cloth", [(0.3, (0.02, 0.05, 0.2)), (0.7, (0.035, 0.09, 0.3))], scale=30, rough=0.9, bump=0.1),
    "gold": lib.mat_flat("gold", (0.85, 0.55, 0.12), rough=0.35, metallic=1.0),
    "interior": lib.mat_noise("interior", [(0.3, (0.02, 0.012, 0.007)), (0.7, (0.05, 0.03, 0.016))], scale=8, rough=1.0, bump=0.0),
}
b = lib.Builder(M, seed=21)
rng = b.rng

# ------------------------------------------------------------- dimensions
HX, HY = 1.15, 1.55            # half width (x), half length (y, ridge axis)
PL = 0.5                       # plinth height
TOP = 2.05                     # wall top plate
MID = 1.3                      # mid rail
PITCH = math.radians(42)
TANP = math.tan(PITCH)
RIDGE = TOP + HX * TANP
EAVE_OUT = 0.22
EAVE_Z = TOP - EAVE_OUT * TANP
ROOF_HY = HY + 0.26
DW = 0.62                      # half door width
DH = 1.72                      # door height
REC = 0.55                     # depth of the door recess
T = 0.075                      # timber thickness
SILL = PL + 0.03
FRONT_N, RIGHT_N = (0, -1, 0), (1, 0, 0)


def fy(x, z, out=0.0):
    return (x, -HY - 0.02 - out, z)


def rx(y, z, out=0.0):
    return (HX + 0.02 + out, y, z)


def fbeam(x1, z1, x2, z2, t=T, d=0.07, out=0.0):
    b.beam("wood_dark", fy(x1, z1, out), fy(x2, z2, out), t, d, FRONT_N)


def rbeam(y1, z1, y2, z2, t=T, d=0.07, out=0.0):
    b.beam("wood_dark", rx(y1, z1, out), rx(y2, z2, out), t, d, RIGHT_N)


def slope_z(x):
    return RIDGE - abs(x) * TANP


# ------------------------------------------------------------- ground
GC = Vector((0.25, -0.4, 0))
dirt_pts = None
b.blob_disc("dirt", 2.75, 0.04)
for obj_v in b.bms["dirt"].verts:          # shift the patch toward the facade
    obj_v.co += GC
for _ in range(90):
    a, r = rng.uniform(0, math.tau), rng.uniform(0.6, 2.6)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    if abs(x) < HX + 1.0 and -HY - 0.2 < y < HY + 0.2:
        continue
    props_scale = rng.uniform(0.03, 0.075)
    b.sphere("pebble", (x, y, 0.0), props_scale, scale=(1.2, 1, 0.45), segments=8, tint=props._shade(rng, 0.75, 1.25))
for row in range(7):                       # cobbled apron in front of the doors
    y = -HY - 0.12 - row * 0.15
    for i in range(9):
        x = -0.62 + i * 0.155 + (0.077 if row % 2 else 0) + rng.uniform(-0.02, 0.02)
        if rng.random() < 0.05 + row * 0.1 or abs(x) > 0.7:
            continue
        s = rng.uniform(0.85, 1.15)
        b.box("pebble", (x, y + rng.uniform(-0.02, 0.02), 0.012), (0.13 * s, 0.12 * s, 0.04),
              Matrix.Rotation(rng.uniform(-0.3, 0.3), 3, "Z"), tint=props._shade(rng, 0.8, 1.2))
b.blob_disc("grain", 0.42, 0.006, segments=24, wobble=0.35, z_top=0.03)
for v in b.bms["grain"].verts:
    v.co += Vector((0.1, -HY - 0.18, 0))
    v.co.y = min(v.co.y, -HY + 0.3)

for _ in range(150):
    a, r = rng.uniform(0, math.tau), rng.uniform(1.5, 2.75)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    if abs(x) < 0.9 and y < -HY:
        continue
    if abs(x) < HX + 0.95 and -HY - 0.25 < y < HY + 0.1:
        continue
    props.tuft(b, x, y, rng.uniform(0.8, 1.4))
    if rng.random() < 0.12:
        props.flowers(b, x + 0.1, y + 0.05, rng.randint(3, 6))
for y in (-HY + 0.2, 0.3, 1.0, 1.4):
    props.tuft(b, HX + 0.12 + (0.95 if -1.05 < y < 0.9 else 0), y, 0.9)
for x in (-HX + 0.05, -0.85, 0.85, HX - 0.05):
    props.tuft(b, x, -HY - 0.14, 0.85)

# ------------------------------------------------------------- masonry & walls
# Main body sits behind the door recess; the facade is three blocks around the opening.
b.box("stone", (0, (-HY + REC + HY) / 2 + 0.03, PL / 2), (2 * HX + 0.12, 2 * HY - REC + 0.06, PL))
b.box("plaster", (0, (-HY + REC + HY) / 2, (PL + TOP) / 2), (2 * HX, 2 * HY - REC, TOP - PL))
for sx in (-1, 1):
    cx = sx * (DW + HX) / 2
    w = HX - DW
    b.box("stone", (cx + sx * 0.03, -HY + REC / 2 - 0.03, PL / 2), (w + 0.06, REC + 0.06, PL))
    b.box("plaster", (cx, -HY + REC / 2, (PL + TOP) / 2), (w, REC, TOP - PL))
b.box("plaster", (0, -HY + REC / 2, (DH + TOP) / 2), (2 * DW, REC, TOP - DH))
# recess interior: dark back wall, plank floor
b.box("interior", (0, -HY + REC - 0.01, DH / 2), (2 * DW, 0.02, DH))
for i in range(6):
    b.box("wood_light", (-DW + (i + 0.5) * (2 * DW / 6), -HY + REC / 2, 0.02), (2 * DW / 6 - 0.01, REC, 0.04),
          tint=props._shade(rng, 0.45, 0.65))
# gable triangles
for sy in (-1, 1):
    y = sy * HY
    b.prism("plaster", [(-HX, y, TOP), (HX, y, TOP), (0, y, RIDGE)], (0, -sy * 0.3, 0))
# corner quoins
for sx in (-1, 1):
    for sy in (-1, 1):
        for k in range(3):
            w = 0.27 if k % 2 == 0 else 0.18
            b.box("stone", (sx * (HX + 0.045), sy * (HY + 0.045), 0.09 + k * 0.165),
                  (w if k % 2 == 0 else 0.18, 0.18 if k % 2 == 0 else w, 0.15), tint=props._shade(rng, 0.9, 1.2))

# ------------------------------------------------------------- front facade timber
for x in (-HX, HX):
    fbeam(x, SILL, x, TOP)
fbeam(-HX, SILL, -DW - 0.05, SILL)
fbeam(DW + 0.05, SILL, HX, SILL)
fbeam(-HX, TOP, HX, TOP)
for s in (-1, 1):
    fbeam(s * HX, MID, s * (DW + 0.05), MID)
    fbeam(s * HX, SILL, s * (DW + 0.05), MID)
    fbeam(s * (DW + 0.05), MID, s * HX, TOP)
    b.beam("wood_dark", fy(s * (DW + 0.04), 0.0, 0.02), fy(s * (DW + 0.04), DH + 0.08, 0.02), 0.11, 0.1, FRONT_N)
b.beam("wood_dark", fy(-DW - 0.18, DH + 0.07, 0.03), fy(DW + 0.18, DH + 0.07, 0.03), 0.13, 0.11, FRONT_N)
for x in (-0.3, 0.3):
    fbeam(x, DH + 0.07, x, TOP)
# gable: collar, king post, struts, loft hatch
COLLAR = TOP + 0.62
half = (RIDGE - COLLAR) / TANP
fbeam(-half, COLLAR, half, COLLAR)
fbeam(0, COLLAR, 0, RIDGE - 0.05)
for s in (-1, 1):
    fbeam(s * 0.78, TOP, s * 0.3, COLLAR)
    fbeam(s * 0.3, TOP, s * 0.3, COLLAR)
LOFT_Z0, LOFT_Z1 = TOP + 0.08, TOP + 0.52
b.box("interior", fy(0, (LOFT_Z0 + LOFT_Z1) / 2, -0.015), (0.5, 0.02, LOFT_Z1 - LOFT_Z0))
fbeam(-0.3, LOFT_Z0, 0.3, LOFT_Z0)
props.door_leaf(b, fy(0.26, LOFT_Z0, 0.02), (0.17, -0.98), (0.98, 0.17), 0.5, LOFT_Z1 - LOFT_Z0, planks=4)
for i, (dx, dz) in enumerate(((-0.1, 0.0), (0.08, 0.0), (-0.01, 0.17))):
    props.sack(b, dx, -HY + 0.12, LOFT_Z0 + dz, size=0.11, yaw=rng.uniform(-0.4, 0.4))

# hoist beam out of the gable with a load of sacks on the rope
HOIST_Z = COLLAR + 0.22
b.box("wood_dark", (0, -HY - 0.3, HOIST_Z), (0.1, 1.05, 0.1))
b.cylinder("iron", (0, -HY - 0.72, HOIST_Z - 0.09), 0.065, 0.065, 0.03, segments=16, rot=Matrix.Rotation(math.pi / 2, 3, "Y"))
LOAD_Z = DH + 0.2
b.cylinder("burlap", (0, -HY - 0.78, (HOIST_Z + LOAD_Z + 0.25) / 2), 0.011, 0.011, HOIST_Z - LOAD_Z - 0.25, segments=6)
for s in (-1, 1):
    b.beam("burlap", (0, -HY - 0.78, LOAD_Z + 0.25), (s * 0.13, -HY - 0.78, LOAD_Z + 0.08), 0.012, 0.012, (0, 1, 0))
props.sack(b, -0.07, -HY - 0.78, LOAD_Z - 0.18, size=0.12, yaw=0.5, lean=0.3)
props.sack(b, 0.08, -HY - 0.76, LOAD_Z - 0.2, size=0.12, yaw=-0.3, lean=-0.3)

# ------------------------------------------------------------- doors and goods inside
leaf_w = DW - 0.02
props.door_leaf(b, fy(-DW + 0.01, 0.02, 0.04), (-0.342, -0.94), (-0.94, 0.342), leaf_w, DH - 0.06)
props.door_leaf(b, fy(DW - 0.01, 0.02, 0.04), (0.342, -0.94), (0.94, 0.342), leaf_w, DH - 0.06)
IN_Y = -HY + 0.3
for x, z, s in ((-0.42, 0.04, 0.15), (-0.18, 0.04, 0.15), (-0.31, 0.3, 0.14), (-0.44, 0.3, 0.13), (-0.37, 0.55, 0.13)):
    props.sack(b, x, IN_Y + rng.uniform(-0.05, 0.05), z, size=s, yaw=rng.uniform(-0.5, 0.5))
props.crate(b, (0.38, IN_Y + 0.05, 0.22), 0.36, 0.08)
props.crate(b, (0.4, IN_Y + 0.02, 0.57), 0.32, -0.12)
props.barrel(b, 0.12, IN_Y + 0.08, 0.04, h=0.4, r=0.14)
props.sack(b, 0.05, -HY - 0.2, 0.0, size=0.14, yaw=0.6, lean=0.35)

# lantern and banner on the facade
props.lantern(b, fy(DW + 0.24, DH - 0.12), FRONT_N)
b.beam("wood_dark", fy(-0.98, TOP - 0.18, 0.05), fy(-0.72, TOP - 0.18, 0.05), 0.04, 0.04, FRONT_N)
b.prism("cloth", [fy(-0.96, TOP - 0.2, 0.06), fy(-0.74, TOP - 0.2, 0.06), fy(-0.74, TOP - 0.72, 0.06),
                  fy(-0.85, TOP - 0.63, 0.06), fy(-0.96, TOP - 0.72, 0.06)], (0, -0.015, 0))
b.sphere("gold", fy(-0.85, TOP - 0.42, 0.085), 0.05, scale=(1, 0.4, 1.35), segments=10)
props.ivy(b, (-HX + 0.1, -HY - 0.08, 0.0), FRONT_N, 0.55, 1.5)
props.ivy(b, (-HX - 0.08, -HY + 0.15, 0.0), (-1, 0, 0), 0.4, 1.1, density=60)

# ------------------------------------------------------------- right side
for y in (-HY, -0.8, 0.0, 0.8, HY):
    rbeam(y, SILL, y, TOP)
rbeam(-HY, SILL, HY, SILL)
rbeam(-HY, TOP, HY, TOP)
rbeam(-HY, MID, HY, MID)
for y1, y2 in ((-HY, -0.8), (0.0, 0.8)):
    rbeam(y1, SILL, y2, MID)
    rbeam(y2, MID, y1, TOP)
for y1, y2 in ((-0.8, 0.0),):
    rbeam(y2, SILL, y1, MID)
    rbeam(y1, MID, y2, TOP)
WY, WW, WZ0, WZ1 = 1.18, 0.4, 1.05, 1.6
b.box("interior", rx(WY, (WZ0 + WZ1) / 2, -0.01), (0.02, WW, WZ1 - WZ0))
for y in (WY - WW / 2, WY + WW / 2):
    rbeam(y, WZ0 - 0.05, y, WZ1 + 0.05)
for z in (WZ0, WZ1):
    rbeam(WY - WW / 2 - 0.06, z, WY + WW / 2 + 0.06, z)
b.beam("wood_dark", rx(WY, WZ0, 0.01), rx(WY, WZ1, 0.01), 0.03, 0.03, RIGHT_N)
for side in (-1, 1):
    hinge = rx(WY + side * WW / 2, WZ0 + 0.02, 0.03)
    ang = math.radians(75) * side
    along = Matrix.Rotation(ang, 3, "Z") @ Vector((0, side, 0))
    props.door_leaf(b, hinge, (along.x, along.y), (math.cos(ang), math.sin(ang)), WW / 2, WZ1 - WZ0 - 0.04, planks=2)

# lean-to shed along the right wall
LT_Y0, LT_Y1 = -1.05, 0.85
LT_OUT = 0.95
LT_HI, LT_LO = EAVE_Z - 0.16, 1.22
for y in (LT_Y0 + 0.08, (LT_Y0 + LT_Y1) / 2, LT_Y1 - 0.08):
    b.box("wood_dark", (HX + LT_OUT - 0.08, y, LT_LO / 2), (0.1, 0.1, LT_LO))
    b.beam("wood_dark", (HX + LT_OUT - 0.08, y, LT_LO - 0.32), (HX + LT_OUT - 0.4, y, LT_LO - 0.02), 0.06, 0.06, (0, 1, 0))
b.box("wood_dark", (HX + LT_OUT - 0.08, (LT_Y0 + LT_Y1) / 2, LT_LO - 0.02), (0.1, LT_Y1 - LT_Y0, 0.1))
b.box("wood_dark", (HX + 0.06, (LT_Y0 + LT_Y1) / 2, LT_HI - 0.02), (0.1, LT_Y1 - LT_Y0, 0.1))
lt_eave = Vector((HX + LT_OUT + 0.1, (LT_Y0 + LT_Y1) / 2, LT_LO - (0.1 * (LT_HI - LT_LO) / LT_OUT)))
lt_top = Vector((HX + 0.02, (LT_Y0 + LT_Y1) / 2, LT_HI))
lt_up = (lt_top - lt_eave).normalized()
lt_n = Vector((lt_top.z - lt_eave.z, 0, lt_eave.x - lt_top.x)).normalized()
b.box("wood_dark", (lt_eave + lt_top) / 2 + lt_n * 0.03, (LT_Y1 - LT_Y0 + 0.12, (lt_top - lt_eave).length, 0.04),
      Matrix((lt_up.cross(lt_n), lt_up, lt_n)).transposed())
lib.shingles(b, "slate", lt_eave, lt_up, lt_n, (LT_Y1 - LT_Y0) / 2 + 0.06, (lt_top - lt_eave).length, lift=0.07)
# stores under the shed
props.barrel(b, HX + 0.3, -0.75)
props.barrel(b, HX + 0.62, -0.62, h=0.42)
props.barrel(b, HX + 0.45, -0.25, lying=True, yaw=0.3)
props.crate(b, (HX + 0.35, 0.3, 0.18), 0.36, 0.1)
props.crate(b, (HX + 0.36, 0.3, 0.53), 0.32, -0.2)
props.crate(b, (HX + 0.68, 0.55, 0.16), 0.32, 0.4)
props.sack(b, HX + 0.7, 0.12, 0.0, size=0.14, yaw=0.4)
props.sack(b, HX + 0.62, -0.05, 0.0, size=0.13, yaw=-0.3, lean=0.3)

# ------------------------------------------------------------- back / left skeleton
for x, y in ((-HX, HY), (-HX, -HY), (HX, HY)):
    b.box("wood_dark", (x * 1.012, y * 1.012, (SILL + TOP) / 2), (T, T, TOP - SILL))
b.box("wood_dark", (0, HY + 0.02, TOP), (2 * HX, 0.07, T))
b.box("wood_dark", (-HX - 0.02, 0, TOP), (0.07, 2 * HY, T))

# ------------------------------------------------------------- dormer on the right slope
DY, DHW = -0.15, 0.3            # centre y, half width
DX0, DX1 = 0.15, 0.8            # back / front x
DZ0, DZ1 = slope_z(DX1) - 0.12, slope_z(DX1) + 0.38
D_RIDGE = DZ1 + 0.26
b.box("plaster", ((DX0 + DX1) / 2, DY, (DZ0 + DZ1) / 2), (DX1 - DX0, 2 * DHW, DZ1 - DZ0))
b.prism("plaster", [(DX1, DY - DHW, DZ1), (DX1, DY + DHW, DZ1), (DX1, DY, D_RIDGE)], (-(DX1 - DX0), 0, 0))
for s in (-1, 1):
    b.box("wood_dark", (DX1 + 0.02, DY + s * (DHW - 0.02), (DZ0 + DZ1) / 2), (0.07, 0.07, DZ1 - DZ0))
b.box("wood_dark", (DX1 + 0.02, DY, DZ1), (0.07, 2 * DHW + 0.04, 0.07))
b.box("interior", (DX1 + 0.005, DY, (DZ0 + DZ1) / 2 + 0.06), (0.02, 0.26, 0.26))
b.box("wood_dark", (DX1 + 0.025, DY, (DZ0 + DZ1) / 2 + 0.06), (0.03, 0.03, 0.26))
b.box("wood_dark", (DX1 + 0.025, DY, (DZ0 + DZ1) / 2 + 0.06), (0.03, 0.26, 0.03))
for s in (-1, 1):
    eave = Vector(((DX0 + DX1) / 2 + 0.05, DY + s * (DHW + 0.09), DZ1 - 0.07))
    top = Vector(((DX0 + DX1) / 2 + 0.05, DY, D_RIDGE + 0.02))
    up = (top - eave).normalized()
    n = Vector((0, s * (top.z - eave.z), abs(top.y - eave.y))).normalized()
    b.box("wood_dark", (eave + top) / 2 + n * 0.02, (DX1 - DX0 + 0.12, (top - eave).length, 0.03),
          Matrix((up.cross(n), up, n)).transposed())
    lib.shingles(b, "slate", eave, up, n, (DX1 - DX0) / 2 + 0.06, (top - eave).length, sw=0.12, sl=0.16, step=0.085, lift=0.05)
b.beam("wood_dark", (DX1 + 0.08, DY - DHW - 0.09, DZ1 - 0.07), (DX1 + 0.08, DY, D_RIDGE + 0.02), 0.06, 0.04, (1, 0, 0))
b.beam("wood_dark", (DX1 + 0.08, DY + DHW + 0.09, DZ1 - 0.07), (DX1 + 0.08, DY, D_RIDGE + 0.02), 0.06, 0.04, (1, 0, 0))

# ------------------------------------------------------------- main roof
CH_X, CH_Y = -0.55, 1.0          # chimney


def skip_tile(c):
    if c.x > 0.05 and DX0 - 0.05 < c.x < DX1 + 0.1 and abs(c.y - DY) < DHW + 0.08:
        return True
    return abs(c.x - CH_X) < 0.26 and abs(c.y - CH_Y) < 0.26


for s in (-1, 1):
    eave = Vector((s * (HX + EAVE_OUT), 0, EAVE_Z))
    ridge = Vector((0, 0, RIDGE))
    up = (ridge - eave).normalized()
    length = (ridge - eave).length
    n = Vector((s * math.sin(PITCH), 0, math.cos(PITCH)))
    base = Matrix((up.cross(n), up, n)).transposed()
    b.box("wood_dark", (eave + ridge) / 2 + n * 0.02, (2 * ROOF_HY, length, 0.05), base)
    for y in [-ROOF_HY + 0.06 + i * 0.33 for i in range(12)]:
        b.box("wood_dark", eave + Vector((0, y, 0)) - n * 0.04 + up * 0.05, (0.07, 0.16, 0.07), base)
    lib.shingles(b, "slate", eave, up, n, ROOF_HY, length, skip=skip_tile)
    for sy in (-1, 1):  # barge boards with a little carved overhang
        b.beam("wood_dark", eave + Vector((0, sy * ROOF_HY, 0)) + n * 0.06, ridge + Vector((0, sy * ROOF_HY, 0)) + n * 0.06,
               0.12, 0.06, (0, sy, 0))
    b.beam("wood_dark", eave + Vector((0, -ROOF_HY, 0)) - n * 0.02, eave + Vector((0, ROOF_HY, 0)) - n * 0.02, 0.08, 0.06, n)
y = -ROOF_HY
while y < ROOF_HY:                       # ridge cap
    g = rng.uniform(0.8, 1.1)
    b.box("slate", (0, y + 0.13, RIDGE + 0.1), (0.17, 0.25, 0.17), Matrix.Rotation(math.pi / 4, 3, "Y"), tint=(g, g, g))
    y += 0.27
for sy in (-1, 1):                       # carved finials at the gable peaks
    b.box("wood_dark", (0, sy * (ROOF_HY + 0.02), RIDGE + 0.1), (0.07, 0.07, 0.36))

# chimney and ridge ventilator
b.box("stone", (CH_X, CH_Y, 2.75), (0.42, 0.42, 1.9))
b.box("stone", (CH_X, CH_Y, 3.72), (0.54, 0.54, 0.09), tint=(0.85, 0.85, 0.85))
b.cylinder("wood_dark", (CH_X, CH_Y, 3.82), 0.08, 0.07, 0.13, segments=12, tint=(0.75, 0.5, 0.42))
VY = -0.85
b.box("wood_light", (0, VY, RIDGE + 0.12), (0.38, 0.38, 0.42))
for s in (-1, 1):
    for k in range(3):
        b.box("wood_dark", (s * 0.195, VY, RIDGE + 0.04 + k * 0.1), (0.02, 0.3, 0.035), Matrix.Rotation(s * 0.5, 3, "Y"))
        b.box("wood_dark", (0, VY - 0.195, RIDGE + 0.04 + k * 0.1), (0.3, 0.02, 0.035), Matrix.Rotation(-0.5, 3, "X"))
b.cylinder("slate", (0, VY, RIDGE + 0.5), 0.36, 0.0, 0.34, segments=4, rot=Matrix.Rotation(math.pi / 4, 3, "Z"), tint=(0.85, 0.85, 0.85))
b.sphere("gold", (0, VY, RIDGE + 0.72), 0.04, segments=10)

# ------------------------------------------------------------- yard
props.fence(b, (-HX - 0.1, -HY - 0.95), (-2.0, -HY - 0.95))
props.fence(b, (-2.0, -HY - 0.95), (-2.0, -0.4))
props.barrel(b, -1.55, -HY - 0.55)
props.sack(b, -1.78, -HY - 0.3, 0.0, size=0.15, yaw=0.2)
props.sack(b, -1.5, -HY - 0.18, 0.0, size=0.14, yaw=-0.5)
props.crate(b, (-1.75, -1.0, 0.17), 0.34, 0.3)
props.cart(b, (1.25, -HY - 0.75, 0), yaw=math.radians(200))

# ------------------------------------------------------------- output
b.build(bevel=0.012, no_bevel=("dirt", "grass", "burlap", "pebble", "glow", "cloth", "leaf", "flower", "grain"),
        outline=OUTLINE, no_outline=("grass", "leaf", "flower", "grain", "glow"))
cam = lib.add_game_camera()
lib.frame_camera(cam)
os.makedirs(os.path.dirname(OUT), exist_ok=True)
lib.save_outputs(os.path.join(HERE, "..", "models"), "warehouse")
lib.render(OUT)
