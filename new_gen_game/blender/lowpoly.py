"""Low-poly kit for game-ready buildings (the Unity port).

The first four buildings (warehouse, mill, bakery, smeltery) were made for
2D sprites: every shingle, plank and stone is real geometry plus a bevel
modifier -- 150-300k triangles each, far too heavy for integrated GPUs once a
town has dozens of them. Everything built with this kit instead keeps detail
in *texture* (baked from the procedural materials on export) and geometry in
big simple shapes:

  * walls, roofs, posts, planks -- plain boxes/prisms, no bevel;
  * shingles/logs/bricks -- material patterns (brick texture, wood grain),
    not separate meshes;
  * cylinders 6-10 segments, spheres 6-8.

Budget: ~1-3k triangles per building, one baked texture (export_unity.py
--game joins everything into one mesh). Unity's camera orbits freely, so
all four walls are finished, not just the two the sprite camera saw.

Faces: "front" = -Y (the entrance; faces south in the game), "right" = +X,
"back" = +Y, "left" = -X.
"""

import math

from mathutils import Matrix, Vector

import lib


def materials():
    """Shared palette; scripts may add their own keys on top."""
    return {
        "plaster": lib.mat_noise("plaster", [(0.3, (0.60, 0.50, 0.33)), (0.75, (0.80, 0.70, 0.50))], scale=3, bump=0.0, rough=0.95),
        "plaster_white": lib.mat_noise("plaster_white", [(0.3, (0.70, 0.67, 0.58)), (0.75, (0.88, 0.85, 0.76))], scale=3, bump=0.0),
        "wood_dark": lib.mat_wood("wood_dark", (0.07, 0.035, 0.015), (0.19, 0.095, 0.04)),
        "wood_light": lib.mat_wood("wood_light", (0.26, 0.14, 0.055), (0.52, 0.31, 0.13)),
        "planks": planks("planks", (0.20, 0.10, 0.04), (0.42, 0.24, 0.10)),
        "logs": logs("logs", (0.20, 0.11, 0.05), (0.40, 0.24, 0.11)),
        "stone": lib.mat_stone("stone", (0.30, 0.27, 0.22), (0.46, 0.42, 0.35), (0.10, 0.09, 0.07), scale=2.6),
        "stone_grey": lib.mat_stone("stone_grey", (0.30, 0.30, 0.31), (0.48, 0.48, 0.47), (0.12, 0.12, 0.12), scale=2.6),
        "roof_red": tiles("roof_red", (0.30, 0.07, 0.035), (0.50, 0.15, 0.07), (0.12, 0.03, 0.015)),
        "roof_slate": tiles("roof_slate", (0.05, 0.06, 0.09), (0.11, 0.12, 0.16), (0.02, 0.02, 0.03)),
        "roof_brown": tiles("roof_brown", (0.16, 0.08, 0.035), (0.30, 0.16, 0.07), (0.06, 0.03, 0.012)),
        "thatch": lib.mat_noise("thatch", [(0.25, (0.30, 0.20, 0.06)), (0.55, (0.55, 0.40, 0.14)), (0.8, (0.72, 0.56, 0.22))],
                                scale=18, bump=0.0, rough=1.0),
        "iron": lib.mat_flat("iron", (0.06, 0.06, 0.065), rough=0.45, metallic=0.85),
        "glass": lib.mat_flat("glass", (0.05, 0.07, 0.09), rough=0.2),
        "glow": lib.mat_flat("glow", (1.0, 0.72, 0.32), emission=(1.0, 0.55, 0.18), emission_strength=6),
        "cloth": lib.mat_noise("cloth", [(0.3, (0.40, 0.08, 0.05)), (0.7, (0.58, 0.14, 0.08))], scale=20, rough=0.9, bump=0.0),
        "straw": lib.mat_noise("straw", [(0.3, (0.55, 0.40, 0.12)), (0.7, (0.85, 0.68, 0.28))], scale=25, rough=1.0, bump=0.0),
        "dirt": lib.mat_noise("dirt", [(0.3, (0.22, 0.14, 0.06)), (0.72, (0.40, 0.27, 0.12))], scale=3.5, rough=1.0, bump=0.0),
        "rope": lib.mat_flat("rope", (0.45, 0.36, 0.20), rough=1.0),
        "rock": lib.mat_noise("rock", [(0.25, (0.07, 0.065, 0.06)), (0.55, (0.13, 0.125, 0.115)), (0.8, (0.21, 0.20, 0.185))],
                              scale=4, bump=0.0, rough=0.95),
    }


