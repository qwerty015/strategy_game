"""Shared helpers for building game art procedurally inside Blender.

Run any model script through ../run.ps1 (headless Blender). Everything here
is plain bpy/bmesh -- no add-ons -- so a fresh Blender install is enough.

Conventions:
  * 1 Blender unit ~ 1 game tile edge (a building ~3x2 units sits on ~1-2 tiles
    visually; the game scales the sprite by its own tilesTall anyway).
  * Z is up, the camera looks from +X/-Y (south-east) at 35 degrees, so the
    -Y face ("front") and the +X face ("right") are the visible ones.
"""

import math
import random

import bmesh
import bpy
from mathutils import Matrix, Vector

# ---------------------------------------------------------------- scene


def clear_scene():
    bpy.ops.wm.read_factory_settings(use_empty=True)


def setup_render(resolution=1024, samples=192):
    scene = bpy.context.scene
    scene.render.engine = "CYCLES"
    scene.cycles.samples = samples
    scene.cycles.use_denoising = True
    scene.render.resolution_x = resolution
    scene.render.resolution_y = resolution
    scene.render.film_transparent = True
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"

    # GPU if available (OptiX on RTX, CUDA otherwise); CPU silently otherwise.
    try:
        prefs = bpy.context.preferences.addons["cycles"].preferences
        for backend in ("OPTIX", "CUDA", "HIP", "ONEAPI", "METAL"):
            try:
                prefs.compute_device_type = backend
            except TypeError:
                continue
            prefs.get_devices()
            gpus = [d for d in prefs.devices if d.type != "CPU"]
            if gpus:
                for d in prefs.devices:
                    d.use = d.type != "CPU"
                scene.cycles.device = "GPU"
                print(f"[lib] rendering on {backend}: {[d.name for d in gpus]}")
                break
    except Exception as exc:  # pragma: no cover - depends on machine
        print("[lib] GPU setup failed, using CPU:", exc)

    # Standard keeps the saturated, "painted" colours game art wants; AgX
    # desaturates toward photographic and made the first test look washed out.
    scene.view_settings.view_transform = "Standard"
    scene.view_settings.exposure = 0.1


def setup_world(color=(0.55, 0.62, 0.78), strength=0.55):
    world = bpy.data.worlds.new("World")
    bpy.context.scene.world = world
    world.use_nodes = True
    bg = world.node_tree.nodes["Background"]
    bg.inputs["Color"].default_value = (*color, 1)
    bg.inputs["Strength"].default_value = strength


def add_sun(direction=(0.35, 0.75, -0.9), strength=3.6, color=(1.0, 0.88, 0.70), angle_deg=4):
    data = bpy.data.lights.new("Sun", "SUN")
    data.energy = strength
    data.color = color
    data.angle = math.radians(angle_deg)
    sun = bpy.data.objects.new("Sun", data)
    bpy.context.scene.collection.objects.link(sun)
    sun.rotation_euler = (-Vector(direction)).to_track_quat("Z", "Y").to_euler()
    return sun


def add_game_camera(elevation_deg=35.0, azimuth_deg=45.0, distance=30.0):
    """Orthographic camera looking at the origin from the south-east."""
    data = bpy.data.cameras.new("Camera")
    data.type = "ORTHO"
    cam = bpy.data.objects.new("Camera", data)
    bpy.context.scene.collection.objects.link(cam)
    bpy.context.scene.camera = cam
    el, az = math.radians(elevation_deg), math.radians(azimuth_deg)
    cam.rotation_euler = (math.pi / 2 - el, 0, az)
    view_dir = Vector((-math.cos(el) * math.sin(az), math.cos(el) * math.cos(az), -math.sin(el)))
    cam.location = -view_dir * distance
    return cam


def frame_camera(cam, margin=1.04, frames=None):
    """Fit the ortho frame tightly around every mesh in the scene.

    frames: animation frames to union over, so moving parts (mill sails)
    never leave a fixed frame -- one scale for a whole sprite sheet.
    """
    scene = bpy.context.scene
    xs, ys = [], []
    for f in frames or [scene.frame_current]:
        scene.frame_set(f)
        _collect_extents(cam, xs, ys)
    w, h = max(xs) - min(xs), max(ys) - min(ys)
    size = max(w, h) * margin
    cam.data.ortho_scale = size
    cam.data.shift_x = (max(xs) + min(xs)) / 2 / size
    cam.data.shift_y = (max(ys) + min(ys)) / 2 / size


