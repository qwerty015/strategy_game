"""Mill (Мельница): stone tower mill with a shingled cap and four canvas sails.

    blender -b --factory-startup --python mill.py -- <out.png> [resolution] [samples] [frames]

The sails hang from an empty "spin" that is keyframed 0->360 degrees over
96 frames, so the saved .blend plays the rotation in Blender (Space). The
sprite sheet uses `frames` evenly spaced poses over a quarter turn -- the
four sails are identical, so a quarter turn already loops seamlessly.
Output: <out.png> (pose 0), <out>_sheet.png (all poses in a row) via
sheet.py, and <out>_frames/NN.png.
"""

import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import bpy  # noqa: E402
from mathutils import Matrix, Vector  # noqa: E402

import lib  # noqa: E402
import props  # noqa: E402

args = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []
HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.abspath(args[0] if args else os.path.join(HERE, "..", "renders", "mill.png"))
RES = int(args[1]) if len(args) > 1 else 1024
SAMPLES = int(args[2]) if len(args) > 2 else 192
POSES = int(args[3]) if len(args) > 3 else 8
CYCLE = 96                       # frames per full turn in the .blend

lib.clear_scene()
lib.setup_render(RES, SAMPLES)
lib.setup_world()
lib.add_sun()

M = {
    "block": lib.mat_noise("block", [(0.3, (0.22, 0.19, 0.155)), (0.72, (0.40, 0.36, 0.30))], scale=7, rough=0.9, bump=0.35),
    "mortar": lib.mat_noise("mortar", [(0.3, (0.07, 0.06, 0.05)), (0.7, (0.12, 0.10, 0.08))], scale=10, rough=1.0, bump=0.1),
    "plaster": lib.mat_noise("plaster", [(0.3, (0.56, 0.44, 0.27)), (0.75, (0.76, 0.63, 0.42))], scale=3, bump=0.18, rough=0.95),
    "wood_dark": lib.mat_wood("wood_dark", (0.06, 0.028, 0.012), (0.17, 0.08, 0.032)),
    "wood_light": lib.mat_wood("wood_light", (0.24, 0.12, 0.045), (0.50, 0.29, 0.11)),
    "slate": lib.mat_noise(
        "slate",
        [(0.0, (0.02, 0.024, 0.04)), (0.5, (0.045, 0.052, 0.08)), (0.66, (0.05, 0.056, 0.07)), (0.74, (0.08, 0.11, 0.02))],
        scale=2.2, rough=0.55, bump=0.1,
    ),
    "canvas": lib.mat_noise("canvas", [(0.3, (0.42, 0.27, 0.12)), (0.7, (0.62, 0.44, 0.22))], scale=25, rough=0.95, bump=0.15),
    "iron": lib.mat_flat("iron", (0.04, 0.04, 0.045), rough=0.45, metallic=0.85),
    "dirt": lib.mat_noise("dirt", [(0.3, (0.22, 0.13, 0.05)), (0.72, (0.42, 0.28, 0.12))], scale=3.5, rough=1.0, bump=0.3),
    "pebble": lib.mat_noise("pebble", [(0.3, (0.17, 0.155, 0.13)), (0.7, (0.33, 0.30, 0.25))], scale=12, rough=0.9, bump=0.2),
    "grass": lib.mat_flat("grass", (0.09, 0.24, 0.025), rough=0.8),
    "leaf": lib.mat_noise("leaf", [(0.3, (0.04, 0.12, 0.015)), (0.7, (0.10, 0.26, 0.03))], scale=40, rough=0.6, bump=0.0),
    "flower": lib.mat_flat("flower", (0.9, 0.9, 0.9), rough=0.5),
    "burlap": lib.mat_noise("burlap", [(0.3, (0.36, 0.26, 0.13)), (0.7, (0.55, 0.43, 0.24))], scale=45, rough=1.0, bump=0.35),
    "grain": lib.mat_noise("grain", [(0.35, (0.45, 0.28, 0.06)), (0.65, (0.75, 0.52, 0.14))], scale=70, rough=0.8, bump=0.3),
    "glow": lib.mat_flat("glow", (1.0, 0.7, 0.3), emission=(1.0, 0.55, 0.18), emission_strength=14),
    "interior": lib.mat_noise("interior", [(0.3, (0.02, 0.012, 0.007)), (0.7, (0.05, 0.03, 0.016))], scale=8, rough=1.0, bump=0.0),
    "gold": lib.mat_flat("gold", (0.85, 0.55, 0.12), rough=0.35, metallic=1.0),
}
b = lib.Builder(M, seed=5)
rng = b.rng
FLOUR = (1.75, 1.72, 1.62)

