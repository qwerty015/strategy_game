"""Warehouse (Склад): timber-framed barn on a stone plinth.

    blender -b --factory-startup --python warehouse.py -- <out.png> [resolution] [samples]
"""

import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from mathutils import Matrix, Vector  # noqa: E402

import lib  # noqa: E402

args = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []
OUT = os.path.abspath(args[0] if args else os.path.join(os.path.dirname(__file__), "..", "renders", "warehouse.png"))
RES = int(args[1]) if len(args) > 1 else 1024
SAMPLES = int(args[2]) if len(args) > 2 else 192

lib.clear_scene()
lib.setup_render(RES, SAMPLES)
lib.setup_world()
lib.add_sun()

M = {
    "plaster": lib.mat_noise("plaster", [(0.35, (0.52, 0.42, 0.28)), (0.7, (0.70, 0.60, 0.42))], scale=3, bump=0.15, rough=0.95),
    "wood_dark": lib.mat_wood("wood_dark", (0.07, 0.035, 0.018), (0.17, 0.09, 0.04)),
    "wood_light": lib.mat_wood("wood_light", (0.22, 0.12, 0.05), (0.42, 0.26, 0.12)),
    "stone": lib.mat_stone("stone", (0.22, 0.20, 0.17), (0.34, 0.31, 0.26), (0.09, 0.08, 0.07), scale=2.6),
    "slate": lib.mat_noise(
        "slate",
        [(0.0, (0.018, 0.02, 0.03)), (0.55, (0.04, 0.045, 0.065)), (0.68, (0.045, 0.05, 0.06)), (0.76, (0.06, 0.08, 0.018))],
        scale=2.5, rough=0.6, bump=0.1,
    ),
    "iron": lib.mat_flat("iron", (0.05, 0.05, 0.055), rough=0.45, metallic=0.85),
    "dirt": lib.mat_noise("dirt", [(0.3, (0.15, 0.095, 0.05)), (0.7, (0.30, 0.20, 0.10))], scale=4, rough=1.0, bump=0.3),
    "pebble": lib.mat_noise("pebble", [(0.3, (0.18, 0.17, 0.15)), (0.7, (0.32, 0.30, 0.27))], scale=12, rough=0.9, bump=0.2),
    "grass": lib.mat_flat("grass", (0.10, 0.24, 0.035), rough=0.8),
    "burlap": lib.mat_noise("burlap", [(0.3, (0.34, 0.25, 0.13)), (0.7, (0.50, 0.40, 0.23))], scale=45, rough=1.0, bump=0.35),
    "glow": lib.mat_flat("glow", (1.0, 0.7, 0.3), emission=(1.0, 0.55, 0.18), emission_strength=12),
    "cloth": lib.mat_noise("cloth", [(0.3, (0.025, 0.06, 0.22)), (0.7, (0.04, 0.10, 0.32))], scale=30, rough=0.9, bump=0.1),
    "gold": lib.mat_flat("gold", (0.85, 0.55, 0.12), rough=0.35, metallic=1.0),
    "interior": lib.mat_flat("interior", (0.02, 0.012, 0.008), rough=1.0),
}
b = lib.Builder(M, seed=11)
rng = b.rng

# ------------------------------------------------------------- dimensions
HL, HW = 1.5, 1.1             # half length (x) / half width (y)
PLINTH = 0.55
TOP = 2.15                    # wall top plate
MID = 1.35                    # mid rail
PITCH = math.radians(37)
RIDGE = TOP + HW * math.tan(PITCH)
EAVE_OUT = 0.2
EAVE_Z = TOP - EAVE_OUT * math.tan(PITCH)
ROOF_HX = HL + 0.24
T = 0.075                      # timber thickness
FRONT = (0, -1, 0)
RIGHT = (1, 0, 0)


def fy(x, z, out=0.0):
    """Point on the front face."""
    return (x, -HW - 0.02 - out, z)


def rx(y, z, out=0.0):
    """Point on the right face."""
    return (HL + 0.02 + out, y, z)