def _collect_extents(cam, xs, ys):
    bpy.context.view_layer.update()
    inv = cam.matrix_world.inverted()
    # Real vertices, not bounding boxes: a round ground patch's AABB corners
    # stick far outside the visible silhouette from a 45-degree view.
    for obj in bpy.context.scene.objects:
        if obj.type != "MESH":
            continue
        mw = inv @ obj.matrix_world
        for v in obj.data.vertices:
            p = mw @ v.co
            xs.append(p.x)
            ys.append(p.y)


def enable_outline(thickness=1.3, color=(0.05, 0.03, 0.018), alpha=0.8, crease_deg=140):
    """Freestyle ink lines: ties the 3D render to the hand-drawn look of the game."""
    scene = bpy.context.scene
    try:
        scene.render.use_freestyle = True
        scene.render.line_thickness_mode = "ABSOLUTE"
        scene.render.line_thickness = thickness
        vl = bpy.context.view_layer
        vl.use_freestyle = True
        fs = vl.freestyle_settings
        fs.crease_angle = math.radians(crease_deg)
        ls = fs.linesets[0] if len(fs.linesets) else fs.linesets.new("outline")
        ls.select_by_visibility = True
        ls.visibility = "VISIBLE"
        ls.select_by_edge_types = True
        ls.select_silhouette = True
        ls.select_border = True
        ls.select_crease = True
        if ls.linestyle is None:
            ls.linestyle = bpy.data.linestyles.new("ink")
        st = ls.linestyle
        st.color = color
        st.alpha = alpha
        st.thickness = thickness
    except Exception as exc:  # Freestyle missing in this build: render without lines
        print("[lib] outline disabled:", exc)


def outline_material(color=(0.035, 0.02, 0.012), name="ink"):
    """Inverted-hull ink: a flipped shell seen only past the silhouette.

    Front-facing shell faces and every non-camera ray (shadows, bounces) see
    a transparent surface, so the shell draws lines without casting shadows.
    """
    if name in bpy.data.materials:
        return bpy.data.materials[name]
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    nt = mat.node_tree
    nt.nodes.clear()
    out = nt.nodes.new("ShaderNodeOutputMaterial")
    geo = nt.nodes.new("ShaderNodeNewGeometry")
    lp = nt.nodes.new("ShaderNodeLightPath")
    # Flipped shell: its far side faces the camera (Backfacing = 0) and only
    # peeks out past the silhouette -- that is the line we keep.
    front = nt.nodes.new("ShaderNodeMath")
    front.operation = "SUBTRACT"
    front.inputs[0].default_value = 1.0
    nt.links.new(geo.outputs["Backfacing"], front.inputs[1])
    mul = nt.nodes.new("ShaderNodeMath")
    mul.operation = "MULTIPLY"
    nt.links.new(front.outputs["Value"], mul.inputs[0])
    nt.links.new(lp.outputs["Is Camera Ray"], mul.inputs[1])
    mix = nt.nodes.new("ShaderNodeMixShader")
    nt.links.new(mul.outputs["Value"], mix.inputs["Fac"])
    nt.links.new(nt.nodes.new("ShaderNodeBsdfTransparent").outputs[0], mix.inputs[1])
    em = nt.nodes.new("ShaderNodeEmission")
    em.inputs["Color"].default_value = (*color, 1)
    nt.links.new(em.outputs[0], mix.inputs[2])
    nt.links.new(mix.outputs[0], out.inputs["Surface"])
    return mat


def shingles(b, key, eave_mid, up, n, half_len, length, sw=0.16, sl=0.21, step=0.11, lift=0.07, skip=None, tilt_deg=7):
    """Overlapping rows of tiles on a roof plane.

    eave_mid: middle of the eave line; up: unit vector eave->ridge; n: outward
    normal; the row axis is up x n. `skip(center)` drops tiles that would poke
    through something sitting on the roof (dormer, chimney).
    """
    up, n = Vector(up).normalized(), Vector(n).normalized()
    xdir = up.cross(n)
    base = Matrix((xdir, up, n)).transposed()
    rng = b.rng
    row = 0
    while row * step + sl <= length + 0.03:
        t = row * step + sl / 2
        u = -half_len - (sw / 2 if row % 2 else 0)
        while u < half_len:
            left, right = max(u, -half_len), min(u + sw, half_len)
            u += sw
            if right - left < 0.04:
                continue
            c = Vector(eave_mid) + xdir * ((left + right) / 2 + rng.uniform(-0.008, 0.008)) + up * t + n * (lift + 0.004 * rng.random())
            if skip and skip(c):
                continue
            r = base @ Matrix.Rotation(rng.uniform(-0.05, 0.05), 3, "Z") @ Matrix.Rotation(math.radians(-tilt_deg), 3, "X")
            g = rng.uniform(0.68, 1.28)
            b.box(key, c, (right - left - 0.012, sl, 0.03), r, tint=(g, g, g * rng.uniform(0.95, 1.08)))
        row += 1
    return base