# ------------------------------------------------------------- dimensions
BASE_R, BASE_H = 0.98, 1.25      # straight lower drum (holds the door)
TAPER_R0, TAPER_R1 = 0.9, 0.66   # tapered upper tower
TOWER_TOP = 2.75
CAP0, CAP_R, CAP_H = TOWER_TOP + 0.08, 0.88, 0.95
FACE = math.radians(-30)          # sails/shaft face this way (camera sits at -45)
D = Vector((math.cos(FACE), math.sin(FACE), 0))
TILT = math.radians(6)
AXIS = Vector((D.x * math.cos(TILT), D.y * math.cos(TILT), math.sin(TILT)))
HUB = Vector((0, 0, CAP0 + 0.28)) + AXIS * 1.28
SAIL_LEN = 1.85
DOOR_ANG = math.radians(-50)      # door faces the camera


def radius_at(z):
    if z <= BASE_H:
        return BASE_R
    t = (z - BASE_H) / (TOWER_TOP - BASE_H)
    return TAPER_R0 + (TAPER_R1 - TAPER_R0) * t


def ring_frame(theta, slope=0.0):
    """Local frame on the tower surface: x = tangent, y = up the wall, z = outward."""
    n = Vector((math.cos(theta), math.sin(theta), 0))
    t = Vector((-math.sin(theta), math.cos(theta), 0))
    up = Vector((0, 0, 1))
    if slope:
        rot = Matrix.Rotation(slope, 3, t)
        n, up = rot @ n, rot @ up
    return Matrix((t, up, n)).transposed(), n


def openings_hit(theta, z):
    """Skip wall stones where the door or windows are."""
    def near(a, w):
        return abs((theta - a + math.pi) % math.tau - math.pi) < w
    if z < 1.08 and near(DOOR_ANG, 0.33):
        return True
    for a, z0, z1, w in WINDOWS:
        if z0 - 0.06 < z < z1 + 0.06 and near(a, w):
            return True
    return False


WINDOWS = [(math.radians(-10), 1.62, 1.98, 0.17), (math.radians(-85), 2.18, 2.5, 0.17)]

# ------------------------------------------------------------- ground
GC = Vector((0.15, -0.25, 0))
b.blob_disc("dirt", 2.45, 0.04)
for v in b.bms["dirt"].verts:
    v.co += GC
for _ in range(80):
    a, r = rng.uniform(0, math.tau), rng.uniform(1.2, 2.3)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    b.sphere("pebble", (x, y, 0.0), rng.uniform(0.03, 0.07), scale=(1.2, 1, 0.45), segments=8, tint=props._shade(rng, 0.75, 1.25))
# path of flagstones from the door outward
door_n = Vector((math.cos(DOOR_ANG), math.sin(DOOR_ANG), 0))
door_t = Vector((-door_n.y, door_n.x, 0))
for row in range(6):
    for i in range(-2, 3):
        if rng.random() < 0.1 + row * 0.1:
            continue
        p = door_n * (BASE_R + 0.2 + row * 0.17) + door_t * (i * 0.16 + (0.08 if row % 2 else 0))
        s = rng.uniform(0.85, 1.15)
        b.box("pebble", (p.x, p.y, 0.012), (0.14 * s, 0.13 * s, 0.04), Matrix.Rotation(DOOR_ANG + rng.uniform(-0.3, 0.3), 3, "Z"),
              tint=props._shade(rng, 0.8, 1.2))
