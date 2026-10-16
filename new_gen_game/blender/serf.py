"""Build a 0.9-unit rigged serf and render transparent eight-direction sheets.

blender -b --factory-startup --python serf.py -- <out.png> [res=256] [samples=32]
The output path is a still preview; serf_{walk,idle,carry}.png go beside it.
Rows: model yaw 0,45,...315 degrees about +Z; neutral facing is -Y.
Columns: frames 1..8 (walk/carry), 1..4 (idle); shared camera/pivot throughout.
Actions include an extra closure key, excluded from exported sprite frames.
Optional --preview-only renders the still without the sheets for quick checks.
"""
import math
import os
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bpy
from mathutils import Matrix, Vector
import lib
import units


def build():
    spec = [('pelvis', (0, 0, .36), (0, 0, .45), None),
            ('spine', (0, 0, .45), (0, 0, .64), 'pelvis'),
            ('head', (0, 0, .64), (0, 0, .86), 'spine'),
            ('load', (.205, .025, .65), (.205, .025, .83), 'spine')]
    for side, sign in [('L', -1), ('R', 1)]:
        x = sign * .087
        spec += [(f'thigh.{side}', (x, 0, .36), (x, 0, .21), 'pelvis'),
                 (f'shin.{side}', (x, 0, .21), (x, 0, .08), f'thigh.{side}'),
                 (f'foot.{side}', (x, 0, .08), (x, -.09, .04), f'shin.{side}'),
                 (f'upper_arm.{side}', (sign * .151, 0, .595), (sign * .18, 0, .455), 'spine'),
                 (f'forearm.{side}', (sign * .18, 0, .455), (sign * .19, 0, .34), f'upper_arm.{side}')]
    rig = units.armature(spec)
    materials = {
        'shirt': lib.mat_noise('ochre_linen', [(.2, (.40, .21, .065)), (.8, (.63, .39, .15))], scale=75, bump=.10),
        'cream': lib.mat_flat('linen_trim', (.73, .58, .35)),
        'pants': lib.mat_flat('umber_trousers', (.18, .095, .047)),
        'boot': lib.mat_flat('worn_leather', (.105, .044, .021)),
        'skin': lib.mat_flat('warm_skin', (.64, .35, .19)),
        'hair': lib.mat_flat('chestnut_hair', (.13, .047, .017)),
        'cap': lib.mat_noise('rust_cap', [(.25, (.23, .064, .027)), (.75, (.38, .13, .049))], scale=60, bump=.08),
        'eye': lib.mat_flat('eyes', (.025, .013, .007)),
        'gold': lib.mat_flat('brass_buckle', (.57, .33, .09), metallic=.55),
        'burlap': lib.mat_noise('sack_canvas', [(.3, (.36, .26, .13)), (.7, (.55, .43, .24))], scale=65, bump=.15),
    }
    def part(bone, draw):
        builder = lib.Builder(materials)
        draw(builder)
        return units.bind(builder, rig, bone)
    def torso(b):
        b.sphere('shirt', (0, 0, .51), .15, scale=(1, .70, 1.01), segments=12)
        b.cylinder('shirt', (0, 0, .405), .152, .125, .09, segments=10)
        b.cylinder('boot', (0, 0, .442), .134, .134, .032, segments=12)
        b.box('gold', (0, -.133, .443), (.039, .015, .032))
        b.box('boot', (0, -.143, .443), (.020, .007, .016))
        b.sphere('cream', (0, -.005, .623), .060, scale=(1, .8, .32), segments=10)
        for z in (.575, .545):
            b.sphere('boot', (0, -.105, z), .009, segments=8)
    part('spine', torso)
    part('pelvis', lambda b: b.sphere('pants', (0, 0, .359), .112, scale=(1.17, .73, .65), segments=10))
    def head(b):
        b.cylinder('skin', (0, 0, .646), .042, .043, .055, segments=10)
        b.sphere('skin', (0, -.009, .745), .121, scale=(.96, .84, 1.03), segments=12)
        b.sphere('hair', (0, .02, .779), .122, scale=(1.02, .85, .80), segments=12)
        # The hair cap leaves the lower face and eyes unobstructed.
        b.sphere('skin', (0, -.038, .724), .094, scale=(1.02, .81, .85), segments=12)
        for s in (-1, 1):
            b.sphere('skin', (s * .118, -.006, .741), .026, scale=(.55, .7, 1), segments=8)
            b.sphere('eye', (s * .041, -.108, .752), .009, scale=(1, .5, 1.18), segments=8)
            b.box('hair', (s * .041, -.104, .773), (.029, .008, .007))
        b.sphere('skin', (0, -.121, .731), .024, scale=(.7, 1, .7), segments=8)
        b.box('hair', (0, -.106, .698), (.028, .006, .005))
        b.sphere('cap', (0, .007, .839), .127, scale=(1.08, .92, .44), segments=12)
        b.sphere('cap', (-.019, .019, .872), .097, scale=(1.13, .98, .31), segments=12)
        b.sphere('boot', (0, -.068, .823), .125, scale=(1.02, .67, .10), segments=12)
    part('head', head)
    for side, s in [('L', -1), ('R', 1)]:
        x = s * .087
        part(f'thigh.{side}', lambda b, x=x: b.sphere('pants', (x, 0, .286), .071, scale=(.88, .94, 1.30), segments=10))
        part(f'shin.{side}', lambda b, x=x: b.cylinder('boot', (x, 0, .147), .047, .056, .147, segments=10))
        def foot(b, x=x):
            b.sphere('boot', (x, -.029, .049), .057, scale=(.94, 1.48, .77), segments=10)
            b.box('boot', (x, -.027, .013), (.106, .157, .023))
        part(f'foot.{side}', foot)
        part(f'upper_arm.{side}', lambda b, s=s: b.sphere('shirt', (s * .163, 0, .548), .064, scale=(.91, .92, 1.10), segments=10))
        def forearm(b, s=s):
            b.sphere('skin', (s * .183, 0, .438), .038, scale=(.93, 1, 1.45), segments=10)
            b.sphere('skin', (s * .191, -.002, .355), .043, scale=(.85, .83, 1.08), segments=10)
            b.cylinder('cream', (s * .176, 0, .476), .048, .048, .031, segments=10)
        part(f'forearm.{side}', forearm)
    def load(b):
        b.sphere('burlap', (.215, .042, .726), .126, scale=(.86, 1.14, 1.15), segments=12)
        b.cylinder('burlap', (.215, -.066, .805), .027, .047, .060, segments=8, rot=Matrix.Rotation(.6, 3, 'X'))
        b.cylinder('boot', (.215, -.055, .786), .030, .030, .012, segments=8)
    for obj in part('load', load):
        # Hide the outline as well as the mesh when the sack is collapsed.
        for prop in ('hide_render', 'hide_viewport'):
            driver = obj.driver_add(prop).driver
            variable = driver.variables.new()
            variable.name = 'visible_scale'
            variable.type = 'SINGLE_PROP'
            variable.targets[0].id = rig
            variable.targets[0].data_path = 'pose.bones["load"].scale[0]'
            driver.expression = 'visible_scale < 0.5'
    return rig