def save_outputs(models_dir, name):
    """Keep the editable scene (.blend) and a game-engine copy (.glb).

    The .blend keeps modifiers/procedural materials live, so it opens in
    Blender exactly as built. The .glb (modifiers applied) is for a future
    Unity/Godot import; procedural textures don't survive glTF, only base
    colours do -- baking is a later step.
    """
    import os

    os.makedirs(models_dir, exist_ok=True)
    blend = os.path.join(models_dir, name + ".blend")
    bpy.ops.wm.save_as_mainfile(filepath=blend, compress=True)
    print("[lib] saved", blend)
    try:
        glb = os.path.join(models_dir, name + ".glb")
        bpy.ops.export_scene.gltf(filepath=glb, export_format="GLB", export_apply=True, use_selection=False)
        print("[lib] exported", glb)
    except Exception as exc:  # glTF add-on missing or API drift: not fatal
        print("[lib] glb export skipped:", exc)


def render(path):
    bpy.context.scene.render.filepath = path
    bpy.ops.render.render(write_still=True)
    print("[lib] wrote", path)


# ---------------------------------------------------------------- materials


def _principled(name):
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    nt = mat.node_tree
    bsdf = nt.nodes["Principled BSDF"]
    return mat, nt, bsdf


def _coords(nt, kind):
    tc = nt.nodes.new("ShaderNodeTexCoord")
    return tc.outputs["UV" if kind == "UV" else "Object"]


def _ramp(nt, fac_socket, stops):
    ramp = nt.nodes.new("ShaderNodeValToRGB")
    els = ramp.color_ramp.elements
    els[0].position, els[0].color = stops[0][0], (*stops[0][1], 1)
    els[1].position, els[1].color = stops[-1][0], (*stops[-1][1], 1)
    for pos, col in stops[1:-1]:
        e = els.new(pos)
        e.color = (*col, 1)
    nt.links.new(fac_socket, ramp.inputs["Fac"])
    return ramp.outputs["Color"]


def _tinted(nt, color_socket, bsdf):
    """Multiply by the per-primitive 'tint' attribute written by Builder."""
    attr = nt.nodes.new("ShaderNodeAttribute")
    attr.attribute_name = "tint"
    mul = nt.nodes.new("ShaderNodeVectorMath")
    mul.operation = "MULTIPLY"
    nt.links.new(color_socket, mul.inputs[0])
    nt.links.new(attr.outputs["Color"], mul.inputs[1])
    nt.links.new(mul.outputs["Vector"], bsdf.inputs["Base Color"])


def _bump(nt, height_socket, bsdf, strength, distance=0.02, invert=False):
    bump = nt.nodes.new("ShaderNodeBump")
    bump.inputs["Strength"].default_value = strength
    bump.inputs["Distance"].default_value = distance
    bump.invert = invert
    nt.links.new(height_socket, bump.inputs["Height"])
    nt.links.new(bump.outputs["Normal"], bsdf.inputs["Normal"])


def mat_noise(name, stops, scale=6.0, rough=0.85, bump=0.25, metallic=0.0, coords="Object", detail=8.0):
    mat, nt, bsdf = _principled(name)
    noise = nt.nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = scale
    noise.inputs["Detail"].default_value = detail
    nt.links.new(_coords(nt, coords), noise.inputs["Vector"])
    _tinted(nt, _ramp(nt, noise.outputs["Fac"], stops), bsdf)
    bsdf.inputs["Roughness"].default_value = rough
    bsdf.inputs["Metallic"].default_value = metallic
    if bump:
        _bump(nt, noise.outputs["Fac"], bsdf, bump)
    return mat


def mat_wood(name, dark, light, scale=4.0, rough=0.7, bump=0.35):
    """Box UVs stretch noise along each plank's length -> grain for free."""
    mat, nt, bsdf = _principled(name)
    mapping = nt.nodes.new("ShaderNodeMapping")
    mapping.inputs["Scale"].default_value = (scale, scale * 0.35, scale)
    nt.links.new(_coords(nt, "UV"), mapping.inputs["Vector"])
    noise = nt.nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = 3.0
    noise.inputs["Detail"].default_value = 12.0
    noise.inputs["Distortion"].default_value = 0.6
    nt.links.new(mapping.outputs["Vector"], noise.inputs["Vector"])
    col = _ramp(nt, noise.outputs["Fac"], [(0.3, dark), (0.5, tuple((d + l) / 2 for d, l in zip(dark, light))), (0.7, light)])
    _tinted(nt, col, bsdf)
    bsdf.inputs["Roughness"].default_value = rough
    _bump(nt, noise.outputs["Fac"], bsdf, bump, 0.01)
    return mat


