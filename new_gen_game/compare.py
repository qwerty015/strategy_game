"""Side-by-side: current game sprite vs new Blender render, big and at in-game size.

    python new_gen_game/compare.py <old.png> <new.png> <out.png>
"""
import sys
from PIL import Image, ImageDraw, ImageFont

out = sys.argv[3]
old, new = (Image.open(p).convert("RGBA") for p in sys.argv[1:3])


def fit(im, h):
    im = im.crop(im.getbbox())
    return im.resize((max(1, round(im.width * h / im.height)), h), Image.LANCZOS)


BIG, SMALL = 520, 64           # 64 px ~ warehouse height in game at 1.25x zoom
canvas = Image.new("RGBA", (1180, 760), (58, 92, 52, 255))  # grass-ish backdrop
d = ImageDraw.Draw(canvas)
try:
    font = ImageFont.truetype("arial.ttf", 22)
except OSError:
    font = ImageFont.load_default()
for i, (label, im) in enumerate((("сейчас в игре", old), ("Blender 3D", new))):
    x0 = 40 + i * 570
    big = fit(im, BIG)
    canvas.alpha_composite(big, (x0 + (520 - big.width) // 2, 50))
    d.text((x0, 18), label, fill=(255, 255, 255, 255), font=font)
    for j, h in enumerate((SMALL, SMALL * 2)):
        s = fit(im, h)
        canvas.alpha_composite(s, (x0 + j * 160, 600 + (128 - h)))
canvas.save(out)
print("wrote", out)
