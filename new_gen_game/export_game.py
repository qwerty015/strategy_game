"""Crop game-pass renders and install them as the game's building art.

    python new_gen_game/export_game.py <slug> <game_pass_dir> [smoke_frames_dir]

Writes internal/assets/buildings/<slug>/:
  body.png or body_NN.png   tightly cropped (all body frames share one box)
  smoke_NN.png              optional overlay frames

The game anchors a standing sprite at the bottom-centre of its tile and
scales it by height, so every overlay is cropped with the SAME bottom edge
and the SAME horizontal centre as the body. The Go side then derives the
overlay's height in tiles from the pixel-height ratio -- no metadata file.
"""
import glob
import os
import sys

from PIL import Image, ImageChops, ImageEnhance

ALPHA = 10                      # ignore near-invisible fringe
SATURATION, CONTRAST = 1.3, 1.08  # legacy sprites are punchier than the raw render
# Body height in pixels after export. A building is ~2.15 tiles tall and the
# camera tops out at 24 px/tile * maxZoom, so ~130 px on screen: 256 keeps it
# sharp with room to spare without spending GPU memory on 1000 px frames.
BODY_HEIGHT = 256


def shrink(im, factor):
    return im.resize((max(1, round(im.width * factor)), max(1, round(im.height * factor))), Image.LANCZOS)


def bbox(im):
    return im.getchannel("A").point(lambda a: 255 if a > ALPHA else 0).getbbox()


def solid_bbox(im):
    """Box of the object itself, ignoring the shadow-catcher's black shadow.

    The catcher leaves a faint black veil over the whole ground disc; cropping
    by alpha alone kept the full frame and shrank the building in game.
    """
    a = im.getchannel("A").point(lambda v: 255 if v > 60 else 0)
    lum = im.convert("RGB").convert("L").point(lambda v: 255 if v > 22 else 0)
    return ImageChops.multiply(a, lum).getbbox()


def punch(im):
    rgb, alpha = im.convert("RGB"), im.getchannel("A")
    rgb = ImageEnhance.Contrast(ImageEnhance.Color(rgb).enhance(SATURATION)).enhance(CONTRAST)
    rgb.putalpha(alpha)
    return rgb


def union(boxes):
    boxes = [b for b in boxes if b]
    return (min(b[0] for b in boxes), min(b[1] for b in boxes), max(b[2] for b in boxes), max(b[3] for b in boxes))


slug, src = sys.argv[1], sys.argv[2]
smoke_dir = sys.argv[3] if len(sys.argv) > 3 else None
root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
dst = os.path.join(root, "internal", "assets", "buildings", slug)
os.makedirs(dst, exist_ok=True)

bodies = sorted(glob.glob(os.path.join(src, "body*.png")))
images = [Image.open(p).convert("RGBA") for p in bodies]
box = union([solid_bbox(im) for im in images])
pad = 6
box = (max(0, box[0] - pad), max(0, box[1] - pad), min(images[0].width, box[2] + pad), min(images[0].height, box[3] + pad))
factor = min(1.0, BODY_HEIGHT / (box[3] - box[1]))
for p, im in zip(bodies, images):
    punch(shrink(im.crop(box), factor)).save(os.path.join(dst, os.path.basename(p)), optimize=True)
print(f"{slug}: {len(images)} body frame(s), crop {box} -> {box[2] - box[0]}x{box[3] - box[1]}")

if smoke_dir:
    frames = [Image.open(p).convert("RGBA") for p in sorted(glob.glob(os.path.join(smoke_dir, "*.png")))]
    sbox = union([bbox(im) for im in frames])
    cx = (box[0] + box[2]) / 2
    half = max(cx - sbox[0], sbox[2] - cx, (box[2] - box[0]) / 2)
    left, right = round(cx - half), round(cx + half)
    top, bottom = min(sbox[1], box[1]), box[3]
    for i, im in enumerate(frames):
        canvas = Image.new("RGBA", (right - left, bottom - top))
        canvas.paste(im.crop((max(0, left), top, min(im.width, right), bottom)), (max(0, -left), 0))
        punch(shrink(canvas, factor)).save(os.path.join(dst, f"smoke_{i:02d}.png"), optimize=True)
    print(f"{slug}: {len(frames)} smoke frame(s), canvas {right - left}x{bottom - top} (body {box[3] - box[1]} px tall)")