for _ in range(130):
    a, r = rng.uniform(0, math.tau), rng.uniform(1.2, 2.45)
    x, y = GC.x + math.cos(a) * r, GC.y + math.sin(a) * r
    if abs((a - DOOR_ANG + math.pi) % math.tau - math.pi) < 0.35 and r < 2.0:
        continue
    props.tuft(b, x, y, rng.uniform(0.8, 1.4))
    if rng.random() < 0.12:
        props.flowers(b, x + 0.1, y + 0.05, rng.randint(3, 6))
for k in range(14):                      # grass hugging the foot of the tower
    a = rng.uniform(0, math.tau)
    if abs((a - DOOR_ANG + math.pi) % math.tau - math.pi) < 0.4:
        continue
    props.tuft(b, math.cos(a) * (BASE_R + 0.08), math.sin(a) * (BASE_R + 0.08), 0.8)

# ------------------------------------------------------------- stone tower
b.cylinder("mortar", (0, 0, BASE_H / 2), BASE_R - 0.05, BASE_R - 0.05, BASE_H, segments=32)
b.cylinder("mortar", (0, 0, (BASE_H + TOWER_TOP) / 2), TAPER_R0 - 0.05, TAPER_R1 - 0.05, TOWER_TOP - BASE_H, segments=32)
z, row = 0.0, 0
slope = math.atan((TAPER_R0 - TAPER_R1) / (TOWER_TOP - BASE_H))
while z < TOWER_TOP - 0.05:
    h = 0.17 if z < BASE_H else 0.15
    zc = z + h / 2
    r = radius_at(zc)
    count = max(10, int(math.tau * r / 0.3))
    off = (math.pi / count) * (row % 2)
    for k in range(count):
        theta = off + math.tau * k / count + rng.uniform(-0.03, 0.03)
        if openings_hit(theta, zc):
            continue
        frame, n = ring_frame(theta, slope if zc > BASE_H else 0.0)
        w = math.tau * r / count - 0.025 + rng.uniform(-0.03, 0.02)
        c = Vector((0, 0, zc)) + n * (r - 0.04 + rng.uniform(0, 0.018))
        b.box("block", c, (w, h - 0.02, 0.12), frame, tint=props._shade(rng, 0.78, 1.22))
    z += h
    row += 1
    if z < BASE_H <= z + h:
        z = BASE_H
# ledge between drum and taper
for k in range(28):
    theta = math.tau * k / 28
    frame, n = ring_frame(theta)
    b.box("block", Vector((0, 0, BASE_H + 0.03)) + n * (BASE_R + 0.01), (math.tau * (BASE_R + 0.05) / 28 - 0.015, 0.08, 0.16), frame,
          tint=props._shade(rng, 0.95, 1.2))

# door: arched stone surround, plank door, threshold
frame, n = ring_frame(DOOR_ANG)
tdir = frame.col[0]
door_c = n * (BASE_R - 0.02)
b.box("interior", door_c + Vector((0, 0, 0.5)), (0.52, 1.0, 0.04), frame)
props.door_leaf(b, door_c + n * 0.03 - tdir * 0.24 + Vector((0, 0, 0.02)), (tdir.x, tdir.y), (n.x, n.y), 0.48, 0.86, planks=4)
for k in range(9):                       # voussoirs
    a = math.pi * k / 8
    p = door_c + n * 0.06 + tdir * (math.cos(a) * 0.32) + Vector((0, 0, 0.88 + math.sin(a) * 0.2))
    rot = frame @ Matrix.Rotation(a - math.pi / 2, 3, "Z")
    b.box("block", p, (0.12, 0.17, 0.14), rot, tint=props._shade(rng, 1.0, 1.3))
for s in (-1, 1):
    for k in range(4):
        b.box("block", door_c + n * 0.06 + tdir * (s * 0.32) + Vector((0, 0, 0.12 + k * 0.2)), (0.14, 0.18, 0.14), frame,
              tint=props._shade(rng, 1.0, 1.3))