# ------------------------------------------------------------- ground
b.blob_disc("dirt", 2.45, 0.04, tint=(1, 1, 1))
for _ in range(70):                            # pebbles
    a, r = rng.uniform(0, math.tau), rng.uniform(1.0, 2.3)
    x, y = math.cos(a) * r, math.sin(a) * r
    if abs(x) < HL + 0.1 and abs(y) < HW + 0.1:
        continue
    s = rng.uniform(0.03, 0.07)
    b.sphere("pebble", (x, y, 0.0), s, scale=(1.2, 1, 0.45), segments=8, tint=(rng.uniform(0.8, 1.2),) * 3)
# cobbled path out of the door
for row in range(6):
    y = -HW - 0.45 - row * 0.15
    for i in range(7):
        x = -0.48 + i * 0.16 + (0.08 if row % 2 else 0) + rng.uniform(-0.02, 0.02)
        if rng.random() < 0.12 + row * 0.08:
            continue
        s = rng.uniform(0.85, 1.15)
        b.box("pebble", (x, y + rng.uniform(-0.02, 0.02), 0.012), (0.13 * s, 0.12 * s, 0.035),
              Matrix.Rotation(rng.uniform(-0.3, 0.3), 3, "Z"), tint=(rng.uniform(0.8, 1.15),) * 3)


def tuft(x, y, size=1.0):
    for _ in range(rng.randint(5, 8)):
        h = rng.uniform(0.09, 0.2) * size
        tilt = Matrix.Rotation(rng.uniform(-0.45, 0.45), 3, "X") @ Matrix.Rotation(rng.uniform(-0.45, 0.45), 3, "Y")
        c = Vector((x + rng.uniform(-0.05, 0.05), y + rng.uniform(-0.05, 0.05), 0)) + tilt @ Vector((0, 0, h / 2))
        g = rng.uniform(0.75, 1.35)
        b.cylinder("grass", c, 0.016 * size, 0.0, h, segments=4, rot=tilt, tint=(g * 0.9, g, g * 0.8))


for _ in range(110):
    a = rng.uniform(0, math.tau)
    r = rng.uniform(1.6, 2.45)
    x, y = math.cos(a) * r, math.sin(a) * r
    if abs(x) < 0.6 and y < 0:          # keep the path clear
        continue
    tuft(x, y, rng.uniform(0.8, 1.3))
for x in (-1.45, -1.1, 1.2, 1.45):       # tufts hugging the plinth
    tuft(x, -HW - 0.12, 0.8)
for y in (-0.8, 0.2, 0.95):
    tuft(HL + 0.12, y, 0.8)

# ------------------------------------------------------------- body
b.box("stone", (0, 0, PLINTH / 2), (2 * HL + 0.12, 2 * HW + 0.12, PLINTH))
b.box("plaster", (0, 0, (PLINTH + TOP) / 2), (2 * HL, 2 * HW, TOP - PLINTH))
# corner quoins: larger stones sticking out a little at the plinth corners
for sx in (-1, 1):
    for sy in (-1, 1):
        for k in range(3):
            z = 0.1 + k * 0.18
            w = 0.26 if k % 2 == 0 else 0.18
            b.box("stone", (sx * (HL + 0.04), sy * (HW + 0.04), z), (w if k % 2 else 0.18, 0.18 if k % 2 else w, 0.16),
                  tint=(rng.uniform(0.9, 1.15),) * 3)

# gable triangles
for sx in (-1, 1):
    x = sx * HL
    b.prism("plaster", [(x, -HW, TOP), (x, HW, TOP), (x, 0, RIDGE)], (-sx * 0.25, 0, 0))

# ------------------------------------------------------------- timber frame
SILL = PLINTH + 0.03


def front_beam(x1, z1, x2, z2):
    b.beam("wood_dark", fy(x1, z1), fy(x2, z2), T, 0.07, FRONT)


def right_beam(y1, z1, y2, z2):
    b.beam("wood_dark", rx(y1, z1), rx(y2, z2), T, 0.07, RIGHT)


