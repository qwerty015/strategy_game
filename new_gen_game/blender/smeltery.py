"""Smeltery (Плавильня): workshop plus a stone furnace stack with dark coal smoke.

    blender -b --factory-startup --python smeltery.py -- <out.png> [resolution] [samples] [smoke_frames]

Outputs mirror bakery.py: smeltery.png (preview with smoke), smeltery_base.png
(game sprite), smeltery_smoke_frames/ (smoke layer, same camera).
"""

import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import bpy  # noqa: E402
from mathutils import Matrix, Vector  # noqa: E402

import house  # noqa: E402
import lib  # noqa: E402
import props  # noqa: E402

args = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []
HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.abspath(args[0] if args else os.path.join(HERE, "..", "renders", "smeltery.png"))
RES = int(args[1]) if len(args) > 1 else 1024
SAMPLES = int(args[2]) if len(args) > 2 else 160
SMOKE_FRAMES = int(args[3]) if len(args) > 3 else 16

lib.clear_scene()
lib.setup_render(RES, SAMPLES)
lib.setup_world()
lib.add_sun()


def emissive(name, color, strength, base=(0.05, 0.02, 0.01)):
    return lib.mat_flat(name, base, rough=0.6, emission=color, emission_strength=strength)


M = {
    # soot-darkened plaster: this is a dirty workplace
    "plaster": lib.mat_noise("plaster", [(0.3, (0.36, 0.30, 0.22)), (0.75, (0.56, 0.48, 0.36))], scale=3, bump=0.2, rough=0.95),
    "wood_dark": lib.mat_wood("wood_dark", (0.05, 0.024, 0.011), (0.14, 0.07, 0.03)),
    "wood_light": lib.mat_wood("wood_light", (0.22, 0.11, 0.045), (0.46, 0.27, 0.11)),
    "stone": lib.mat_stone("stone", (0.20, 0.18, 0.155), (0.33, 0.30, 0.26), (0.06, 0.05, 0.045), scale=2.6),
    "furnace": lib.mat_stone("furnace", (0.17, 0.15, 0.13), (0.30, 0.27, 0.23), (0.05, 0.045, 0.04), scale=3.4),
    "slate": lib.mat_noise(
        "slate",
        [(0.0, (0.018, 0.02, 0.03)), (0.5, (0.04, 0.045, 0.06)), (0.66, (0.045, 0.048, 0.055)), (0.74, (0.07, 0.09, 0.02))],
        scale=2.2, rough=0.55, bump=0.1,
    ),
    "iron": lib.mat_flat("iron", (0.04, 0.04, 0.045), rough=0.45, metallic=0.85),
    "ingot": lib.mat_noise("ingot", [(0.3, (0.18, 0.18, 0.19)), (0.7, (0.32, 0.32, 0.34))], scale=20, rough=0.35, bump=0.1,
                           metallic=0.9),
    "gold": lib.mat_flat("gold", (0.85, 0.55, 0.12), rough=0.3, metallic=1.0),
    "coal": lib.mat_noise("coal", [(0.3, (0.012, 0.012, 0.014)), (0.7, (0.04, 0.04, 0.045))], scale=30, rough=0.45, bump=0.4),
    "ore_iron": lib.mat_noise("ore_iron", [(0.3, (0.16, 0.06, 0.03)), (0.7, (0.32, 0.14, 0.07))], scale=25, rough=0.85, bump=0.4),
    "ore_gold": lib.mat_noise("ore_gold", [(0.3, (0.18, 0.16, 0.13)), (0.62, (0.30, 0.27, 0.22)), (0.72, (0.8, 0.55, 0.1))],
                              scale=30, rough=0.6, bump=0.4),
    "leather": lib.mat_noise("leather", [(0.3, (0.10, 0.05, 0.02)), (0.7, (0.22, 0.11, 0.05))], scale=15, rough=0.7, bump=0.3),
    "molten": emissive("molten", (1.0, 0.45, 0.08), 22, base=(1.0, 0.5, 0.1)),
    "fire": emissive("fire", (1.0, 0.32, 0.05), 16, base=(1.0, 0.35, 0.05)),
    "glow": emissive("glow", (1.0, 0.55, 0.18), 14, base=(1.0, 0.7, 0.3)),
    "glow_soft": emissive("glow_soft", (1.0, 0.45, 0.15), 2.2),
    "dirt": lib.mat_noise("dirt", [(0.3, (0.17, 0.11, 0.05)), (0.72, (0.34, 0.24, 0.12))], scale=3.5, rough=1.0, bump=0.3),
    "pebble": lib.mat_noise("pebble", [(0.3, (0.15, 0.14, 0.12)), (0.7, (0.30, 0.28, 0.24))], scale=12, rough=0.9, bump=0.2),
    "grass": lib.mat_flat("grass", (0.08, 0.21, 0.025), rough=0.8),
    "leaf": lib.mat_noise("leaf", [(0.3, (0.04, 0.12, 0.015)), (0.7, (0.10, 0.26, 0.03))], scale=40, rough=0.6, bump=0.0),
    "flower": lib.mat_flat("flower", (0.9, 0.9, 0.9), rough=0.5),
    "burlap": lib.mat_noise("burlap", [(0.3, (0.36, 0.26, 0.13)), (0.7, (0.55, 0.43, 0.24))], scale=45, rough=1.0, bump=0.35),
    "interior": lib.mat_noise("interior", [(0.3, (0.02, 0.012, 0.007)), (0.7, (0.05, 0.03, 0.016))], scale=8, rough=1.0, bump=0.0),
}
b = lib.Builder(M, seed=77)
rng = b.rng