def _pattern(name, c1, c2, mortar, brick_w, row_h, scale, mortar_size, offset=0.5):
    """Brick-texture pattern wrapped like lib.mat_stone (u = x + y, v = z)."""
    mat = lib.mat_stone(name, c1, c2, mortar, scale=scale)
    brick = next(n for n in mat.node_tree.nodes if n.type == "TEX_BRICK")
    brick.inputs["Brick Width"].default_value = brick_w
    brick.inputs["Row Height"].default_value = row_h
    brick.inputs["Mortar Size"].default_value = mortar_size
    brick.offset = offset
    return mat


def tiles(name, c1, c2, gap):
    """Roof tiles: short wide 'bricks' in horizontal rows -- reads as shingles on a slope."""
    return _pattern(name, c1, c2, gap, brick_w=0.32, row_h=0.13, scale=3.2, mortar_size=0.03)


def planks(name, dark, light):
    """Vertical-looking board walls: long thin bricks with dark gaps."""
    return _pattern(name, dark, light, (dark[0] * 0.4, dark[1] * 0.4, dark[2] * 0.4), brick_w=1.6, row_h=0.12, scale=2.2,
                    mortar_size=0.015, offset=0.3)


def logs(name, dark, light):
    """Log-cabin walls: very long rows, chunky dark seams."""
    return _pattern(name, dark, light, (dark[0] * 0.35, dark[1] * 0.35, dark[2] * 0.35), brick_w=3.0, row_h=0.2, scale=2.0,
                    mortar_size=0.04, offset=0.0)


# ---------------------------------------------------------------- house


def _face_frame(name, hx, hy):
    """(point(u, z, out), outward normal, half span) for a wall face."""
    if name == "front":
        return (lambda u, z, out=0.0: Vector((u, -hy - out, z))), Vector((0, -1, 0)), hx
    if name == "back":
        return (lambda u, z, out=0.0: Vector((-u, hy + out, z))), Vector((0, 1, 0)), hx
    if name == "right":
        return (lambda u, z, out=0.0: Vector((hx + out, u, z))), Vector((1, 0, 0)), hy
    return (lambda u, z, out=0.0: Vector((-hx - out, -u, z))), Vector((-1, 0, 0)), hy