def mat_stone(name, c1, c2, mortar, scale=3.2, rough=0.9):
    """Brick texture wrapped around vertical walls: u = x + y, v = z."""
    mat, nt, bsdf = _principled(name)
    sep = nt.nodes.new("ShaderNodeSeparateXYZ")
    nt.links.new(_coords(nt, "Object"), sep.inputs["Vector"])
    add = nt.nodes.new("ShaderNodeMath")
    add.operation = "ADD"
    nt.links.new(sep.outputs["X"], add.inputs[0])
    nt.links.new(sep.outputs["Y"], add.inputs[1])
    comb = nt.nodes.new("ShaderNodeCombineXYZ")
    nt.links.new(add.outputs["Value"], comb.inputs["X"])
    nt.links.new(sep.outputs["Z"], comb.inputs["Y"])
    brick = nt.nodes.new("ShaderNodeTexBrick")
    brick.inputs["Color1"].default_value = (*c1, 1)
    brick.inputs["Color2"].default_value = (*c2, 1)
    brick.inputs["Mortar"].default_value = (*mortar, 1)
    brick.inputs["Scale"].default_value = scale
    brick.inputs["Mortar Size"].default_value = 0.025
    brick.inputs["Mortar Smooth"].default_value = 0.3
    brick.inputs["Brick Width"].default_value = 0.7
    brick.inputs["Row Height"].default_value = 0.3
    brick.inputs["Bias"].default_value = 0.0
    nt.links.new(comb.outputs["Vector"], brick.inputs["Vector"])
    # Break up flat brick colours with a little dirt noise.
    noise = nt.nodes.new("ShaderNodeTexNoise")
    noise.inputs["Scale"].default_value = 9.0
    nt.links.new(_coords(nt, "Object"), noise.inputs["Vector"])
    mix = nt.nodes.new("ShaderNodeVectorMath")
    mix.operation = "MULTIPLY"
    dirt = _ramp(nt, noise.outputs["Fac"], [(0.35, (0.75, 0.73, 0.70)), (0.65, (1.05, 1.03, 1.0))])
    nt.links.new(brick.outputs["Color"], mix.inputs[0])
    nt.links.new(dirt, mix.inputs[1])
    _tinted(nt, mix.outputs["Vector"], bsdf)
    bsdf.inputs["Roughness"].default_value = rough
    _bump(nt, brick.outputs["Fac"], bsdf, 0.6, 0.03, invert=True)
    return mat


def mat_flat(name, color, rough=0.6, metallic=0.0, emission=None, emission_strength=0.0):
    mat, nt, bsdf = _principled(name)
    attr_color = nt.nodes.new("ShaderNodeRGB")
    attr_color.outputs[0].default_value = (*color, 1)
    _tinted(nt, attr_color.outputs[0], bsdf)
    bsdf.inputs["Roughness"].default_value = rough
    bsdf.inputs["Metallic"].default_value = metallic
    if emission:
        bsdf.inputs["Emission Color"].default_value = (*emission, 1)
        bsdf.inputs["Emission Strength"].default_value = emission_strength
    return mat


# ---------------------------------------------------------------- geometry


