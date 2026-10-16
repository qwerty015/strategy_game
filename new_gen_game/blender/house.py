"""Generic timber-framed house: plinth, plaster walls, framing around openings,
gables and a shingled roof. Building scripts describe *what* they want
(size, ridge direction, doors/windows on the visible faces) and add their own
annexes and props on top.

Visible faces (camera from +X/-Y): "front" = the -Y wall (u runs along X),
"right" = the +X wall (u runs along Y). Openings:
    dict(kind="door"|"window", u=centre, w=width, z0=bottom, z1=top,
         glow=False, open_deg=0, shutters=True)
"""

import math

from mathutils import Matrix, Vector

import lib
import props

T = 0.075  # timber thickness


class House:
    def __init__(self, b, hx, hy, top=2.0, plinth=0.5, pitch_deg=42, ridge="y", eave_out=0.22, overhang=0.26,
                 openings=None):
        self.b, self.hx, self.hy = b, hx, hy
        self.top, self.pl = top, plinth
        self.mid = plinth + (top - plinth) * 0.52
        self.sill = plinth + 0.03
        self.pitch = math.radians(pitch_deg)
        self.tan = math.tan(self.pitch)
        self.ridge_axis = ridge
        self.span = hx if ridge == "y" else hy          # half span under the roof
        self.length = hy if ridge == "y" else hx        # half length along the ridge
        self.ridge_z = top + self.span * self.tan
        self.eave_out, self.overhang = eave_out, overhang
        self.eave_z = top - eave_out * self.tan
        self.openings = openings or {}

    # -- coordinates on a visible face
    def face(self, name):
        if name == "front":
            return dict(normal=Vector((0, -1, 0)), span=self.hx,
                        p=lambda u, z, out=0.0: Vector((u, -self.hy - 0.02 - out, z)),
                        gable=self.ridge_axis == "y")
        return dict(normal=Vector((1, 0, 0)), span=self.hy,
                    p=lambda u, z, out=0.0: Vector((self.hx + 0.02 + out, u, z)),
                    gable=self.ridge_axis == "x")

    # -- body
    def body(self):
        b = self.b
        hx, hy, pl, top = self.hx, self.hy, self.pl, self.top
        b.box("stone", (0, 0, pl / 2), (2 * hx + 0.12, 2 * hy + 0.12, pl))
        b.box("plaster", (0, 0, (pl + top) / 2), (2 * hx, 2 * hy, top - pl))
        for s in (-1, 1):
            if self.ridge_axis == "y":
                y = s * hy
                b.prism("plaster", [(-hx, y, top), (hx, y, top), (0, y, self.ridge_z)], (0, -s * 0.3, 0))
            else:
                x = s * hx
                b.prism("plaster", [(x, -hy, top), (x, hy, top), (x, 0, self.ridge_z)], (-s * 0.3, 0, 0))
        rng = b.rng
        for sx in (-1, 1):
            for sy in (-1, 1):
                for k in range(3):
                    w = 0.27 if k % 2 == 0 else 0.18
                    b.box("stone", (sx * (hx + 0.045), sy * (hy + 0.045), 0.09 + k * 0.165),
                          (w if k % 2 == 0 else 0.18, 0.18 if k % 2 == 0 else w, 0.15), tint=props._shade(rng, 0.9, 1.2))
        # hidden faces only get corner posts and top plates
        for x, y in ((-hx, hy), (-hx, -hy), (hx, hy), (hx, -hy)):
            b.box("wood_dark", (x * 1.012, y * 1.012, (self.sill + top) / 2), (T, T, top - self.sill))
        b.box("wood_dark", (0, hy + 0.02, top), (2 * hx, 0.07, T))
        b.box("wood_dark", (-hx - 0.02, 0, top), (0.07, 2 * hy, T))

    # -- framing
    def frame_face(self, name):
        b, f = self.b, self.face(name)
        p, n, span = f["p"], f["normal"], f["span"]
        ops = sorted(self.openings.get(name, []), key=lambda o: o["u"])

        def beam(u1, z1, u2, z2, t=T, d=0.07, out=0.0):
            b.beam("wood_dark", p(u1, z1, out), p(u2, z2, out), t, d, n)

        # posts: corners, both sides of every opening, then fill wide gaps
        posts = {-span, span}
        for o in ops:
            posts |= {o["u"] - o["w"] / 2 - 0.05, o["u"] + o["w"] / 2 + 0.05}
        posts = sorted(posts)
        filled = [posts[0]]
        for a, c in zip(posts, posts[1:]):
            gaps = max(1, math.ceil((c - a) / 0.72))
            filled += [a + (c - a) * k / gaps for k in range(1, gaps + 1)]
        posts = filled

        def blocked(u, z0, z1):
            for o in ops:
                if abs(u - o["u"]) < o["w"] / 2 + 0.04 and z1 > o["z0"] - 0.04 and z0 < o["z1"] + 0.04:
                    return True
            return False

        for u in posts:
            if not blocked(u, self.sill, self.top):
                beam(u, self.sill, u, self.top)
        beam(-span, self.top, span, self.top)
        for a, c in zip(posts, posts[1:]):
            mid = (a + c) / 2
            if not blocked(mid, self.sill - 0.02, self.sill + 0.02):
                beam(a, self.sill, c, self.sill)
            if not blocked(mid, self.mid - 0.02, self.mid + 0.02):
                beam(a, self.mid, c, self.mid)
            if not blocked(mid, self.sill, self.top) and c - a > 0.25:
                flip = int(round(mid * 3)) % 2
                lo = (a, c) if flip else (c, a)
                beam(lo[0], self.sill, lo[1], self.mid)
                beam(lo[1], self.mid, lo[0], self.top)
        for o in ops:
            self.opening(name, o)
        if f["gable"]:
            self.gable_timber(name)

    def opening(self, name, o):
        b, f = self.b, self.face(name)
        p, n = f["p"], f["normal"]
        u, w, z0, z1 = o["u"], o["w"], o["z0"], o["z1"]
        t = Vector((1, 0, 0)) if name == "front" else Vector((0, 1, 0))
        rot = props.right_handed(t, -n)
        key = "glow_soft" if o.get("glow") else "interior"
        b.box(key, p(u, (z0 + z1) / 2, -0.015), (w, 0.02, z1 - z0), rot)
        thick = 0.1 if o["kind"] == "door" else 0.075
        for s in (-1, 1):
            b.beam("wood_dark", p(u + s * (w / 2 + 0.03), z0 - (0 if o["kind"] == "door" else 0.04), 0.02),
                   p(u + s * (w / 2 + 0.03), z1 + 0.05, 0.02), thick, 0.09, n)
        b.beam("wood_dark", p(u - w / 2 - 0.14, z1 + 0.06, 0.03), p(u + w / 2 + 0.14, z1 + 0.06, 0.03), 0.12, 0.1, n)
        if o["kind"] == "door":
            ang = math.radians(o.get("open_deg", 0))
            hinge = p(u - w / 2 + 0.01, 0.02 if z0 < 0.1 else z0, 0.04)
            along = Matrix.Rotation(-ang, 3, "Z") @ t     # swings outward on either face
            out = Matrix.Rotation(-ang, 3, "Z") @ n
            props.door_leaf(b, hinge, (along.x, along.y), (out.x, out.y), w - 0.02, z1 - z0 - 0.04)
            step = p(u, 0.05, 0.16)
            b.box("stone", step, (w + 0.3, 0.3, 0.1), rot, tint=(1.05, 1.05, 1.05))
        else:
            b.beam("wood_dark", p(u - w / 2 - 0.08, z0 - 0.02, 0.03), p(u + w / 2 + 0.08, z0 - 0.02, 0.03), 0.07, 0.12, n)
            b.beam("wood_dark", p(u, z0, 0.01), p(u, z1, 0.01), 0.03, 0.03, n)
            b.beam("wood_dark", p(u - w / 2, (z0 + z1) / 2, 0.01), p(u + w / 2, (z0 + z1) / 2, 0.01), 0.03, 0.03, n)
            if o.get("shutters", True):
                for s in (-1, 1):
                    hinge = p(u + s * w / 2, z0 + 0.02, 0.03)
                    ang = math.radians(75) * s
                    along = Matrix.Rotation(-ang, 3, "Z") @ (t * s)
                    out = Matrix.Rotation(-ang, 3, "Z") @ n
                    props.door_leaf(b, hinge, (along.x, along.y), (out.x, out.y), w / 2, z1 - z0 - 0.04, planks=2)
            if o.get("box"):                     # flower box under the window
                b.box("wood_light", p(u, z0 - 0.1, 0.1), (w + 0.1, 0.14, 0.12), rot, tint=(0.8, 0.8, 0.8))
                for k in range(6):
                    q = p(u - w / 2 + 0.05 + k * (w / 5), z0 - 0.02, 0.1)
                    b.sphere("leaf", q, 0.06, scale=(1, 1, 0.8), segments=6)
                    b.sphere("flower", q + Vector((0, 0, 0.05)), 0.028, segments=6,
                             tint=props.FLOWER_TINTS[k % 3])

    def gable_timber(self, name):
        b, f = self.b, self.face(name)
        p, n = f["p"], f["normal"]
        collar = self.top + (self.ridge_z - self.top) * 0.5
        half = (self.ridge_z - collar) / self.tan

        def beam(u1, z1, u2, z2):
            b.beam("wood_dark", p(u1, z1), p(u2, z2), T, 0.07, n)

        beam(-half, collar, half, collar)
        beam(0, collar, 0, self.ridge_z - 0.05)
        beam(0, self.top, 0, collar)
        for s in (-1, 1):
            beam(s * self.span * 0.68, self.top, s * 0.08, collar)

    # -- roof
    def roof(self, skip=None):
        b = self.b
        P = Vector((1, 0, 0)) if self.ridge_axis == "y" else Vector((0, 1, 0))
        A = Vector((0, 1, 0)) if self.ridge_axis == "y" else Vector((1, 0, 0))
        half_len = self.length + self.overhang
        ridge = Vector((0, 0, self.ridge_z))
        rng = b.rng
        for s in (-1, 1):
            eave = P * (s * (self.span + self.eave_out)) + Vector((0, 0, self.eave_z))
            up = (ridge - eave).normalized()
            length = (ridge - eave).length
            n = P * (s * math.sin(self.pitch)) + Vector((0, 0, math.cos(self.pitch)))
            base = Matrix((up.cross(n), up, n)).transposed()
            b.box("wood_dark", (eave + ridge) / 2 + n * 0.02, (2 * half_len, length, 0.05), base)
            k = -half_len + 0.06
            while k < half_len:
                b.box("wood_dark", eave + A * k - n * 0.04 + up * 0.05, (0.07, 0.16, 0.07), base)
                k += 0.33
            lib.shingles(b, "slate", eave, up, n, half_len, length, skip=skip)
            for sa in (-1, 1):
                b.beam("wood_dark", eave + A * (sa * half_len) + n * 0.06, ridge + A * (sa * half_len) + n * 0.06, 0.12, 0.06,
                       A * sa)
            b.beam("wood_dark", eave - A * half_len - n * 0.02, eave + A * half_len - n * 0.02, 0.08, 0.06, n)
        k = -half_len
        axis = "Y" if self.ridge_axis == "y" else "X"
        while k < half_len:
            g = rng.uniform(0.8, 1.1)
            size = (0.17, 0.25, 0.17) if self.ridge_axis == "y" else (0.25, 0.17, 0.17)
            b.box("slate", ridge + A * (k + 0.13) + Vector((0, 0, 0.1)), size, Matrix.Rotation(math.pi / 4, 3, axis), tint=(g, g, g))
            k += 0.27
        for sa in (-1, 1):
            b.box("wood_dark", ridge + A * (sa * (half_len + 0.02)) + Vector((0, 0, 0.1)), (0.07, 0.07, 0.36))

    def slope_z(self, coord):
        """Roof surface height above a point `coord` units off the ridge line."""
        return self.ridge_z - abs(coord) * self.tan + 0.08

    def build_all(self, roof_skip=None):
        self.body()
        for name in ("front", "right"):
            self.frame_face(name)
        self.roof(roof_skip)