def house(b, hx, hy, top=1.5, plinth=0.25, wall="plaster", roof="roof_red", pitch_deg=40, ridge="y",
          overhang=0.22, timber=True, openings=None, chimney=None, origin=(0, 0)):
    """A complete low-poly house centred on `origin`.

    openings: {face: [dict(kind="door"|"window", u=, w=, z0=, z1=, glow=False)]}
    chimney:  (x, y) relative to the house centre, or None.
    Returns a dict with the roof geometry (ridge_z etc.) for annexes.
    """
    ox, oy = origin
    O = Vector((ox, oy, 0))
    openings = openings or {}
    pitch = math.radians(pitch_deg)
    span = hx if ridge == "y" else hy
    length = hy if ridge == "y" else hx
    ridge_z = top + span * math.tan(pitch)

    b.box("stone", O + Vector((0, 0, plinth / 2)), (2 * hx + 0.08, 2 * hy + 0.08, plinth))
    b.box(wall, O + Vector((0, 0, (plinth + top) / 2)), (2 * hx, 2 * hy, top - plinth))
    for s in (-1, 1):                                     # gable triangles
        if ridge == "y":
            pts = [(ox - hx, oy + s * hy, top), (ox + hx, oy + s * hy, top), (ox, oy + s * hy, ridge_z)]
            b.prism(wall, pts, (0, -s * 0.2, 0))
        else:
            pts = [(ox + s * hx, oy - hy, top), (ox + s * hx, oy + hy, top), (ox + s * hx, oy, ridge_z)]
            b.prism(wall, pts, (-s * 0.2, 0, 0))

    t = 0.07
    if timber:
        for x, y in ((-hx, -hy), (hx, -hy), (-hx, hy), (hx, hy)):        # corner posts
            b.box("wood_dark", O + Vector((x, y, (plinth + top) / 2)), (t * 1.4, t * 1.4, top - plinth))
    for name in ("front", "right", "back", "left"):
        P, n, half = _face_frame(name, hx, hy)
        P0 = lambda u, z, out=0.0: P(u, z, out) + O  # noqa: E731
        if timber:
            b.beam("wood_dark", P0(-half, top - t / 2, 0.01), P0(half, top - t / 2, 0.01), t, 0.05, n)
            b.beam("wood_dark", P0(-half, plinth + t / 2, 0.01), P0(half, plinth + t / 2, 0.01), t, 0.05, n)
            ops = openings.get(name, [])
            # one brace on each side of the openings, if there's room
            lo = min([o["u"] - o["w"] / 2 for o in ops], default=0.0)
            hi = max([o["u"] + o["w"] / 2 for o in ops], default=0.0)
            if lo + half > 0.45:
                b.beam("wood_dark", P0(-half + 0.08, plinth + 0.05, 0.01), P0(lo - 0.12, top - 0.08, 0.01), t, 0.05, n)
            if half - hi > 0.45:
                b.beam("wood_dark", P0(half - 0.08, plinth + 0.05, 0.01), P0(hi + 0.12, top - 0.08, 0.01), t, 0.05, n)
        for o in openings.get(name, []):
            opening(b, P0, n, o)

    _gable_roof(b, O, hx, hy, top, ridge_z, pitch, overhang, ridge, roof)
    if chimney:
        cx, cy = chimney
        h = ridge_z + 0.35
        b.box("stone", O + Vector((cx, cy, h / 2 + 0.3)), (0.3, 0.3, h - 0.6))
        b.box("stone", O + Vector((cx, cy, h + 0.02)), (0.38, 0.38, 0.08))
    return dict(ridge_z=ridge_z, top=top, span=span, length=length, pitch=pitch)


def opening(b, P, n, o):
    """Door or window set into a wall: dark recess + frame + sill/lintel."""
    u, w, z0, z1 = o["u"], o["w"], o["z0"], o["z1"]
    c = (P(u, (z0 + z1) / 2, 0.0))
    side = Vector((-n.y, n.x, 0))
    if o.get("kind") == "door":
        b.box("planks", c + n * 0.015, _sized(n, w, 0.04, z1 - z0))
        b.box("iron", c + n * 0.04 + side * (w * 0.3) + Vector((0, 0, -0.05)), _sized(n, 0.05, 0.03, 0.05))
    else:
        key = "glow" if o.get("glow") else "glass"
        b.box(key, c + n * 0.01, _sized(n, w, 0.03, z1 - z0))
        b.box("wood_dark", c + n * 0.03, _sized(n, 0.04, 0.03, z1 - z0))               # mullion
        b.box("wood_dark", P(u, z0 - 0.02, 0.05), _sized(n, w + 0.12, 0.1, 0.05))      # sill
    b.box("wood_dark", P(u, z1 + 0.03, 0.03), _sized(n, w + 0.12, 0.06, 0.07))         # lintel
    for s in (-1, 1):
        b.box("wood_dark", c + n * 0.03 + side * (s * (w / 2 + 0.03)), _sized(n, 0.06, 0.06, z1 - z0))


def _sized(n, along, depth, height):
    """Box size for a part lying on a wall with normal n."""
    if abs(n.x) > 0.5:
        return (depth, along, height)
    return (along, depth, height)