class Builder:
    """Collects primitives per material into one mesh object each.

    Every primitive can carry a 'tint' (RGB multiplier) stored as a
    face-corner colour attribute; materials multiply their colour by it, so
    hundreds of shingles/stones get individual variation in one object.
    """

    def __init__(self, materials, seed=7):
        self.materials = materials
        self.bms = {}
        self.rng = random.Random(seed)

    def _bm(self, key):
        if key not in self.bms:
            bm = bmesh.new()
            bm.loops.layers.uv.verify()
            bm.loops.layers.float_color.new("tint")
            self.bms[key] = bm
        return self.bms[key]

    def _finish(self, bm, verts, tint, smooth=False):
        layer = bm.loops.layers.float_color["tint"]
        faces = {f for v in verts for f in v.link_faces}
        col = (*tint, 1.0)
        for f in faces:
            # Cylinder caps are single n-gons; only the quad sides go smooth.
            f.smooth = True if smooth == "all" else bool(smooth) and len(f.verts) == 4
            for loop in f.loops:
                loop[layer] = col

    # -- primitives

    def box(self, key, center, size, rot=Matrix.Identity(3), tint=(1, 1, 1)):
        m = Matrix.Translation(Vector(center)) @ rot.to_4x4() @ Matrix.Diagonal((*size, 1))
        bm = self._bm(key)
        verts = bmesh.ops.create_cube(bm, size=1.0, matrix=m, calc_uvs=True)["verts"]
        self._finish(bm, verts, tint)

    def beam(self, key, p1, p2, thickness, depth, normal, tint=(1, 1, 1), extend=None):
        """Rectangular beam from p1 to p2, its depth axis along `normal`."""
        p1, p2, n = Vector(p1), Vector(p2), Vector(normal).normalized()
        d = p2 - p1
        x = d.normalized()
        y = (n - x * n.dot(x)).normalized()
        z = x.cross(y)
        rot = Matrix((x, y, z)).transposed()
        ext = thickness if extend is None else extend
        self.box(key, (p1 + p2) / 2, (d.length + ext, depth, thickness), rot, tint)

    def cylinder(self, key, center, r1, r2, depth, segments=16, rot=Matrix.Identity(3), tint=(1, 1, 1)):
        m = Matrix.Translation(Vector(center)) @ rot.to_4x4()
        bm = self._bm(key)
        verts = bmesh.ops.create_cone(
            bm, cap_ends=True, cap_tris=False, segments=segments,
            radius1=r1, radius2=r2, depth=depth, matrix=m, calc_uvs=True,
        )["verts"]
        self._finish(bm, verts, tint, smooth=True)

    def sphere(self, key, center, radius, scale=(1, 1, 1), segments=12, tint=(1, 1, 1), rot=Matrix.Identity(3)):
        m = Matrix.Translation(Vector(center)) @ rot.to_4x4() @ Matrix.Diagonal((*scale, 1))
        bm = self._bm(key)
        verts = bmesh.ops.create_uvsphere(
            bm, u_segments=segments, v_segments=max(6, segments // 2), radius=radius, matrix=m, calc_uvs=True,
        )["verts"]
        self._finish(bm, verts, tint, smooth="all")

    def prism(self, key, points, offset, tint=(1, 1, 1)):
        """Planar polygon `points` extruded by vector `offset`."""
        bm = self._bm(key)
        off = Vector(offset)
        bottom = [bm.verts.new(Vector(p)) for p in points]
        top = [bm.verts.new(Vector(p) + off) for p in points]
        bm.faces.new(bottom[::-1])
        bm.faces.new(top)
        n = len(points)
        for i in range(n):
            j = (i + 1) % n
            bm.faces.new((bottom[i], bottom[j], top[j], top[i]))
        bmesh.ops.recalc_face_normals(bm, faces=list({f for v in bottom for f in v.link_faces}))
        self._finish(bm, bottom + top, tint)

    def blob_disc(self, key, radius, height, segments=56, wobble=0.14, z_top=0.0, tint=(1, 1, 1)):
        """Irregular flat ground patch, top face at z_top."""
        pts = []
        phase = [self.rng.uniform(0, math.tau) for _ in range(3)]
        for i in range(segments):
            a = math.tau * i / segments
            r = radius * (1 + wobble * (0.55 * math.sin(3 * a + phase[0]) + 0.3 * math.sin(5 * a + phase[1]) + 0.15 * math.sin(9 * a + phase[2])))
            pts.append((math.cos(a) * r, math.sin(a) * r, z_top - height))
        self.prism(key, pts, (0, 0, height), tint)

    # -- output

    def build(self, bevel=0.012, no_bevel=(), outline=0.0, no_outline=()):
        """Turn the collected primitives into objects.

        outline > 0 adds an inverted-hull ink line of that world-space width
        around every part (see outline_material); keys in no_outline skip it.
        """
        ink = outline_material() if outline else None
        objs = []
        for key, bm in self.bms.items():
            mesh = bpy.data.meshes.new(key)
            bm.to_mesh(mesh)
            bm.free()
            obj = bpy.data.objects.new(key, mesh)
            bpy.context.scene.collection.objects.link(obj)
            obj.data.materials.append(self.materials[key])
            if bevel and key not in no_bevel:
                mod = obj.modifiers.new("bevel", "BEVEL")
                mod.width = bevel
                mod.segments = 2
                mod.limit_method = "ANGLE"
                mod.angle_limit = math.radians(40)
            if ink and key not in no_outline:
                obj.data.materials.append(ink)
                mod = obj.modifiers.new("ink", "SOLIDIFY")
                mod.thickness = outline
                mod.offset = 1.0
                mod.use_flip_normals = True
                mod.use_rim = False
                mod.material_offset = 1
            objs.append(obj)
        self.bms.clear()
        return objs
