"""Bakery (Пекарня): timber house with a brick bread oven and a smoking chimney.

    blender -b --factory-startup --python bakery.py -- <out.png> [resolution] [samples] [smoke_frames]

Outputs (next to <out.png>):
  bakery.png              building + smoke (preview)
  bakery_base.png         building only -- the game sprite
  bakery_smoke_frames/    smoke only (building as holdout), same camera
  -> sheet.py turns the frames into bakery_smoke_sheet.png + a GIF preview.
The .blend plays the smoke loop in Blender (Space).
"""

import math
import os
import shutil
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import bpy  # noqa: E402
from mathutils import Matrix, Vector  # noqa: E402

import house  # noqa: E402
import lib  # noqa: E402
import props  # noqa: E402

args = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []
HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.abspath(args[0] if args else os.path.join(HERE, "..", "renders", "bakery.png"))
RES = int(args[1]) if len(args) > 1 else 1024
SAMPLES = int(args[2]) if len(args) > 2 else 160
SMOKE_FRAMES = int(args[3]) if len(args) > 3 else 16

lib.clear_scene()
lib.setup_render(RES, SAMPLES)
lib.setup_world()
lib.add_sun()

M = {
    "plaster": lib.mat_noise("plaster", [(0.3, (0.58, 0.47, 0.30)), (0.75, (0.78, 0.67, 0.47))], scale=3, bump=0.18, rough=0.95),
    "wood_dark": lib.mat_wood("wood_dark", (0.06, 0.028, 0.012), (0.17, 0.08, 0.032)),
    "wood_light": lib.mat_wood("wood_light", (0.24, 0.12, 0.045), (0.50, 0.29, 0.11)),
    "stone": lib.mat_stone("stone", (0.24, 0.21, 0.17), (0.38, 0.34, 0.28), (0.08, 0.07, 0.055), scale=2.6),
    "brick": lib.mat_stone("brick", (0.26, 0.085, 0.04), (0.38, 0.14, 0.07), (0.16, 0.13, 0.10), scale=5.5),
    "slate": lib.mat_noise(
        "slate",
        [(0.0, (0.06, 0.03, 0.022)), (0.5, (0.13, 0.06, 0.035)), (0.66, (0.15, 0.075, 0.045)), (0.74, (0.09, 0.11, 0.02))],
        scale=2.2, rough=0.6, bump=0.1,
    ),  # warm clay tiles set the bakery apart from the slate-roofed warehouse
    "iron": lib.mat_flat("iron", (0.04, 0.04, 0.045), rough=0.45, metallic=0.85),
    "dirt": lib.mat_noise("dirt", [(0.3, (0.22, 0.13, 0.05)), (0.72, (0.42, 0.28, 0.12))], scale=3.5, rough=1.0, bump=0.3),
    "pebble": lib.mat_noise("pebble", [(0.3, (0.17, 0.155, 0.13)), (0.7, (0.33, 0.30, 0.25))], scale=12, rough=0.9, bump=0.2),
    "grass": lib.mat_flat("grass", (0.09, 0.24, 0.025), rough=0.8),
    "leaf": lib.mat_noise("leaf", [(0.3, (0.04, 0.12, 0.015)), (0.7, (0.10, 0.26, 0.03))], scale=40, rough=0.6, bump=0.0),
    "flower": lib.mat_flat("flower", (0.9, 0.9, 0.9), rough=0.5),
    "burlap": lib.mat_noise("burlap", [(0.3, (0.36, 0.26, 0.13)), (0.7, (0.55, 0.43, 0.24))], scale=45, rough=1.0, bump=0.35),
    "grain": lib.mat_noise("grain", [(0.35, (0.45, 0.28, 0.06)), (0.65, (0.75, 0.52, 0.14))], scale=70, rough=0.8, bump=0.3),
    "bread": lib.mat_noise("bread", [(0.3, (0.32, 0.13, 0.03)), (0.6, (0.55, 0.28, 0.07)), (0.8, (0.70, 0.45, 0.16))],
                           scale=9, rough=0.55, bump=0.4),
    "canvas": lib.mat_noise("canvas", [(0.3, (0.62, 0.55, 0.40)), (0.7, (0.80, 0.73, 0.57))], scale=25, rough=0.95, bump=0.1),
    "red_cloth": lib.mat_noise("red_cloth", [(0.3, (0.30, 0.03, 0.02)), (0.7, (0.45, 0.06, 0.03))], scale=25, rough=0.9, bump=0.1),
    "glow": lib.mat_flat("glow", (1.0, 0.7, 0.3), emission=(1.0, 0.55, 0.18), emission_strength=14),
    "glow_soft": lib.mat_flat("glow_soft", (0.08, 0.04, 0.02), emission=(1.0, 0.55, 0.22), emission_strength=1.6),
    "fire": lib.mat_noise("fire", [(0.3, (1.0, 0.35, 0.05)), (0.7, (1.0, 0.75, 0.25))], scale=6, rough=1.0, bump=0.0),
    "interior": lib.mat_noise("interior", [(0.3, (0.02, 0.012, 0.007)), (0.7, (0.05, 0.03, 0.016))], scale=8, rough=1.0, bump=0.0),
}
fire = M["fire"].node_tree.nodes["Principled BSDF"]
fire.inputs["Emission Color"].default_value = (1.0, 0.42, 0.08, 1)
fire.inputs["Emission Strength"].default_value = 18