# ------------------------------------------------------------- workshop
HX, HY = 0.95, 0.82
h = house.House(
    b, HX, HY, top=1.9, ridge="x", pitch_deg=38,
    openings={
        "front": [
            dict(kind="door", u=-0.4, w=0.52, z0=0.0, z1=1.45, open_deg=70),
            dict(kind="window", u=0.42, w=0.42, z0=0.9, z1=1.4, glow=True),
        ],
        "right": [dict(kind="window", u=0.45, w=0.34, z0=1.15, z1=1.6, glow=True)],
    },
)
h.build_all()
# anvil + glow visible through the open door
b.box("glow_soft", (-0.4, -HY + 0.25, 0.6), (0.4, 0.02, 0.6))

# ------------------------------------------------------------- furnace stack
FX, FY = HX + 0.85, -0.15
F_BASE, F_TOP, F_H = 0.62, 0.36, 2.55       # half sizes of the square frustum
sq = Matrix.Rotation(math.pi / 4, 3, "Z")
b.cylinder("furnace", (FX, FY, F_H / 2), F_BASE * math.sqrt(2), F_TOP * math.sqrt(2), F_H, segments=4, rot=sq)
# heavy plinth and iron bands
b.cylinder("stone", (FX, FY, 0.14), (F_BASE + 0.1) * math.sqrt(2), (F_BASE + 0.08) * math.sqrt(2), 0.28, segments=4, rot=sq)
for z in (0.75, 1.45, 2.1):
    r = F_BASE + (F_TOP - F_BASE) * z / F_H + 0.025
    b.cylinder("iron", (FX, FY, z), r * math.sqrt(2), (r - 0.004) * math.sqrt(2), 0.06, segments=4, rot=sq)
# crown with a glowing throat
b.cylinder("stone", (FX, FY, F_H + 0.06), (F_TOP + 0.08) * math.sqrt(2), (F_TOP + 0.08) * math.sqrt(2), 0.12, segments=4, rot=sq)
b.box("fire", (FX, FY, F_H + 0.125), (2 * F_TOP - 0.12, 2 * F_TOP - 0.12, 0.01))
# tap arch on the front face
TAP = Vector((FX, FY - F_BASE + 0.02, 0.5))
b.box("interior", TAP, (0.34, 0.06, 0.4))
b.sphere("molten", TAP + Vector((0, -0.02, -0.12)), 0.13, scale=(1.1, 0.3, 0.55), segments=12)
for k in range(9):
    a = math.pi * k / 8
    p = TAP + Vector((math.cos(a) * 0.24, -0.05, 0.1 + math.sin(a) * 0.18))
    b.box("stone", p, (0.09, 0.08, 0.13), Matrix.Rotation(-(a - math.pi / 2), 3, "Y"), tint=props._shade(rng, 1.0, 1.3))