def _gable_roof(b, O, hx, hy, top, ridge_z, pitch, overhang, ridge, key):
    P = Vector((1, 0, 0)) if ridge == "y" else Vector((0, 1, 0))
    A = Vector((0, 1, 0)) if ridge == "y" else Vector((1, 0, 0))
    span = hx if ridge == "y" else hy
    half_len = (hy if ridge == "y" else hx) + overhang
    eave_out = overhang
    eave_z = top - eave_out * math.tan(pitch)
    ridge_p = O + Vector((0, 0, ridge_z))
    for s in (-1, 1):
        eave = O + P * (s * (span + eave_out)) + Vector((0, 0, eave_z))
        up = (ridge_p - eave).normalized()
        n = P * (s * math.sin(pitch)) + Vector((0, 0, math.cos(pitch)))
        base = Matrix((up.cross(n), up, n)).transposed()
        length = (ridge_p - eave).length
        b.box(key, (eave + ridge_p) / 2 + n * 0.05, (2 * half_len, length + 0.03, 0.08), base)
        b.beam("wood_dark", eave - A * half_len + Vector((0, 0, -0.02)), eave + A * half_len + Vector((0, 0, -0.02)), 0.06, 0.05, n)
        for sa in (-1, 1):                                  # barge boards
            b.beam("wood_dark", eave + A * (sa * half_len) + n * 0.06, ridge_p + A * (sa * half_len) + n * 0.06, 0.1, 0.05, A * sa)
    axis = "Y" if ridge == "y" else "X"
    size = (0.16, 2 * half_len + 0.04, 0.16) if ridge == "y" else (2 * half_len + 0.04, 0.16, 0.16)
    b.box(key, ridge_p + Vector((0, 0, 0.08)), size, Matrix.Rotation(math.pi / 4, 3, axis), tint=(0.8, 0.8, 0.8))


def lean_to(b, x0, x1, y0, y1, z_low, z_high, key="roof_brown", posts=True, wall=None):
    """Mono-pitch shed roof over the rectangle, sloping down toward y0."""
    c = Vector(((x0 + x1) / 2, (y0 + y1) / 2, (z_low + z_high) / 2))
    rise = z_high - z_low
    run = y1 - y0
    ang = math.atan2(rise, run)
    b.box(key, c + Vector((0, 0, 0.04)), (x1 - x0 + 0.1, math.hypot(rise, run) + 0.1, 0.07), Matrix.Rotation(ang, 3, "X"))
    if posts:
        for x in (x0 + 0.05, x1 - 0.05):
            b.box("wood_dark", (x, y0 + 0.05, z_low / 2), (0.08, 0.08, z_low))
    if wall:
        b.box(wall, ((x0 + x1) / 2, y1 - 0.03, z_high / 2), (x1 - x0, 0.06, z_high))


# ---------------------------------------------------------------- props (cheap)


def barrel(b, x, y, z=0.0, h=0.42, r=0.15, lying=False, yaw=0.0, key="wood_light"):
    if lying:
        rot = Matrix.Rotation(yaw, 3, "Z") @ Matrix.Rotation(math.pi / 2, 3, "Y")
        c = Vector((x, y, z + r))
    else:
        rot = Matrix.Identity(3)
        c = Vector((x, y, z + h / 2))
    tone = b.rng.uniform(0.85, 1.1)
    b.cylinder(key, c, r * 0.92, r * 0.92, h, segments=8, rot=rot, tint=(tone, tone, tone))
    axis = rot @ Vector((0, 0, 1))
    for f in (-0.3, 0.3):
        b.cylinder("iron", c + axis * (h * f), r * 0.97, r * 0.97, 0.03, segments=8, rot=rot)


def crate(b, x, y, z=0.0, size=0.32, yaw=0.0):
    tone = b.rng.uniform(0.8, 1.1)
    b.box("planks", (x, y, z + size / 2), (size, size, size), Matrix.Rotation(yaw, 3, "Z"), tint=(tone, tone, tone))


def sack(b, x, y, z=0.0, size=0.14, tint=None):
    t = tint or (b.rng.uniform(0.85, 1.1),) * 3
    b.sphere("straw", (x, y, z + size * 0.85), size, scale=(0.9, 0.8, 1.0), segments=6, tint=t)


def log_pile(b, x, y, length=0.9, rows=3, yaw=0.0, r=0.07):
    rot = Matrix.Rotation(yaw, 3, "Z") @ Matrix.Rotation(math.pi / 2, 3, "Y")
    side = Matrix.Rotation(yaw, 3, "Z") @ Vector((0, 1, 0))
    for row in range(rows):
        count = rows - row + 1
        for i in range(count):
            off = (i - (count - 1) / 2) * r * 2.05
            c = Vector((x, y, r + row * r * 1.75)) + side * off
            tone = b.rng.uniform(0.8, 1.1)
            b.cylinder("logs", c, r, r, length * b.rng.uniform(0.92, 1.0), segments=6, rot=rot, tint=(tone, tone, tone))