b = lib.Builder(M, seed=33)
rng = b.rng

HX, HY = 1.1, 0.88
h = house.House(
    b, HX, HY, top=1.95, ridge="x", pitch_deg=40,
    openings={
        "front": [
            dict(kind="door", u=-0.55, w=0.5, z0=0.0, z1=1.45, open_deg=0),
            dict(kind="window", u=0.38, w=0.7, z0=0.85, z1=1.45, glow=True, shutters=False),
        ],
        "right": [dict(kind="window", u=0.55, w=0.36, z0=1.25, z1=1.7, glow=True, box=True)],
    },
)
CH = Vector((-0.55, 0.35))                       # house chimney (on the back slope)
h.build_all(roof_skip=lambda c: abs(c.x - CH.x) < 0.25 and abs(c.y - CH.y) < 0.25)
b.box("stone", (CH.x, CH.y, 2.35), (0.38, 0.38, 1.6))
b.box("stone", (CH.x, CH.y, 3.18), (0.48, 0.48, 0.08), tint=(0.85, 0.85, 0.85))

# ------------------------------------------------------------- bread oven annex
OX0, OX1 = HX + 0.02, HX + 0.78
OY0, OY1 = -0.72, 0.24
OCX, OCY = (OX0 + OX1) / 2, (OY0 + OY1) / 2
b.box("brick", (OCX, OCY, 0.42), (OX1 - OX0, OY1 - OY0, 0.84))
b.cylinder("brick", (OCX, OCY, 0.84), (OY1 - OY0) / 2, (OY1 - OY0) / 2, OX1 - OX0, segments=24,
           rot=Matrix.Rotation(math.pi / 2, 3, "Y"))
b.box("stone", (OCX, OCY, 0.06), (OX1 - OX0 + 0.08, OY1 - OY0 + 0.08, 0.12), tint=(0.9, 0.9, 0.9))
# oven mouth with fire, arch of upright bricks, iron door hanging open
MOUTH = Vector((OCX, OY0 - 0.01, 0.48))
b.box("interior", MOUTH, (0.42, 0.02, 0.36))
b.sphere("fire", MOUTH + Vector((0, 0.02, -0.08)), 0.16, scale=(1.2, 0.3, 0.75), segments=12)
for dx in (-0.1, 0.0, 0.1):
    b.cylinder("wood_dark", MOUTH + Vector((dx, 0.03, -0.15)), 0.035, 0.035, 0.18, segments=8,
               rot=Matrix.Rotation(math.pi / 2, 3, "X"), tint=(0.4, 0.3, 0.3))
for k in range(11):
    a = math.pi * k / 10
    p = MOUTH + Vector((math.cos(a) * 0.27, -0.03, 0.12 + math.sin(a) * 0.2))
    b.box("brick", p, (0.08, 0.06, 0.13), Matrix.Rotation(-(a - math.pi / 2), 3, "Y"), tint=props._shade(rng, 0.95, 1.25))
for s in (-1, 1):
    b.box("brick", MOUTH + Vector((s * 0.27, -0.03, -0.1)), (0.09, 0.06, 0.3), tint=(1.1, 1.1, 1.1))
b.box("stone", MOUTH + Vector((0, -0.12, -0.2)), (0.6, 0.22, 0.06), tint=(1.05, 1.05, 1.05))
b.box("iron", MOUTH + Vector((0.42, -0.12, 0.0)), (0.03, 0.3, 0.34), Matrix.Rotation(math.radians(-25), 3, "Z"))
b.cylinder("wood_dark", MOUTH + Vector((-0.45, -0.25, 0.2)), 0.015, 0.015, 1.3, segments=6,
           rot=Matrix.Rotation(math.radians(30), 3, "Y"))    # bread peel leaning on the oven
