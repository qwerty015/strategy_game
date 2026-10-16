"""Reusable set dressing: barrels, crates, sacks, carts, fences, doors, plants.

Every function takes a lib.Builder and draws into its material buckets, so a
whole building with props still ends up as a handful of objects. Material
keys used: wood_dark, wood_light, iron, burlap, grass, flower, leaf, grain.
"""

import math

from mathutils import Matrix, Vector


def _shade(rng, lo=0.85, hi=1.1):
    v = rng.uniform(lo, hi)
    return (v, v, v)


def right_handed(a, n):
    """Rotation matrix whose local X = a, local Y = +/-n (kept right-handed), Z up."""
    a, n = Vector(a).normalized(), Vector(n).normalized()
    z = a.cross(n)
    if z.z < 0:
        n = -n
        z = -z
    return Matrix((a, n, z)).transposed()


def barrel(b, x, y, z=0.0, h=0.44, r=0.16, lying=False, yaw=0.0):
    rng = b.rng
    tone = _shade(rng)
    if lying:
        rot = Matrix.Rotation(yaw, 3, "Z") @ Matrix.Rotation(math.pi / 2, 3, "Y")
        base = Vector((x, y, z + r))
    else:
        rot = Matrix.Identity(3)
        base = Vector((x, y, z + h / 2))
    axis = rot @ Vector((0, 0, 1))

    def at(frac):
        return base + axis * (h * (frac - 0.5))

    b.cylinder("wood_light", at(0.25), r * 0.88, r, h / 2, segments=20, rot=rot, tint=tone)
    b.cylinder("wood_light", at(0.75), r, r * 0.88, h / 2, segments=20, rot=rot, tint=tone)
    for f, rr in ((0.07, 0.905), (0.38, 0.985), (0.62, 0.985), (0.93, 0.905)):
        b.cylinder("iron", at(f), r * rr + 0.008, r * rr + 0.008, 0.025, segments=20, rot=rot)
    b.cylinder("wood_dark", at(1.0) + axis * 0.002, r * 0.84, r * 0.84, 0.01, segments=20, rot=rot)


def crate(b, c, size, yaw=0.0):
    rng = b.rng
    r = Matrix.Rotation(yaw, 3, "Z")
    c = Vector(c)
    b.box("wood_light", c, (size, size, size), r, tint=_shade(rng))
    h = size / 2 + 0.008
    for ex in (-1, 1):
        for ey in (-1, 1):
            b.box("wood_dark", c + r @ Vector((ex * (h - 0.02), ey * (h - 0.02), 0)), (0.045, 0.045, size + 0.012), r)
    for ez in (-1, 1):
        b.box("wood_dark", c + Vector((0, 0, ez * (h - 0.02))), (size + 0.012, size + 0.012, 0.035), r)
    for side in (Vector((0, -1, 0)), Vector((1, 0, 0))):
        n = r @ side
        t = r @ Vector((-side.y, side.x, 0))
        b.beam("wood_dark", c + n * h + t * (-h + 0.03) + Vector((0, 0, -h + 0.03)),
               c + n * h + t * (h - 0.03) + Vector((0, 0, h - 0.03)), 0.04, 0.02, n)


def sack(b, x, y, z=0.0, size=0.15, yaw=0.0, lean=0.0):
    rng = b.rng
    rot = Matrix.Rotation(yaw, 3, "Z") @ Matrix.Rotation(lean, 3, "X")
    b.sphere("burlap", (x, y, z + size * 0.9), size, scale=(0.9, 0.75, 1.0), segments=14, rot=rot, tint=_shade(rng))
    top = Vector((x, y, z + size * 0.9)) + rot @ Vector((0, 0, size * 0.95))
    b.cylinder("burlap", top, size * 0.28, size * 0.12, size * 0.35, segments=8, rot=rot, tint=_shade(rng, 0.7, 0.85))