def fence(b, p1, p2, step=0.45, height=0.42, key="wood_light"):
    p1, p2 = Vector((*p1[:2], 0)), Vector((*p2[:2], 0))
    d = p2 - p1
    count = max(1, round(d.length / step))
    n = Vector((-d.y, d.x, 0)).normalized()
    for i in range(count + 1):
        p = p1 + d * (i / count)
        b.box(key, p + Vector((0, 0, height / 2)), (0.06, 0.06, height))
    for zz in (0.16, 0.34):
        b.beam(key, p1 + Vector((0, 0, zz)) + n * 0.035, p2 + Vector((0, 0, zz)) + n * 0.035, 0.045, 0.03, n)


def rock(b, x, y, size=0.4, key="rock", squash=0.7):
    rot = Matrix.Rotation(b.rng.uniform(0, math.tau), 3, "Z") @ Matrix.Rotation(b.rng.uniform(-0.3, 0.3), 3, "X")
    tone = b.rng.uniform(0.85, 1.1)
    b.sphere(key, (x, y, size * squash * 0.55), size, scale=(1.0, 0.85, squash), segments=6, rot=rot, tint=(tone, tone, tone))


def ground(b, hx, hy, key="dirt"):
    """Flat yard patch under the building (very thin; the game draws grass around it)."""
    b.box(key, (0, 0, 0.005), (2 * hx, 2 * hy, 0.01))


def build(b):
    """No bevel (that was the 9x triangle multiplier), no ink hull."""
    return b.build(bevel=0.0, outline=0.0)


# ---------------------------------------------------------------- script plumbing


def begin(seed=7):
    """Fresh scene + shared palette; returns (Builder, materials dict, out_png)."""
    import os
    import sys

    args = sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else []
    res = int(args[1]) if len(args) > 1 else 768
    samples = int(args[2]) if len(args) > 2 else 48
    lib.clear_scene()
    lib.setup_render(res, samples)
    lib.setup_world()
    lib.add_sun()
    mats = materials()
    return lib.Builder(mats, seed=seed), mats, (os.path.abspath(args[0]) if args else None)


def finish(b, name, out=None):
    """Build meshes, save the .blend, report the triangle budget, render a preview."""
    import os

    import bpy

    objs = build(b)
    tris = sum(sum(len(p.vertices) - 2 for p in o.data.polygons) for o in objs)
    print(f"[lowpoly] {name}: {tris} triangles in {len(objs)} parts")
    here = os.path.dirname(os.path.abspath(__file__))
    models = os.path.join(here, "..", "models")
    os.makedirs(models, exist_ok=True)
    bpy.ops.wm.save_as_mainfile(filepath=os.path.join(models, name + ".blend"), compress=True)
    cam = lib.add_game_camera()
    lib.frame_camera(cam)
    lib.render(out or os.path.join(here, "..", "renders", name + ".png"))
    return tris


def boat_hull(b, x, y, z=0.0, length=1.1, width=0.45, height=0.22, yaw=0.0, key="planks", upside_down=False):
    """Pointed rowing boat: a 6-sided outline extruded, plus a keel/rim."""
    hl, hw = length / 2, width / 2
    pts = [(-hl, 0), (-hl * 0.55, -hw), (hl * 0.55, -hw), (hl, 0), (hl * 0.55, hw), (-hl * 0.55, hw)]
    r = Matrix.Rotation(yaw, 3, "Z")
    o = Vector((x, y, z))
    poly = [o + r @ Vector((px, py, 0)) for px, py in pts]
    b.prism(key, poly, (0, 0, height))
    rim_z = height + 0.01 if not upside_down else -0.01
    for i in range(len(poly)):
        p1, p2 = poly[i], poly[(i + 1) % len(poly)]
        n = (p2 - p1).cross(Vector((0, 0, 1))).normalized()
        b.beam("wood_dark", p1 + Vector((0, 0, rim_z)), p2 + Vector((0, 0, rim_z)), 0.04, 0.04, n)
    if upside_down:
        b.beam("wood_dark", o + r @ Vector((-hl, 0, height + 0.02)), o + r @ Vector((hl, 0, height + 0.02)), 0.05, 0.05, (0, 0, 1))
    else:
        b.box("wood_dark", o + Vector((0, 0, height * 0.6)), (0.08, width * 0.9, 0.04), r)     # thwart (seat)
