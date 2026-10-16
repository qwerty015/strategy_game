"""Render the in-game variant of a saved building scene.

    blender -b new_gen_game/models/<name>.blend --python new_gen_game/blender/game_pass.py -- <out_dir> [samples]

The game draws its own grass and roads, so the sprite must not carry a big
yard: the dirt patch becomes a shadow catcher (only the shadows stay), grass,
flowers and pebbles are hidden, smoke is hidden (it ships as its own layer).
Same camera as the preview renders, so smoke frames line up 1:1.

Writes <out_dir>/body.png, or body_NN.png for a rig with "sail_spin"
(one per pose, same poses as mill.py).
"""

import os
import sys

import bpy

args = sys.argv[sys.argv.index("--") + 1:]
OUT_DIR = os.path.abspath(args[0])
SAMPLES = int(args[1]) if len(args) > 1 else 160
os.makedirs(OUT_DIR, exist_ok=True)

scene = bpy.context.scene
scene.cycles.samples = SAMPLES
HIDE = ("grass", "flower", "pebble", "smoke_")
for obj in scene.objects:
    base = obj.name.split(".")[0]
    obj.is_holdout = False
    if base == "dirt":
        obj.is_shadow_catcher = True
    elif base.startswith(HIDE):
        obj.hide_render = True


def render(path):
    scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    print("[game_pass] wrote", path)


if "sail_spin" in bpy.data.objects:
    cycle = scene.frame_end
    poses = 8
    quarter = cycle // 4
    for i in range(poses):
        scene.frame_set(1 + round(i * quarter / poses))
        render(os.path.join(OUT_DIR, f"body_{i:02d}.png"))
else:
    scene.frame_set(1)
    render(os.path.join(OUT_DIR, "body.png"))
