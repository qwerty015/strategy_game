"""Export a saved scene to FBX for Unity, with procedural materials baked.

    blender -b new_gen_game/models/<name>.blend --python new_gen_game/blender/export_unity.py -- <out.fbx> [max_texture]

Unity can't run Blender's node materials, so every mesh gets its own UV
layout (Smart UV Project) and its material colour -- noise, wood grain,
per-tile tint and all -- is baked into a texture (DIFFUSE, colour pass only:
lighting is Unity's job). Inverted-hull ink is dropped: it exists for the 2D
sprites. Ground disc, smoke puffs, camera and lights are removed; markers
for things Unity animates itself are added as empties:
  marker_smoke   where chimney smoke starts (bakery, smeltery)
  shaft_base     with sail_axis/sail_spin gives the mill's rotation axis
Armatures and their actions (units) are exported as animation takes.
"""

import math
import os
import sys

import bpy
from mathutils import Vector

args = sys.argv[sys.argv.index("--") + 1:]
OUT = os.path.abspath(args[0])
MAX_TEX = int(args[1]) if len(args) > 1 else 1024
TEX_DIR = os.path.join(os.path.dirname(OUT), "textures")
os.makedirs(TEX_DIR, exist_ok=True)
NAME = os.path.splitext(os.path.basename(OUT))[0]
GLOW = ("glow", "fire", "molten", "glow_soft")

scene = bpy.context.scene
scene.render.engine = "CYCLES"
scene.cycles.samples = 4
scene.cycles.use_denoising = False
scene.render.bake.use_pass_direct = False
scene.render.bake.use_pass_indirect = False
scene.render.bake.use_pass_color = True
scene.render.bake.margin = 6

# ---------------------------------------------------------------- markers
scene.frame_set(1)
smoke = [o for o in scene.objects if o.name.startswith("smoke_")]
if smoke:
    origin = min(smoke, key=lambda o: o.matrix_world.translation.z).matrix_world.translation.copy()
    first = bpy.data.objects.get("smoke_0")
    if first is not None:
        origin = first.matrix_world.translation.copy()
    marker = bpy.data.objects.new("marker_smoke", None)
    scene.collection.objects.link(marker)
    marker.location = origin
if "sail_axis" in bpy.data.objects:
    rig = bpy.data.objects["sail_axis"]
    axis = rig.matrix_world.to_quaternion() @ Vector((0, 0, 1))
    base = bpy.data.objects.new("shaft_base", None)
    scene.collection.objects.link(base)
    base.location = rig.matrix_world.translation - axis * 1.0

for o in list(scene.objects):
    if o.type in {"CAMERA", "LIGHT"} or o.name.startswith("smoke_") or o.name.split(".")[0] == "dirt":
        bpy.data.objects.remove(o, do_unlink=True)
for m in [m for m in bpy.data.materials if m.name.startswith("smoke")]:
    bpy.data.materials.remove(m)

for arm in [o for o in scene.objects if o.type == "ARMATURE"]:
    arm.data.pose_position = "REST"        # bake on the undeformed mesh

# ---------------------------------------------------------------- bake


def area(obj):
    return sum(p.area for p in obj.data.polygons) * max(obj.matrix_world.to_scale()) ** 2


# Units toggle their load/tool per animation by hiding objects; an Unity
# pilot only needs the default look, so hidden parts are simply dropped.
for o in [o for o in scene.objects if o.type == "MESH" and (o.hide_get() or o.hide_viewport or o.hide_render)]:
    bpy.data.objects.remove(o, do_unlink=True)
meshes = [o for o in scene.objects if o.type == "MESH"]
for obj in meshes:
    for mod in [m for m in obj.modifiers if m.type == "SOLIDIFY"]:
        obj.modifiers.remove(mod)
    mats = obj.data.materials
    for i in reversed(range(len(mats))):
        if mats[i] is not None and mats[i].name == "ink":
            mats.pop(index=i)
    if not len(mats):
        continue
    if obj.data.users > 1:
        obj.data = obj.data.copy()

    bpy.ops.object.select_all(action="DESELECT")
    obj.select_set(True)
    bpy.context.view_layer.objects.active = obj
    while len(obj.data.uv_layers):
        obj.data.uv_layers.remove(obj.data.uv_layers[0])
    obj.data.uv_layers.new(name="UVMap")
    bpy.ops.object.mode_set(mode="EDIT")
    bpy.ops.mesh.select_all(action="SELECT")
    bpy.ops.uv.smart_project(angle_limit=math.radians(66), island_margin=0.004)
    bpy.ops.object.mode_set(mode="OBJECT")

    a = area(obj)
    size = 256 if a < 0.5 else 512 if a < 3 else 1024 if a < 12 else 2048
    size = min(size, MAX_TEX)
    tex_name = f"{NAME}_{obj.name.replace('.', '_')}"
    img = bpy.data.images.new(tex_name, size, size, alpha=False)
    nodes = []
    for m in obj.data.materials:
        node = m.node_tree.nodes.new("ShaderNodeTexImage")
        node.image = img
        m.node_tree.nodes.active = node
        nodes.append((m, node))
    bpy.ops.object.bake(type="DIFFUSE")
    img.filepath_raw = os.path.join(TEX_DIR, tex_name + ".png")
    img.file_format = "PNG"
    img.save()

    glow = any(m.name.split(".")[0] in GLOW for m in obj.data.materials)
    rough = obj.data.materials[0].node_tree.nodes.get("Principled BSDF")
    rough = rough.inputs["Roughness"].default_value if rough else 0.8
    baked = bpy.data.materials.new(tex_name + ("_glow" if glow else ""))
    baked.use_nodes = True
    nt = baked.node_tree
    nt.nodes.clear()                      # Blender 5 may start without the default pair
    bsdf = nt.nodes.new("ShaderNodeBsdfPrincipled")
    nt.links.new(bsdf.outputs[0], nt.nodes.new("ShaderNodeOutputMaterial").inputs["Surface"])
    tex = baked.node_tree.nodes.new("ShaderNodeTexImage")
    tex.image = img
    baked.node_tree.links.new(tex.outputs["Color"], bsdf.inputs["Base Color"])
    bsdf.inputs["Roughness"].default_value = rough
    if glow:
        baked.node_tree.links.new(tex.outputs["Color"], bsdf.inputs["Emission Color"])
        bsdf.inputs["Emission Strength"].default_value = 3.0
    obj.data.materials.clear()
    obj.data.materials.append(baked)
    for p in obj.data.polygons:
        p.material_index = 0
    print(f"[export_unity] baked {obj.name} -> {size}px")

for arm in [o for o in scene.objects if o.type == "ARMATURE"]:
    arm.data.pose_position = "POSE"

# ---------------------------------------------------------------- export
has_rig = any(o.type == "ARMATURE" for o in scene.objects)
bpy.ops.export_scene.fbx(
    filepath=OUT,
    use_selection=False,
    object_types={"MESH", "ARMATURE", "EMPTY"},
    use_mesh_modifiers=True,
    apply_scale_options="FBX_SCALE_ALL",
    add_leaf_bones=False,
    bake_anim=has_rig,
    bake_anim_use_all_actions=has_rig,
    bake_anim_use_nla_strips=False,
    path_mode="RELATIVE",
    embed_textures=False,
)
print("[export_unity] wrote", OUT)