b.box("block", door_c + n * 0.2 + Vector((0, 0, 0.04)), (0.7, 0.3, 0.08), frame @ Matrix.Rotation(math.pi / 2, 3, "X"),
      tint=(1.1, 1.1, 1.1))
b.box("wood_dark", door_c + n * 0.08 + Vector((0, 0, 1.15)), (0.85, 0.06, 0.06), frame)   # sign rail
b.box("wood_light", door_c + n * 0.1 + Vector((0, 0, 1.03)) + tdir * 0.22, (0.3, 0.18, 0.03), frame)
b.sphere("grain", door_c + n * 0.12 + Vector((0, 0, 1.03)) + tdir * 0.22, 0.05, scale=(1, 1.4, 0.4), segments=8,
         rot=frame)
props.lantern(b, door_c + n * 0.02 - tdir * 0.42 + Vector((0, 0, 0.85)), n, out=0.16)

# windows with little shutters
for a, z0, z1, _w in WINDOWS:
    fr, n = ring_frame(a, slope)
    r = radius_at((z0 + z1) / 2)
    c = n * (r - 0.02) + Vector((0, 0, (z0 + z1) / 2))
    b.box("interior", c, (0.24, z1 - z0, 0.03), fr)
    for s in (-1, 1):
        b.box("wood_dark", c + fr.col[0] * (s * 0.14) + n * 0.03, (0.05, z1 - z0 + 0.08, 0.05), fr)
    for zz in (z0, z1):
        b.box("wood_dark", n * (r + 0.01) + Vector((0, 0, zz)), (0.34, 0.05, 0.06), fr)
    b.box("block", n * (r + 0.04) + Vector((0, 0, z0 - 0.05)), (0.38, 0.06, 0.12), fr, tint=(1.1, 1.1, 1.1))
    for s in (-1, 1):
        hinge = c + fr.col[0] * (s * 0.14) + n * 0.05
        ang = math.radians(70) * s
        along = Matrix.Rotation(ang, 3, "Z") @ (fr.col[0] * s)
        out = Matrix.Rotation(ang, 3, "Z") @ n
        props.door_leaf(b, hinge - Vector((0, 0, (z1 - z0) / 2 - 0.02)), (along.x, along.y), (out.x, out.y), 0.13, z1 - z0 - 0.04, planks=2)

# ------------------------------------------------------------- cap
b.cylinder("wood_dark", (0, 0, TOWER_TOP + 0.04), TAPER_R1 + 0.06, TAPER_R1 + 0.06, 0.08, segments=32)
b.cylinder("wood_dark", (0, 0, CAP0 + 0.05), CAP_R + 0.02, CAP_R + 0.02, 0.1, segments=32)
for k in range(16):                      # rafter ends under the eave
    a = math.tau * k / 16
    p = Vector((math.cos(a), math.sin(a), 0)) * (CAP_R - 0.02) + Vector((0, 0, CAP0 + 0.0))
    b.box("wood_dark", p, (0.05, 0.05, 0.08), Matrix.Rotation(a, 3, "Z"))
slant = math.hypot(CAP_R, CAP_H)
b.cylinder("wood_dark", (0, 0, CAP0 + 0.1 + CAP_H / 2), CAP_R - 0.04, 0.02, CAP_H, segments=32)
t, row = 0.0, 0
while t < slant - 0.12:
    rr = (CAP_R + 0.06) * (1 - t / slant)
    zz = CAP0 + 0.12 + CAP_H * t / slant
    count = max(6, int(math.tau * rr / 0.15))
    for k in range(count):
        th = math.tau * (k + 0.5 * (row % 2)) / count
        tan = Vector((-math.sin(th), math.cos(th), 0))
        up = Vector((-CAP_R * math.cos(th), -CAP_R * math.sin(th), CAP_H)) / slant
        nn = tan.cross(up)
        fr = Matrix((tan, up, nn)).transposed() @ Matrix.Rotation(math.radians(-7), 3, "X")
        c = Vector((rr * math.cos(th), rr * math.sin(th), zz)) + up * 0.085 + nn * 0.04
        g = rng.uniform(0.68, 1.28)
        b.box("slate", c, (math.tau * rr / count * 1.06, 0.18, 0.03), fr, tint=(g, g, g * rng.uniform(0.95, 1.08)))
    t += 0.1
    row += 1