# channel of molten metal running out to two ingot moulds
CH0 = Vector((FX, FY - F_BASE - 0.1, 0.3))
CH1 = Vector((FX - 0.15, FY - F_BASE - 0.9, 0.06))
b.beam("stone", CH0 + Vector((0, 0, -0.05)), CH1 + Vector((0, 0, -0.05)), 0.12, 0.2, (0, 0, 1), tint=(0.8, 0.8, 0.8))
b.beam("molten", CH0 + Vector((0, 0, 0.01)), CH1 + Vector((0, 0, 0.01)), 0.02, 0.08, (0, 0, 1))
b.beam("stone", TAP + Vector((0, -0.08, -0.2)), CH0 + Vector((0, 0, -0.05)), 0.12, 0.2, (0, 0, 1), tint=(0.8, 0.8, 0.8))
b.beam("molten", TAP + Vector((0, -0.08, -0.14)), CH0 + Vector((0, 0, 0.01)), 0.02, 0.08, (0, 0, 1))
for dx in (-0.2, 0.12):
    c = CH1 + Vector((dx, -0.15, -0.02))
    b.box("stone", c, (0.28, 0.16, 0.08), tint=(0.7, 0.7, 0.7))
    b.box("molten", c + Vector((0, 0, 0.041)), (0.22, 0.1, 0.005))

# bellows on the right face, nozzle into the furnace
BEL = Vector((FX + F_BASE + 0.42, FY + 0.05, 0.42))
bel_rot = Matrix.Rotation(math.radians(8), 3, "Y")
b.box("wood_light", BEL + Vector((0, 0, 0.12)), (0.55, 0.36, 0.04), bel_rot)
b.box("wood_light", BEL - Vector((0, 0, 0.1)), (0.55, 0.36, 0.04))
b.prism("leather", [BEL + Vector((0.27, -0.17, -0.08)), BEL + Vector((0.27, 0.17, -0.08)), BEL + Vector((-0.27, 0.15, -0.08)),
                    BEL + Vector((-0.27, -0.15, -0.08))], (0, 0, 0.18))
b.beam("iron", BEL + Vector((-0.27, 0, 0.0)), Vector((FX + F_BASE - 0.05, FY + 0.05, 0.5)), 0.05, 0.05, (0, 1, 0))
b.beam("wood_dark", BEL + Vector((0.27, 0, 0.14)), BEL + Vector((0.7, 0, 0.55)), 0.04, 0.04, (0, 1, 0))
for s in (-1, 1):
    b.box("wood_dark", BEL + Vector((0.1, s * 0.16, -0.32)), (0.05, 0.05, 0.38))

# ------------------------------------------------------------- stock piles


def pile(key, center, radius, count, height, size=(0.05, 0.09)):
    c = Vector(center)
    for _ in range(count):
        a, r = rng.uniform(0, math.tau), radius * math.sqrt(rng.random())
        z = height * (1 - (r / radius) ** 1.3) * rng.uniform(0.6, 1.0)
        s = rng.uniform(*size)
        b.sphere(key, c + Vector((math.cos(a) * r, math.sin(a) * r, z)), s,
                 scale=(rng.uniform(0.8, 1.3), rng.uniform(0.8, 1.2), rng.uniform(0.6, 0.9)), segments=6,
                 rot=Matrix.Rotation(rng.uniform(0, math.tau), 3, "Z"), tint=props._shade(rng, 0.8, 1.25))


