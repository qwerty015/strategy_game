"""Rigid low-poly skinning and dependency-free sprite-sheet output for Blender."""
import math
import struct
import zlib
from pathlib import Path

import bpy
from mathutils import Quaternion, Vector

import lib


def armature(spec):
    data = bpy.data.armatures.new('SerfSkeleton')
    rig = bpy.data.objects.new('Serf', data)
    bpy.context.collection.objects.link(rig)
    bpy.context.view_layer.objects.active = rig
    rig.select_set(True)
    bpy.ops.object.mode_set(mode='EDIT')
    for name, head, tail, parent in spec:
        bone = data.edit_bones.new(name)
        bone.head, bone.tail = head, tail
        if parent:
            bone.parent = data.edit_bones[parent]
    bpy.ops.object.mode_set(mode='OBJECT')
    rig.show_in_front = True
    data.display_type = 'OCTAHEDRAL'
    rig.animation_data_create()
    return rig


def bind(builder, rig, bone):
    objects = builder.build(bevel=0.003, outline=0.0015)
    for obj in objects:
        obj.name = bone + '_' + obj.name
        obj.parent = rig
        group = obj.vertex_groups.new(name=bone)
        group.add(list(range(len(obj.data.vertices))), 1.0, 'REPLACE')
        mod = obj.modifiers.new('Skeleton', 'ARMATURE')
        mod.object = rig
        # Skin first; the silhouette shell follows the animated mesh.
        bpy.context.view_layer.objects.active = obj
        bpy.ops.object.modifier_move_up(modifier=mod.name)
        bpy.ops.object.modifier_move_up(modifier=mod.name)
    return objects


def rotate(rig, name, angle, axis='X'):
    bone = rig.pose.bones[name]
    basis = bone.bone.matrix_local.to_quaternion()
    bone.rotation_mode = 'QUATERNION'
    bone.rotation_quaternion = basis.inverted() @ Quaternion(axis_vector(axis), angle) @ basis


def axis_vector(axis):
    return Vector({'X': (1, 0, 0), 'Y': (0, 1, 0), 'Z': (0, 0, 1)}[axis])


def curves(action):
    # Blender 4.4+ layered actions (including 5.x), with legacy fallback.
    if hasattr(action, 'layers') and len(action.layers):
        for layer in action.layers:
            for strip in layer.strips:
                for bag in strip.channelbags:
                    yield from bag.fcurves
    else:
        yield from action.fcurves


def make_action(rig, name, count, pose):
    action = bpy.data.actions.new(name)
    action.use_fake_user = True
    rig.animation_data.action = action
    for frame in range(1, count + 2):
        for bone in rig.pose.bones:
            bone.location = (0, 0, 0)
            bone.scale = (1, 1, 1)
            bone.rotation_mode = 'QUATERNION'
            bone.rotation_quaternion = (1, 0, 0, 0)
        pose((frame - 1) / count * math.tau)
        for bone in rig.pose.bones:
            for prop in ('location', 'rotation_quaternion', 'scale'):
                bone.keyframe_insert(prop, frame=frame, group=bone.name)
    for curve in curves(action):
        for key in curve.keyframe_points:
            key.interpolation = 'LINEAR'
        curve.modifiers.new('CYCLES')
    action.use_frame_range = True
    action.frame_start, action.frame_end = 1, count
    action['sprite_frames'] = count
    action['loop_closure_frame'] = count + 1
    return action


def write_sheet(path, frames, columns, cell):
    """Combine Blender-decoded, display-space PNG bytes, top row = direction 0.

    Standard-library PNG writer avoids Pillow and a second colour transform.
    Blender pixel buffers are bottom-up; PNG scanlines are top-down.
    """
    width, height = columns * cell, 8 * cell
    rows = [bytearray(width * 4) for _ in range(height)]
    for direction in range(8):
        for frame in range(columns):
            image = bpy.data.images.load(str(frames[direction * columns + frame]), check_existing=False)
            assert tuple(image.size) == (cell, cell)
            # PNG files load as sRGB; raw pixels retain encoded values.
            pixels = list(image.pixels[:])
            for y in range(cell):
                source = (cell - y - 1) * cell * 4
                start = frame * cell * 4
                rows[direction * cell + y][start:start + cell * 4] = bytes(
                    max(0, min(255, round(v * 255))) for v in pixels[source:source + cell * 4])
            bpy.data.images.remove(image)
    def chunk(kind, data):
        return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind + data) & 0xffffffff)
    payload = b''.join(b'\0' + row for row in rows)
    Path(path).write_bytes(b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 6, 0, 0, 0))
                           + chunk(b'sRGB', b'\0') + chunk(b'IDAT', zlib.compress(payload, 6)) + chunk(b'IEND', b''))