b.cylinder("slate", (0, 0, CAP0 + 0.12 + CAP_H + 0.02), 0.08, 0.0, 0.16, segments=8)
b.cylinder("iron", (0, 0, CAP0 + CAP_H + 0.4), 0.012, 0.012, 0.32, segments=6)
b.sphere("gold", (0, 0, CAP0 + CAP_H + 0.3), 0.045, segments=10)
# windshaft
shaft_from = Vector((0, 0, CAP0 + 0.28)) + AXIS * 0.3
b.beam("wood_dark", shaft_from, HUB - AXIS * 0.12, 0.13, 0.13, Vector((0, 0, 1)))
b.cylinder("iron", HUB - AXIS * 0.4, 0.09, 0.09, 0.05, segments=16, rot=AXIS.to_track_quat("Z", "Y").to_matrix())

# tail pole to turn the cap into the wind
back = -D
tail_top = back * (CAP_R - 0.1) + Vector((0, 0, CAP0 + 0.1))
tail_foot = back * 2.0 + Vector((0, 0, 0.18))
b.beam("wood_dark", tail_top, tail_foot, 0.1, 0.1, Vector((-back.y, back.x, 0)))
b.beam("wood_dark", back * (CAP_R - 0.1) + Vector((0, 0, CAP0 - 0.35)), tail_foot.lerp(tail_top, 0.45), 0.07, 0.07,
       Vector((-back.y, back.x, 0)))
b.cylinder("wood_light", tail_foot, 0.17, 0.17, 0.05, segments=16,
           rot=Vector((-back.y, back.x, 0)).to_track_quat("Z", "Y").to_matrix())

# ------------------------------------------------------------- yard
side = Vector((-door_n.y, door_n.x, 0))
base = door_n * (BASE_R + 0.35)
for i, (dt, dn, z0) in enumerate(((0.55, 0.0, 0.0), (0.78, -0.12, 0.0), (0.66, 0.15, 0.0), (0.67, -0.02, 0.25), (0.98, 0.1, 0.0))):
    p = base + side * dt + door_n * dn
    props.sack(b, p.x, p.y, z0, size=0.15, yaw=rng.uniform(-0.6, 0.6), tint=props._shade(rng, 1.6, 1.8))
for dt, dn in ((-0.7, 0.05), (-0.88, -0.2), (-1.02, 0.12), (-0.78, 0.3)):
    p = base + side * dt + door_n * dn
    props.sheaf(b, p.x, p.y, height=rng.uniform(0.45, 0.55), lean=rng.uniform(-0.15, 0.15), yaw=rng.uniform(0, math.tau))
# spare millstone leaning on the tower
ms_dir = Vector((math.cos(math.radians(30)), math.sin(math.radians(30)), 0))
ms_c = ms_dir * (BASE_R + 0.12) + Vector((0, 0, 0.33))
ms_rot = ms_dir.to_track_quat("Z", "Y").to_matrix() @ Matrix.Rotation(math.radians(-12), 3, "X")
b.cylinder("block", ms_c, 0.33, 0.33, 0.1, segments=28, rot=ms_rot, tint=(1.15, 1.12, 1.08))
b.cylinder("mortar", ms_c + ms_dir * 0.052, 0.06, 0.06, 0.012, segments=16, rot=ms_rot)
props.cart(b, tuple(door_n * 1.95 + side * 0.95), yaw=math.atan2(-side.y, -side.x) + 0.4)
props.barrel(b, *(door_n * (BASE_R + 0.25) - side * 0.45)[:2])
props.fence(b, tuple(door_n * 0.6 + side * -1.55), tuple(door_n * 1.9 + side * -1.45))