# front: posts, plates, rails, braces (door gap in the middle)
for x in (-HL, -0.95, 0.95, HL):
    front_beam(x, SILL, x, TOP)
front_beam(-HL, SILL, -0.55, SILL)
front_beam(0.55, SILL, HL, SILL)
front_beam(-HL, TOP, HL, TOP)
for x1, x2 in ((-HL, -0.95), (0.95, HL)):
    front_beam(x1, MID, x2, MID)
front_beam(-HL, SILL, -0.95, MID)
front_beam(HL, SILL, 0.95, MID)
front_beam(-0.95, MID, -HL, TOP)
front_beam(0.95, MID, HL, TOP)
front_beam(-0.95, SILL, -0.55, TOP)
front_beam(0.95, SILL, 0.55, TOP)
# door frame (runs down to the ground)
for x in (-0.55, 0.55):
    b.beam("wood_dark", fy(x, 0.0, 0.02), fy(x, 1.62, 0.02), 0.1, 0.1, FRONT)
b.beam("wood_dark", fy(-0.68, 1.62, 0.03), fy(0.68, 1.62, 0.03), 0.12, 0.11, FRONT)
for x in (-0.28, 0.0, 0.28):
    front_beam(x, 1.62, x, TOP)

# right side: window between the back posts
WIN_Y, WIN_W, WIN_Z0, WIN_Z1 = 0.72, 0.44, 1.12, 1.72
for y in (-HW, -0.3, 0.3, HW):
    right_beam(y, SILL, y, TOP)
right_beam(-HW, SILL, HW, SILL)
right_beam(-HW, TOP, HW, TOP)
right_beam(-HW, MID, 0.3, MID)
right_beam(-HW, SILL, -0.3, MID)
right_beam(-0.3, MID, -HW, TOP)
right_beam(0.3, SILL, -0.3, MID)
right_beam(-0.3, MID, 0.3, TOP)
# window
b.box("interior", rx(WIN_Y, (WIN_Z0 + WIN_Z1) / 2, -0.01), (0.02, WIN_W, WIN_Z1 - WIN_Z0))
for y in (WIN_Y - WIN_W / 2, WIN_Y + WIN_W / 2):
    right_beam(y, WIN_Z0 - 0.04, y, WIN_Z1 + 0.04)
right_beam(WIN_Y - WIN_W / 2 - 0.05, WIN_Z0, WIN_Y + WIN_W / 2 + 0.05, WIN_Z0)
right_beam(WIN_Y - WIN_W / 2 - 0.05, WIN_Z1, WIN_Y + WIN_W / 2 + 0.05, WIN_Z1)
b.beam("wood_dark", rx(WIN_Y, WIN_Z0, 0.01), rx(WIN_Y, WIN_Z1, 0.01), 0.03, 0.03, RIGHT)
for side in (-1, 1):                      # open shutters
    hinge = Vector(rx(WIN_Y + side * WIN_W / 2, (WIN_Z0 + WIN_Z1) / 2, 0.03))
    ang = side * math.radians(70)
    d = Matrix.Rotation(ang, 3, "Z") @ Vector((0, side, 0))
    for i in range(3):
        c = hinge + d * (0.035 + i * 0.075)
        b.box("wood_light", c, (0.03, 0.068, WIN_Z1 - WIN_Z0 - 0.02), Matrix.Rotation(ang, 3, "Z"),
              tint=(rng.uniform(0.85, 1.1),) * 3)

# back / left: just corner posts and plates so silhouettes read right
for x, y in ((-HL, HW), (-HL, -HW), (HL, HW)):
    b.box("wood_dark", (x * 1.013, y * 1.018, (SILL + TOP) / 2), (T, T, TOP - SILL))
b.box("wood_dark", (0, HW + 0.02, TOP), (2 * HL, 0.07, T))
b.box("wood_dark", (-HL - 0.02, 0, TOP), (0.07, 2 * HW, T))