def tuft(b, x, y, size=1.0, z=0.0):
    rng = b.rng
    for _ in range(rng.randint(6, 10)):
        h = rng.uniform(0.09, 0.22) * size
        tilt = Matrix.Rotation(rng.uniform(-0.5, 0.5), 3, "X") @ Matrix.Rotation(rng.uniform(-0.5, 0.5), 3, "Y")
        c = Vector((x + rng.uniform(-0.06, 0.06), y + rng.uniform(-0.06, 0.06), z)) + tilt @ Vector((0, 0, h / 2))
        g = rng.uniform(0.7, 1.4)
        b.cylinder("grass", c, 0.018 * size, 0.0, h, segments=4, rot=tilt, tint=(g * 0.9, g, g * 0.75))


FLOWER_TINTS = ((1.0, 0.25, 0.2), (1.0, 0.85, 0.2), (0.75, 0.45, 1.0), (1.0, 1.0, 1.0))


def flowers(b, x, y, count=5):
    rng = b.rng
    tint = rng.choice(FLOWER_TINTS)
    for _ in range(count):
        px, py = x + rng.uniform(-0.1, 0.1), y + rng.uniform(-0.1, 0.1)
        h = rng.uniform(0.08, 0.16)
        b.cylinder("grass", (px, py, h / 2), 0.008, 0.008, h, segments=4)
        b.sphere("flower", (px, py, h), 0.026, scale=(1, 1, 0.6), segments=8, tint=tint)


def ivy(b, origin, normal, width, height, density=140):
    """Leaves crawling up a wall: thick at the bottom, thinning out upward."""
    rng = b.rng
    n = Vector(normal).normalized()
    side = n.cross(Vector((0, 0, 1))).normalized()
    rot = n.to_track_quat("Z", "Y").to_matrix()
    for _ in range(density):
        v = rng.random() ** 1.7                       # bias toward the ground
        u = rng.uniform(-0.5, 0.5) * width * (1.0 - 0.6 * v)
        p = Vector(origin) + side * u + Vector((0, 0, v * height)) + n * rng.uniform(0.01, 0.05)
        g = rng.uniform(0.65, 1.25)
        twist = Matrix.Rotation(rng.uniform(0, math.tau), 3, "Z")
        b.sphere("leaf", p, rng.uniform(0.035, 0.055), scale=(1.0, 0.7, 0.3), segments=6, rot=rot @ twist,
                 tint=(g * 0.9, g, g * 0.7))


def door_leaf(b, hinge, along, outward, width, height, planks=5):
    """Plank door hanging on `hinge`; `outward` is the side its braces face."""
    rng = b.rng
    a = Vector((*along, 0)).normalized()
    n = Vector((*outward, 0)).normalized()
    rot = right_handed(a, n)
    o = Vector(hinge)
    z = Vector((0, 0, 1))

    def P(lx, ly, lz):
        return o + a * lx + n * ly + z * lz

    pw = width / planks
    for i in range(planks):
        b.box("wood_light", P((i + 0.5) * pw, 0, height / 2), (pw - 0.012, 0.05, height), rot, tint=_shade(rng, 0.78, 1.12))
    for zz in (0.24, height - 0.24):
        b.beam("wood_light", P(0.03, 0.045, zz), P(width - 0.03, 0.045, zz), 0.1, 0.04, n, tint=(0.8, 0.8, 0.8))
        b.beam("iron", P(-0.02, 0.07, zz + 0.075), P(width * 0.7, 0.07, zz + 0.075), 0.04, 0.012, n)
    b.beam("wood_light", P(0.07, 0.045, 0.3), P(width - 0.07, 0.045, height - 0.3), 0.09, 0.04, n, tint=(0.8, 0.8, 0.8))
    b.cylinder("iron", P(width - 0.1, 0.085, height * 0.48), 0.05, 0.05, 0.014, segments=16, rot=rot @ Matrix.Rotation(math.pi / 2, 3, "X"))