b.box("wood_light", MOUTH + Vector((-0.17, -0.25, -0.38)), (0.2, 0.012, 0.24), Matrix.Rotation(math.radians(30), 3, "Y"))
# oven chimney with the smoke
OCH = Vector((OX1 - 0.25, OY1 - 0.22))
b.box("brick", (OCH.x, OCH.y, 1.9), (0.34, 0.34, 1.8))
b.box("brick", (OCH.x, OCH.y, 2.84), (0.44, 0.44, 0.08), tint=(0.8, 0.8, 0.8))
b.box("interior", (OCH.x, OCH.y, 2.885), (0.22, 0.22, 0.02))
SMOKE_ORIGIN = Vector((OCH.x, OCH.y, 2.95))

# ------------------------------------------------------------- shop front
# striped awning over the shop window
AW_U0, AW_U1 = 0.38 - 0.5, 0.38 + 0.5
strips = 8
for i in range(strips):
    u0 = AW_U0 + (AW_U1 - AW_U0) * i / strips
    u1 = AW_U0 + (AW_U1 - AW_U0) * (i + 1) / strips
    key = "red_cloth" if i % 2 == 0 else "canvas"
    top = Vector(((u0 + u1) / 2, -HY - 0.03, 1.68))
    low = Vector(((u0 + u1) / 2, -HY - 0.5, 1.42))
    d = low - top
    rot = Matrix((Vector((1, 0, 0)), d.normalized(), Vector((1, 0, 0)).cross(d.normalized()))).transposed()
    b.box(key, (top + low) / 2, (u1 - u0 + 0.004, d.length, 0.015), rot)
    b.sphere(key, low + Vector((0, 0, -0.04)), (u1 - u0) / 2, scale=(1, 0.15, 0.7), segments=10)
for s in (-1, 1):
    b.beam("iron", (0.38 + s * 0.5, -HY - 0.02, 1.25), (0.38 + s * 0.5, -HY - 0.5, 1.42), 0.02, 0.02, (1, 0, 0))
# counter with bread
TBL = Vector((0.38, -HY - 0.36, 0))
b.box("wood_light", TBL + Vector((0, 0, 0.62)), (1.0, 0.42, 0.05))
b.box("wood_light", TBL + Vector((0, -0.2, 0.42)), (1.0, 0.03, 0.36), tint=(0.8, 0.8, 0.8))
for sx in (-1, 1):
    for sy in (-1, 1):
        b.box("wood_dark", TBL + Vector((sx * 0.46, sy * 0.17, 0.3)), (0.05, 0.05, 0.6))


def basket(c, r=0.13):
    c = Vector(c)
    b.cylinder("wood_light", c + Vector((0, 0, 0.05)), r * 0.85, r, 0.1, segments=16, tint=(1.2, 1.1, 0.8))
    for k in range(3):
        a = k * 2.1 + rng.uniform(0, 0.5)
        b.sphere("bread", c + Vector((math.cos(a) * r * 0.45, math.sin(a) * r * 0.45, 0.12)), 0.07, scale=(1.3, 0.9, 0.65),
                 segments=12, rot=Matrix.Rotation(a, 3, "Z"), tint=props._shade(rng, 0.85, 1.15))


basket(TBL + Vector((-0.3, 0.02, 0.645)))
basket(TBL + Vector((0.02, -0.02, 0.645)), 0.12)
for k in range(3):                               # loaves straight on the counter
    b.sphere("bread", TBL + Vector((0.28 + k * 0.1, 0.06 - k * 0.04, 0.7)), 0.075, scale=(1.0, 1.0, 0.7), segments=12,
             tint=props._shade(rng, 0.8, 1.1))
for k in range(4):                               # baguettes in a barrel
    tilt = Matrix.Rotation(rng.uniform(-0.25, 0.25), 3, "X") @ Matrix.Rotation(rng.uniform(-0.25, 0.25), 3, "Y")
    b.sphere("bread", Vector((0.98 + rng.uniform(-0.05, 0.05), -HY - 0.3 + rng.uniform(-0.05, 0.05), 0.55)), 0.04,
             scale=(1, 1, 6.5), segments=10, rot=tilt, tint=props._shade(rng, 0.9, 1.15))
props.barrel(b, 0.98, -HY - 0.3, h=0.38, r=0.13)

# pretzel sign over the door
b.beam("iron", (-0.55 - 0.35, -HY - 0.02, 1.72), (-0.55 - 0.35, -HY - 0.45, 1.72), 0.03, 0.03, (1, 0, 0))
SIGN = Vector((-0.9, -HY - 0.4, 1.5))
for t in range(48):
    a = math.tau * t / 48
    # pretzel-ish curve: two loops on top, crossing arms below
    x = 0.12 * math.sin(a) * (1 + 0.35 * math.cos(a))
    z = 0.09 * math.cos(2 * a) + 0.05 * math.cos(a)
    b.sphere("bread", SIGN + Vector((0, x, z)), 0.026, segments=8,
             tint=(1.1, 1.0, 0.9))