pile("coal", (FX + 0.25, FY + F_BASE + 0.6, 0), 0.5, 140, 0.45)
pile("ore_iron", (-1.2, -HY - 0.75, 0), 0.32, 60, 0.28)
pile("ore_gold", (-0.65, -HY - 1.05, 0), 0.26, 40, 0.22)
# ingots stacked crosswise
STK = Vector((0.55, -HY - 0.75, 0))
for layer in range(4):
    for k in range(3):
        along = layer % 2
        off = (k - 1) * 0.1
        p = STK + (Vector((0, off, 0)) if along == 0 else Vector((off, 0, 0))) + Vector((0, 0, 0.03 + layer * 0.055))
        b.box("ingot", p, (0.26, 0.085, 0.05) if along == 0 else (0.085, 0.26, 0.05), tint=props._shade(rng, 0.85, 1.15))
for k in range(3):
    b.box("gold", STK + Vector((0.42, -0.05 + k * 0.1, 0.03)), (0.22, 0.075, 0.05))
b.box("gold", STK + Vector((0.42, 0.0, 0.08)), (0.22, 0.075, 0.05))
props.barrel(b, -1.25, -HY - 0.2, h=0.4, r=0.14)
props.crate(b, (1.25, -HY - 1.15, 0.15), 0.3, 0.4)
# a wheelbarrow-ish cart of coal
props.cart(b, (-0.2, -HY - 1.55, 0), yaw=math.radians(160), load=False)
pile("coal", (-0.2, -HY - 1.55, 0.45), 0.24, 30, 0.12, size=(0.04, 0.07))

# ------------------------------------------------------------- yard
GC = Vector((0.5, -0.45, 0))
b.blob_disc("dirt", 2.7, 0.04)
for v in b.bms["dirt"].verts:
    v.co += GC
for _ in range(90):
    a, r = rng.uniform(0, math.tau), rng.uniform(1.0, 2.6)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    b.sphere("coal" if rng.random() < 0.35 else "pebble", (x, y, 0.0), rng.uniform(0.025, 0.06), scale=(1.2, 1, 0.45), segments=6,
             tint=props._shade(rng, 0.75, 1.25))
for _ in range(80):                       # sparse, trampled grass at a smelter
    a, r = rng.uniform(0, math.tau), rng.uniform(1.9, 2.7)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    props.tuft(b, x, y, rng.uniform(0.7, 1.2))

# ------------------------------------------------------------- build, smoke, render
NO_BEVEL = ("dirt", "grass", "burlap", "pebble", "glow", "glow_soft", "leaf", "flower", "coal", "ore_iron", "ore_gold",
            "molten", "fire", "leather")
NO_INK = ("grass", "leaf", "flower", "glow", "glow_soft", "molten", "fire", "coal", "ore_iron", "ore_gold")
building = b.build(bevel=0.012, no_bevel=NO_BEVEL, outline=0.011, no_outline=NO_INK)

scene = bpy.context.scene
bpy.context.preferences.edit.keyframe_new_interpolation_type = "LINEAR"
smoke = lib.add_smoke((FX, FY, F_H + 0.2), SMOKE_FRAMES, puffs=9, rise=1.9, drift=(0.55, 0.25), r0=0.18, r1=0.62,
                      density=14, color=(0.32, 0.31, 0.3))
scene.frame_start, scene.frame_end = 1, SMOKE_FRAMES
scene.render.fps = 12
cam = lib.add_game_camera()
lib.frame_camera(cam, frames=range(1, SMOKE_FRAMES + 1))
scene.frame_set(1)
lib.save_outputs(os.path.join(HERE, "..", "models"), "smeltery")

stem = os.path.splitext(OUT)[0]
os.makedirs(os.path.dirname(OUT), exist_ok=True)
lib.render(OUT)
lib.hide(smoke, True)
lib.render(stem + "_base.png")
lib.hide(smoke, False)
lib.set_holdout(building, True)
frames_dir = stem + "_smoke_frames"
os.makedirs(frames_dir, exist_ok=True)
for i in range(SMOKE_FRAMES):
    scene.frame_set(1 + i)
    lib.render(os.path.join(frames_dir, f"{i:02d}.png"))