# gable timber + loft hatch + hoist (right gable is the visible one)
GX = HL + 0.02
b.beam("wood_dark", (GX, 0, TOP), (GX, 0, TOP + 0.07), T, 0.07, RIGHT)
COLLAR = TOP + 0.57
half = (RIDGE - COLLAR) / math.tan(PITCH)
b.beam("wood_dark", (GX, -half, COLLAR), (GX, half, COLLAR), T, 0.07, RIGHT)
b.beam("wood_dark", (GX, 0, COLLAR), (GX, 0, RIDGE - 0.05), T, 0.07, RIGHT)
for s in (-1, 1):
    b.beam("wood_dark", (GX, s * 0.62, TOP), (GX, s * 0.24, COLLAR), T, 0.07, RIGHT)
    b.beam("wood_dark", (GX, s * 0.24, TOP + 0.07), (GX, s * 0.24, COLLAR), T, 0.07, RIGHT)
b.beam("wood_dark", (GX, -0.24, TOP + 0.07), (GX, 0.24, TOP + 0.07), T, 0.07, RIGHT)
b.box("interior", (HL + 0.005, 0, TOP + 0.32), (0.02, 0.42, 0.46))
hatch_rot = Matrix.Rotation(math.radians(-75), 3, "Z")
for i in range(3):
    c = Vector((HL + 0.03, 0.22, TOP + 0.32)) + hatch_rot @ Vector((0, 0.035 + i * 0.07, 0))
    b.box("wood_light", c, (0.03, 0.065, 0.42), hatch_rot, tint=(rng.uniform(0.85, 1.1),) * 3)
# hoist beam, pulley, rope and a crate on the hook
b.box("wood_dark", (HL + 0.28, 0, COLLAR + 0.16), (0.9, 0.09, 0.09))
b.cylinder("iron", (HL + 0.62, 0, COLLAR + 0.07), 0.06, 0.06, 0.03, segments=14, rot=Matrix.Rotation(math.pi / 2, 3, "X"))
b.cylinder("burlap", (HL + 0.62, 0, TOP + 0.07), 0.012, 0.012, COLLAR - TOP, segments=6)
b.cylinder("burlap", (HL + 0.56, 0, TOP + 0.2), 0.01, 0.01, COLLAR - TOP - 0.13, segments=6)
for s in (-1, 1):
    b.beam("burlap", (HL + 0.62, 0, TOP - 0.2), (HL + 0.62 + s * 0.12, 0, TOP - 0.4), 0.012, 0.012, (0, 1, 0))


def crate(c, size, rot=0.0):
    cx, cy, cz = c
    r = Matrix.Rotation(rot, 3, "Z")
    b.box("wood_light", (cx, cy, cz), (size, size, size), r, tint=(rng.uniform(0.85, 1.1),) * 3)
    h = size / 2 + 0.008
    for ex in (-1, 1):
        for ey in (-1, 1):
            b.box("wood_dark", Vector(c) + r @ Vector((ex * (h - 0.02), ey * (h - 0.02), 0)), (0.045, 0.045, size + 0.012), r)
    for ez in (-1, 1):
        b.box("wood_dark", Vector(c) + Vector((0, 0, ez * (h - 0.02))), (size + 0.012, size + 0.012, 0.035), r)
    b.beam("wood_dark", Vector(c) + r @ Vector((-h + 0.03, -h, -h + 0.03)), Vector(c) + r @ Vector((h - 0.03, -h, h - 0.03)),
           0.04, 0.02, r @ Vector((0, -1, 0)))


crate((HL + 0.62, 0, TOP - 0.56), 0.3, 0.2)

# ------------------------------------------------------------- doors
b.box("interior", fy(0, 0.78, -0.04), (1.0, 0.02, 1.56))
b.box("stone", (0, -HW - 0.2, 0.05), (1.25, 0.34, 0.1), tint=(0.95, 0.95, 0.95))