b.beam("iron", SIGN + Vector((0, 0, 0.22)), SIGN + Vector((0, 0, 0.1)), 0.012, 0.012, (1, 0, 0))

# ------------------------------------------------------------- yard
GC = Vector((0.45, -0.35, 0))
b.blob_disc("dirt", 2.45, 0.04)
for v in b.bms["dirt"].verts:
    v.co += GC
for _ in range(70):
    a, r = rng.uniform(0, math.tau), rng.uniform(1.3, 2.3)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    b.sphere("pebble", (x, y, 0.0), rng.uniform(0.03, 0.07), scale=(1.2, 1, 0.45), segments=8, tint=props._shade(rng, 0.75, 1.25))
for row in range(5):                             # flagstones to the door
    for i in range(4):
        if rng.random() < 0.1 + row * 0.12:
            continue
        x = -0.78 + i * 0.155 + (0.077 if row % 2 else 0)
        y = -HY - 0.28 - row * 0.15
        s = rng.uniform(0.85, 1.15)
        b.box("pebble", (x, y, 0.012), (0.13 * s, 0.12 * s, 0.04), Matrix.Rotation(rng.uniform(-0.3, 0.3), 3, "Z"),
              tint=props._shade(rng, 0.8, 1.2))
for _ in range(130):
    a, r = rng.uniform(0, math.tau), rng.uniform(1.25, 2.45)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    if -1.0 < x < 1.6 and y < -HY and y > -HY - 1.3:
        continue
    if x > HX and -0.9 < y < 0.5 and x < HX + 1.45:
        continue
    props.tuft(b, x, y, rng.uniform(0.8, 1.4))
    if rng.random() < 0.12:
        props.flowers(b, x + 0.1, y + 0.05, rng.randint(3, 6))
# firewood stack beside the oven
for row, n in enumerate((5, 4, 3)):
    for k in range(n):
        y = 0.45 + 0.13 * k + 0.065 * row + rng.uniform(-0.01, 0.01)
        c = Vector((HX + 1.2, y - 0.25, 0.07 + row * 0.115))
        b.cylinder("wood_light", c, 0.065, 0.065, 0.62, segments=10, rot=Matrix.Rotation(math.pi / 2, 3, "Y"),
                   tint=props._shade(rng, 0.65, 1.0))
props.sack(b, -1.0, -HY - 0.3, 0.0, size=0.15, yaw=0.3, tint=props._shade(rng, 1.6, 1.8))
props.sack(b, -1.22, -HY - 0.12, 0.0, size=0.14, yaw=-0.4, tint=props._shade(rng, 1.6, 1.8))
props.sack(b, -1.1, -HY - 0.2, 0.25, size=0.13, yaw=0.9, tint=props._shade(rng, 1.6, 1.8))
props.crate(b, (HX + 0.45, -1.15, 0.16), 0.32, 0.3)
props.ivy(b, (-HX + 0.15, -HY - 0.08, 0.0), (0, -1, 0), 0.4, 1.0, density=70)

# ------------------------------------------------------------- build, smoke, render
NO_BEVEL = ("dirt", "grass", "burlap", "pebble", "glow", "glow_soft", "leaf", "flower", "grain", "bread", "fire",
            "canvas", "red_cloth")
NO_INK = ("grass", "leaf", "flower", "grain", "glow", "glow_soft", "fire")
building = b.build(bevel=0.012, no_bevel=NO_BEVEL, outline=0.011, no_outline=NO_INK)

scene = bpy.context.scene
bpy.context.preferences.edit.keyframe_new_interpolation_type = "LINEAR"
smoke = lib.add_smoke(SMOKE_ORIGIN, SMOKE_FRAMES, puffs=8, r0=0.12, r1=0.5, density=16)
scene.frame_start, scene.frame_end = 1, SMOKE_FRAMES
scene.render.fps = 12

cam = lib.add_game_camera()
lib.frame_camera(cam, frames=range(1, SMOKE_FRAMES + 1))
scene.frame_set(1)
lib.save_outputs(os.path.join(HERE, "..", "models"), "bakery")

stem = os.path.splitext(OUT)[0]
os.makedirs(os.path.dirname(OUT), exist_ok=True)
lib.render(OUT)                                   # with smoke, as a preview
lib.hide(smoke, True)
lib.render(stem + "_base.png")                    # the building sprite
lib.hide(smoke, False)
lib.set_holdout(building, True)                   # smoke only, masked by the house
frames_dir = stem + "_smoke_frames"
os.makedirs(frames_dir, exist_ok=True)
for i in range(SMOKE_FRAMES):
    scene.frame_set(1 + i)
    lib.render(os.path.join(frames_dir, f"{i:02d}.png"))