def cart(b, pos, yaw=0.0, load=True):
    rng = b.rng
    r = Matrix.Rotation(yaw, 3, "Z")
    o = Vector(pos)

    def P(x, y, z):
        return o + r @ Vector((x, y, z))

    wr = 0.24
    b.box("wood_light", P(0, 0, 0.42), (0.82, 0.5, 0.06), r)
    for sy in (-1, 1):
        b.box("wood_light", P(0, sy * 0.25, 0.52), (0.82, 0.04, 0.16), r, tint=(0.85, 0.85, 0.85))
    for sx in (-1, 1):
        b.box("wood_light", P(sx * 0.41, 0, 0.52), (0.04, 0.5, 0.16), r, tint=(0.85, 0.85, 0.85))
    for sy in (-1, 1):                                    # shafts resting on the ground
        b.beam("wood_dark", P(0.35, sy * 0.18, 0.42), P(1.1, sy * 0.22, 0.04), 0.05, 0.05, r @ Vector((0, 1, 0)))
    wheel_rot = r @ Matrix.Rotation(math.pi / 2, 3, "X")
    for sy in (-1, 1):
        c = P(-0.05, sy * 0.33, wr)
        b.cylinder("wood_dark", c, wr, wr, 0.05, segments=24, rot=wheel_rot)
        b.cylinder("wood_light", c + r @ Vector((0, sy * 0.012, 0)), wr - 0.045, wr - 0.045, 0.05, segments=24, rot=wheel_rot,
                   tint=(0.6, 0.6, 0.6))
        b.cylinder("iron", c, 0.05, 0.05, 0.09, segments=12, rot=wheel_rot)
        axis = r @ Vector((0, 1, 0))
        for k in range(6):
            ang = k * math.pi / 3 + 0.3
            tip = c + r @ Vector((math.cos(ang) * (wr - 0.03), sy * 0.03, math.sin(ang) * (wr - 0.03)))
            b.beam("wood_light", c + axis * sy * 0.03, tip, 0.03, 0.03, axis)
    b.cylinder("wood_dark", P(-0.05, 0, wr), 0.03, 0.03, 0.72, segments=8, rot=wheel_rot)
    if load:
        for dx, dy, yaw2 in ((-0.2, -0.1, 0.2), (0.12, 0.08, -0.4), (-0.05, 0.12, 1.2)):
            p = P(dx, dy, 0.45)
            sack(b, p.x, p.y, p.z, size=0.13, yaw=yaw + yaw2, lean=0.5)


def fence(b, p1, p2, step=0.42, height=0.5):
    rng = b.rng
    p1, p2 = Vector((*p1[:2], 0)), Vector((*p2[:2], 0))
    d = p2 - p1
    count = max(1, round(d.length / step))
    n = Vector((-d.y, d.x, 0)).normalized()
    for i in range(count + 1):
        p = p1 + d * (i / count)
        h = height + rng.uniform(-0.04, 0.04)
        b.box("wood_light", p + Vector((0, 0, h / 2)), (0.06, 0.06, h), Matrix.Rotation(rng.uniform(-0.2, 0.2), 3, "Z"),
              tint=_shade(rng, 0.65, 0.95))
    for zz in (0.18, 0.38):
        b.beam("wood_light", p1 + Vector((0, 0, zz)) + n * 0.035, p2 + Vector((0, 0, zz + rng.uniform(-0.02, 0.02))) + n * 0.035,
               0.045, 0.03, n, tint=_shade(rng, 0.75, 1.0))


def lantern(b, wall_point, normal, out=0.2):
    n = Vector(normal).normalized()
    p = Vector(wall_point)
    b.beam("iron", p, p + n * out, 0.03, 0.03, Vector((0, 0, 1)).cross(n))
    c = p + n * out - Vector((0, 0, 0.12))
    b.box("iron", c + Vector((0, 0, 0.09)), (0.14, 0.14, 0.025))
    b.box("iron", c - Vector((0, 0, 0.09)), (0.13, 0.13, 0.025))
    for ox in (-1, 1):
        for oy in (-1, 1):
            b.box("iron", c + Vector((ox * 0.06, oy * 0.06, 0)), (0.015, 0.015, 0.17))
    b.box("glow", c, (0.1, 0.1, 0.15))
    b.cylinder("iron", c + Vector((0, 0, 0.14)), 0.085, 0.0, 0.08, segments=4, rot=Matrix.Rotation(math.pi / 4, 3, "Z"))