def door_leaf(hinge, angle, width, height, flip=False):
    rot = Matrix.Rotation(angle, 3, "Z")
    o = Vector(hinge)
    sgn = -1 if flip else 1

    def P(lx, ly, lz):
        return o + rot @ Vector((sgn * lx, ly, lz))

    n = 4
    pw = width / n
    for i in range(n):
        b.box("wood_light", P((i + 0.5) * pw, 0, height / 2), (pw - 0.012, 0.05, height), rot,
              tint=(rng.uniform(0.8, 1.12),) * 3)
    outward = rot @ Vector((0, -1, 0))
    for z in (0.22, height - 0.22):
        b.beam("wood_light", P(0.03, -0.045, z), P(width - 0.03, -0.045, z), 0.09, 0.04, outward, tint=(0.8, 0.8, 0.8))
        b.beam("iron", P(0.0, -0.07, z + 0.07), P(width * 0.75, -0.07, z + 0.07), 0.035, 0.01, outward)
    b.beam("wood_light", P(0.06, -0.045, 0.27), P(width - 0.06, -0.045, height - 0.27), 0.08, 0.04, outward, tint=(0.8, 0.8, 0.8))
    b.cylinder("iron", P(width - 0.09, -0.08, height / 2), 0.045, 0.045, 0.012, segments=14,
               rot=rot @ Matrix.Rotation(math.pi / 2, 3, "X"))


door_leaf(fy(0.0, 0.0, 0.02), 0.0, 0.5, 1.5)
door_leaf(fy(-0.5, 0.0, 0.02), math.radians(-150), 0.5, 1.5, flip=False)

# goods visible in the open half of the doorway
for i, (x, z) in enumerate(((-0.36, 0.13), (-0.16, 0.13), (-0.27, 0.36))):
    b.sphere("burlap", (x, -HW - 0.08, z), 0.14, scale=(0.85, 0.7, 1.0), segments=14, tint=(rng.uniform(0.85, 1.1),) * 3)

# ------------------------------------------------------------- roof
rot_cache = {}
for s in (-1, 1):                          # -1 = front slope, +1 = back slope
    eave = Vector((0, s * (HW + EAVE_OUT), EAVE_Z))
    ridge = Vector((0, 0, RIDGE))
    up = (ridge - eave).normalized()
    length = (ridge - eave).length
    n = Vector((0, s * math.sin(PITCH), math.cos(PITCH)))
    x_axis = up.cross(n)
    base = Matrix((x_axis, up, n)).transposed()
    # deck + rafter ends
    b.box("wood_dark", (eave + ridge) / 2 + n * 0.02, (2 * ROOF_HX, length, 0.05), base)
    for x in [-ROOF_HX + 0.05 + i * 0.32 for i in range(12)]:
        b.box("wood_dark", eave + Vector((x, 0, 0)) - n * 0.04 + up * 0.05, (0.06, 0.16, 0.07), base)
    # shingles
    SW, SL, STEP = 0.16, 0.21, 0.11
    row = 0
    while row * STEP + SL <= length + 0.03:
        t = row * STEP + SL / 2
        off = SW / 2 if row % 2 else 0
        x = -ROOF_HX + SW / 2 - off
        while x <= ROOF_HX:
            cx = max(-ROOF_HX + SW / 2, min(ROOF_HX - SW / 2, x)) if abs(x) > ROOF_HX - SW / 2 else x
            w = SW - 0.012 if abs(x) <= ROOF_HX - SW / 2 else SW / 2
            if abs(x) > ROOF_HX - SW / 2:
                cx = math.copysign(ROOF_HX - SW / 4, x)
            jitter = Matrix.Rotation(rng.uniform(-0.05, 0.05), 3, "Z")
            tilt = Matrix.Rotation(math.radians(-7), 3, "X")
            r = base @ jitter @ tilt
            c = eave + Vector((cx + rng.uniform(-0.01, 0.01), 0, 0)) + up * t + n * (0.07 + 0.004 * rng.random())
            g = rng.uniform(0.7, 1.25)
            b.box("slate", c, (w, SL, 0.03), r, tint=(g, g, g * rng.uniform(0.95, 1.08)))
            x += SW
        row += 1
    # barge boards and eave fascia
    for sx in (-1, 1):
        b.beam("wood_dark", Vector((sx * ROOF_HX, 0, 0)) + eave + n * 0.06, Vector((sx * ROOF_HX, 0, RIDGE)) + n * 0.06,
               0.11, 0.06, (sx, 0, 0))
