"""Stitch rendered animation frames into a sprite sheet and a GIF preview.

    python new_gen_game/sheet.py new_gen_game/renders/mill_frames [fps] [base.png]

base.png (optional) is drawn under every frame in the GIF -- for overlay
animations such as chimney smoke rendered separately from the building.

Writes <dir>.png -> <name>_sheet.png (frames in one row, same cell size)
and <name>_preview.gif (looping, on a grass-coloured background).
"""
import glob
import os
import sys

from PIL import Image

frames_dir = sys.argv[1].rstrip("/\\")
fps = int(sys.argv[2]) if len(sys.argv) > 2 else 10
base = Image.open(sys.argv[3]).convert("RGBA") if len(sys.argv) > 3 else None
paths = sorted(glob.glob(os.path.join(frames_dir, "*.png")))
frames = [Image.open(p).convert("RGBA") for p in paths]
w, h = frames[0].size
stem = frames_dir[: -len("_frames")] if frames_dir.endswith("_frames") else frames_dir

sheet = Image.new("RGBA", (w * len(frames), h))
for i, f in enumerate(frames):
    sheet.paste(f, (i * w, 0))
sheet.save(stem + "_sheet.png")

gif = []
for f in frames:
    bg = Image.new("RGBA", (w, h), (58, 92, 52, 255))
    if base is not None:
        bg.alpha_composite(base)
    bg.alpha_composite(f)
    gif.append(bg.convert("RGB").resize((w // 2, h // 2), Image.LANCZOS))
gif[0].save(stem + "_preview.gif", save_all=True, append_images=gif[1:], duration=1000 // fps, loop=0)
print("wrote", stem + "_sheet.png", "and", stem + "_preview.gif", f"({len(frames)} frames)")