def animate(rig):
    def pose(t, mode):
        carrying = mode == 'carry'
        moving = mode != 'idle'
        # A hidden bone scale is stored IN each Action so choosing carry in
        # Blender also reveals the sack, without an external frame handler.
        rig.pose.bones['load'].scale = (1, 1, 1) if carrying else (.00001,) * 3
        bob = .004 * math.cos(2 * t) if moving else .002 * math.sin(t)
        rig.pose.bones['pelvis'].location.y = bob
        rig.pose.bones['spine'].scale = (1, 1 + .008 * math.sin(t), 1)
        units.rotate(rig, 'spine', -.045 if carrying else .012 * math.sin(t), 'Y')
        for side, phase in [('L', t), ('R', t + math.pi)]:
            y = .078 * math.cos(phase) if moving else 0
            lift = .037 * max(0, math.sin(phase)) if moving else 0
            down = .36 + bob - (.084 + lift)
            d = min(math.hypot(y, down), .2799)
            a = math.atan2(y, down) - math.acos(max(-1, min(1, (.15**2 + d*d - .13**2) / (.30*d))))
            knee = math.pi - math.acos(max(-1, min(1, (.15**2 + .13**2 - d*d) / (.039))))
            units.rotate(rig, f'thigh.{side}', a)
            units.rotate(rig, f'shin.{side}', knee)
            units.rotate(rig, f'foot.{side}', -a - knee)
            swing = -.36 * math.cos(phase) if moving else .02 * math.sin(t)
            units.rotate(rig, f'upper_arm.{side}', -2.25 if carrying and side == 'R' else swing)
            units.rotate(rig, f'forearm.{side}', -.40 if carrying and side == 'R' else -.13)
    return {name: units.make_action(rig, name, count, lambda t, name=name: pose(t, name))
            for name, count in [('walk', 8), ('idle', 4), ('carry', 8)]}