# ridge cap
x = -ROOF_HX
while x < ROOF_HX:
    g = rng.uniform(0.8, 1.1)
    b.box("slate", (x + 0.13, 0, RIDGE + 0.1), (0.25, 0.17, 0.17), Matrix.Rotation(math.pi / 4, 3, "X"), tint=(g, g, g))
    x += 0.27

# chimney through the back slope
b.box("stone", (-0.8, 0.55, 2.65), (0.42, 0.42, 1.7))
b.box("stone", (-0.8, 0.55, 3.52), (0.54, 0.54, 0.09), tint=(0.85, 0.85, 0.85))
b.cylinder("wood_dark", (-0.8, 0.55, 3.6), 0.08, 0.07, 0.12, segments=12, tint=(0.6, 0.45, 0.4))

# ------------------------------------------------------------- decoration
# lantern right of the door
b.beam("iron", fy(0.78, 1.62, 0.0), fy(0.78, 1.62, 0.2), 0.03, 0.03, (1, 0, 0))
b.box("iron", fy(0.78, 1.48, 0.2), (0.14, 0.14, 0.03))
b.box("iron", fy(0.78, 1.30, 0.2), (0.14, 0.14, 0.03))
b.box("glow", fy(0.78, 1.39, 0.2), (0.1, 0.1, 0.16))
b.cylinder("iron", fy(0.78, 1.53, 0.2), 0.07, 0.0, 0.08, segments=4, rot=Matrix.Rotation(math.pi / 4, 3, "Z"))

# banner left of the door
b.beam("wood_dark", fy(-1.33, 1.85, 0.05), fy(-1.07, 1.85, 0.05), 0.04, 0.04, FRONT)
b.prism("cloth", [fy(-1.31, 1.83, 0.06), fy(-1.09, 1.83, 0.06), fy(-1.09, 1.3, 0.06), fy(-1.2, 1.38, 0.06), fy(-1.31, 1.3, 0.06)],
        (0, -0.015, 0))
b.sphere("gold", fy(-1.2, 1.6, 0.085), 0.045, scale=(1, 0.4, 1.3), segments=10)

# barrels front-left
def barrel(x, y, z=0.0, h=0.44, r=0.16):
    tone = rng.uniform(0.85, 1.1)
    b.cylinder("wood_light", (x, y, z + h * 0.25), r * 0.88, r, h / 2, segments=20, tint=(tone,) * 3)
    b.cylinder("wood_light", (x, y, z + h * 0.75), r, r * 0.88, h / 2, segments=20, tint=(tone,) * 3)
    for zz, rr in ((0.07, 0.905), (0.38, 0.985), (0.62, 0.985), (0.93, 0.905)):
        b.cylinder("iron", (x, y, z + h * zz), r * rr + 0.008, r * rr + 0.008, 0.025, segments=20)
    b.cylinder("wood_dark", (x, y, z + h + 0.002), r * 0.84, r * 0.84, 0.01, segments=20)


barrel(-1.22, -1.48)
barrel(-1.0, -1.78)
barrel(-1.32, -1.15, h=0.4)
crate((1.05, -1.5, 0.17), 0.34, 0.15)
crate((1.38, -1.42, 0.16), 0.32, -0.2)
crate((1.18, -1.47, 0.5), 0.3, 0.35)
for x, y in ((-0.66, -1.62), (-0.62, -1.38)):
    b.sphere("burlap", (x, y, 0.13), 0.15, scale=(0.9, 0.75, 1.0), segments=14, tint=(rng.uniform(0.85, 1.1),) * 3)

# ------------------------------------------------------------- render
b.build(bevel=0.012, no_bevel=("dirt", "grass", "burlap", "pebble", "glow", "cloth"))
cam = lib.add_game_camera()
lib.frame_camera(cam)
os.makedirs(os.path.dirname(OUT), exist_ok=True)
lib.save_outputs(os.path.join(os.path.dirname(__file__), "..", "models"), "warehouse")
lib.render(OUT)