# ------------------------------------------------------------- sails (separate rotating rig)
rig = bpy.data.objects.new("sail_axis", None)
bpy.context.scene.collection.objects.link(rig)
rig.location = HUB
rig.rotation_mode = "QUATERNION"
rig.rotation_quaternion = AXIS.to_track_quat("Z", "Y")
spin = bpy.data.objects.new("sail_spin", None)
bpy.context.scene.collection.objects.link(spin)
spin.parent = rig
spin.empty_display_size = 0.3

bs = lib.Builder(M, seed=9)
bs.cylinder("wood_dark", (0, 0, -0.02), 0.16, 0.16, 0.24, segments=16)
bs.cylinder("iron", (0, 0, 0.11), 0.1, 0.06, 0.06, segments=16)
for k in range(4):
    phi = math.radians(20) + k * math.pi / 2
    e = Vector((math.cos(phi), math.sin(phi), 0))
    sd = Vector((-math.sin(phi), math.cos(phi), 0))
    bs.beam("wood_dark", e * 0.05 + Vector((0, 0, 0.05)), e * SAIL_LEN + Vector((0, 0, 0.05)), 0.08, 0.08, Vector((0, 0, 1)))
    r0, r1, w = 0.38, SAIL_LEN - 0.04, 0.46
    for off in (0.0, w):
        bs.beam("wood_light", e * r0 + sd * off + Vector((0, 0, 0.1)), e * r1 + sd * off + Vector((0, 0, 0.1)), 0.035, 0.035,
                Vector((0, 0, 1)), tint=(0.85, 0.85, 0.85))
    rr = r0
    while rr <= r1 + 0.001:
        bs.beam("wood_light", e * rr - sd * 0.06 + Vector((0, 0, 0.11)), e * rr + sd * (w + 0.02) + Vector((0, 0, 0.11)), 0.03,
                0.025, Vector((0, 0, 1)), tint=(0.8, 0.8, 0.8))
        rr += (r1 - r0) / 11
    # canvas, slightly reefed on two of the sails so the rotation reads clearly
    cw = w - 0.04 if k % 2 == 0 else w * 0.62
    c = e * ((r0 + r1) / 2) + sd * (0.02 + cw / 2) + Vector((0, 0, 0.075))
    bs.box("canvas", c, (r1 - r0 - 0.03, cw, 0.012), Matrix.Rotation(phi, 3, "Z"), tint=props._shade(rng, 0.9, 1.05))

# ------------------------------------------------------------- build, rig, animate
NO_BEVEL = ("dirt", "grass", "burlap", "pebble", "glow", "leaf", "flower", "grain", "canvas")
NO_INK = ("grass", "leaf", "flower", "grain", "glow")
b.build(bevel=0.012, no_bevel=NO_BEVEL, outline=0.011, no_outline=NO_INK)
for obj in bs.build(bevel=0.008, no_bevel=NO_BEVEL, outline=0.009, no_outline=NO_INK):
    obj.name = "sail_" + obj.name
    obj.parent = spin

scene = bpy.context.scene
scene.frame_start, scene.frame_end = 1, CYCLE
scene.render.fps = 24
bpy.context.preferences.edit.keyframe_new_interpolation_type = "LINEAR"
spin.rotation_euler = (0, 0, 0)
spin.keyframe_insert("rotation_euler", index=2, frame=1)
spin.rotation_euler = (0, 0, -math.tau)          # clockwise seen from the front
spin.keyframe_insert("rotation_euler", index=2, frame=CYCLE + 1)

quarter = CYCLE // 4
pose_frames = [1 + round(i * quarter / POSES) for i in range(POSES)]
cam = lib.add_game_camera()
lib.frame_camera(cam, frames=pose_frames)
scene.frame_set(1)
os.makedirs(os.path.dirname(OUT), exist_ok=True)
lib.save_outputs(os.path.join(HERE, "..", "models"), "mill")

stem = os.path.splitext(OUT)[0]
frames_dir = stem + "_frames"
os.makedirs(frames_dir, exist_ok=True)
for i, f in enumerate(pose_frames):
    scene.frame_set(f)
    lib.render(os.path.join(frames_dir, f"{i:02d}.png"))
    if i == 0:
        import shutil
        shutil.copyfile(os.path.join(frames_dir, "00.png"), OUT)