def main():
    args = sys.argv[sys.argv.index('--') + 1:] if '--' in sys.argv else []
    here = Path(__file__).resolve().parent
    out = Path(args[0]).resolve() if args else here.parent / 'renders' / 'serf_preview.png'
    res = int(args[1]) if len(args) > 1 else 256
    samples = int(args[2]) if len(args) > 2 else 32
    if res < 32 or samples < 1:
        raise ValueError('Resolution must be >=32 and samples >=1')
    out.parent.mkdir(parents=True, exist_ok=True)
    if out.name in ('serf_walk.png', 'serf_idle.png', 'serf_carry.png'):
        out = out.with_name('serf_preview.png')
    lib.clear_scene()
    lib.setup_render(res, samples)
    bpy.context.scene.render.resolution_percentage = 100
    bpy.context.scene.render.fps = 12
    lib.setup_world()
    lib.add_sun()
    rig = build()
    actions = animate(rig)
    camera = lib.add_game_camera()
    camera.location += Vector((0, 0, .44))
    camera.data.ortho_scale = 1.13
    scene = bpy.context.scene
    scene['sprite_layout'] = 'Rows: yaw 0..315 step45; columns: frame1..N; front=-Y'
    scene['sprite_cell'] = res
    scene['sprite_camera_scale'] = 1.13
    scene['sprite_origin'] = 'Shared world origin at feet; camera target Z=0.44'
    rig.animation_data.action = actions['idle']
    scene.frame_start, scene.frame_end = 1, 4
    scene.frame_set(1)
    lib.render(str(out))
    if '--preview-only' not in args:
        with tempfile.TemporaryDirectory(prefix='serf_frames_') as tmp:
            for name, action in actions.items():
                rig.animation_data.action = action
                count = int(action['sprite_frames'])
                frames = []
                for direction in range(8):
                    rig.rotation_euler.z = math.radians(direction * 45)
                    for frame in range(1, count + 1):
                        scene.frame_set(frame)
                        path = Path(tmp) / f'{name}_{direction}_{frame}.png'
                        lib.render(str(path))
                        frames.append(path)
                    print(f'[serf] {name}: direction {direction + 1}/8', flush=True)
                units.write_sheet(out.parent / f'serf_{name}.png', frames, count, res)
    rig.rotation_euler.z = 0
    rig.animation_data.action = actions['idle']
    scene.frame_set(1)
    scene.render.filepath = str(out)
    bpy.ops.object.select_all(action='DESELECT')
    rig.select_set(True)
    bpy.context.view_layer.objects.active = rig
    for screen in bpy.data.screens:
        for area in screen.areas:
            if area.type == 'VIEW_3D':
                area.spaces.active.region_3d.view_distance = 1.8
                area.spaces.active.region_3d.view_location = (0, 0, .44)
    lib.save_outputs(str(here.parent / 'models'), 'serf')
    print('[serf] finished', flush=True)


if __name__ == '__main__':
    main()
