"""Rigid low-poly skinning and dependency-free sprite-sheet output for Blender."""
import math
import os
import sys
import tempfile
from collections import Counter
import struct
import zlib
from pathlib import Path

import bpy
from mathutils import Matrix, Quaternion, Vector

import lib

# All directions/actions share this projection. The root stays at the feet.
CAMERA_SCALE = 1.18
CAMERA_HEIGHT = .44


def extra_materials():
    return {
        'wood': lib.mat_flat('tool_wood', (.30,.15,.055)),
        'endgrain': lib.mat_flat('cut_endgrain', (.68,.46,.22)),
        'steel': lib.mat_flat('tool_steel', (.25,.30,.32), metallic=.55),
        'apron': lib.mat_flat('apron', (.28,.30,.31)),
        'straw': lib.mat_noise('straw', [(.2,(.45,.29,.08)),(.8,(.78,.58,.22))], scale=70, bump=.08),
        'lamp': lib.mat_flat('lamp', (1,.74,.24), emission=(1,.56,.12), emission_strength=.7),
        'line': lib.mat_flat('fishing_line', (.72,.66,.46)),
    }


def add_clothing(b, profile):
    apron = profile.get('apron')
    if apron:
        key = 'boot' if apron == 'leather' else 'apron'
        b.prism(key, [(-.091,-.119,.59),(.091,-.119,.59),(.113,-.127,.347),(-.113,-.127,.347)], (0,.016,0))
        for x in (-.065,.065):
            b.box(key,(x,-.078,.611),(.025,.017,.068),Matrix.Rotation(-.55,3,'X'))
        b.box('boot',(0,-.144,.44),(.219,.020,.025))
    if profile['name'] == 'lumberjack':
        b.box('boot',(0,-.108,.532),(.026,.018,.164))
        for z in (.51,.55,.59):
            b.box('cream',(0,-.122,z),(.052,.006,.009))


def add_hat(b, profile):
    hat = profile['hat']
    if hat == 'hood':
        b.sphere('shirt',(0,.035,.792),.133,scale=(1.03,.83,.83),segments=12)
        # Hood rim is behind the face: the face remains visible from the front.
        b.sphere('cream',(0,-.008,.642),.075,scale=(1.05,.8,.28),segments=10)
        b.sphere('shirt',(0,.083,.643),.09,scale=(1.05,.75,.65),segments=10)
    elif hat in ('straw','wide'):
        key = 'straw' if hat == 'straw' else 'cap'
        b.cylinder(key,(0,.006,.830),.166,.164,.018,segments=16)
        b.cylinder(key,(0,.006,.862),.098,.074,.065,segments=12)
        b.cylinder('boot',(0,.006,.843),.097,.093,.017,segments=12)
    elif hat == 'helmet':
        b.sphere('cap',(0,.012,.833),.126,scale=(1.03,.89,.55),segments=12)
        b.cylinder('steel',(0,.01,.814),.127,.127,.024,segments=12)
        b.cylinder('boot',(0,-.116,.841),.035,.035,.025,segments=10,rot=Matrix.Rotation(math.pi/2,3,'X'))
        b.cylinder('lamp',(0,-.134,.841),.024,.024,.009,segments=10,rot=Matrix.Rotation(math.pi/2,3,'X'))
    else:
        b.sphere('cap',(0,.012,.837),.110,scale=(1.03,.85,.38),segments=12)
        b.sphere('cap',(-.012,.016,.859),.085,scale=(1.03,.85,.26),segments=12)
        b.sphere('boot',(0,-.059,.818),.108,scale=(1.02,.68,.10),segments=12)


def add_load(b, profile):
    kind = profile['load']
    if kind == 'log':
        r = Matrix.Rotation(math.pi/2,3,'X')
        b.cylinder('wood',(.215,.02,.721),.085,.077,.48,segments=10,rot=r)
        for y in (-.224,.264):
            b.cylinder('endgrain',(.215,y,.721),.067,.067,.006,segments=10,rot=r)
            b.cylinder('wood',(.215,y,.721),.029,.029,.008,segments=10,rot=r)
    elif kind in ('stone','ore'):
        b.box('burlap',(.215,.025,.696),(.19,.24,.11))
        for x,y,z,radius in ((.18,-.045,.77,.065),(.25,.06,.77,.073),(.21,.02,.825,.055)):
            b.sphere('steel' if kind=='ore' else 'apron',(x,y,z),radius,scale=(1,.8,.8),segments=6)
    elif kind == 'planks':
        for x,z in ((.18,.697),(.25,.697),(.215,.747)):
            b.box('endgrain',(x,.02,z),(.065,.47,.045))
        b.box('boot',(.215,.02,.727),(.158,.035,.09))
    elif kind == 'fish':
        b.cylinder('wood',(.22,.01,.708),.075,.10,.16,segments=10)
        for x in (.18,.24):
            b.sphere('steel',(x,-.014,.795),.054,scale=(.48,.7,1.2),segments=8)
            b.prism('steel',[(x-.025,-.02,.826),(x+.025,-.02,.826),(x,-.02,.79)],(0,.015,0))
    else:
        b.sphere('burlap',(.215,.042,.726),.126,scale=(.86,1.14,1.15),segments=12)
        b.cylinder('burlap',(.215,-.066,.805),.027,.047,.060,segments=8,rot=Matrix.Rotation(.6,3,'X'))
        b.cylinder('boot',(.215,-.055,.786),.030,.030,.012,segments=8)


def add_tool(b, profile):
    """Tools share a hand grip; the blade lies along the swinging arm's plane."""
    kind = profile['tool']
    grip = Vector((.19,0,.355))
    length = .31 if kind != 'rod' else .39
    b.beam('wood',grip+Vector((0,.065,0)),grip+Vector((0,-length,0)),.018,.018,(1,0,0),extend=0)
    end = grip+Vector((0,-length,0))
    if kind == 'axe':
        b.prism('steel',[(.178,-.21,.365),(.178,-.34,.367),(.178,-.36,.277),(.178,-.23,.296)],(.024,0,0))
        b.box('boot',(.19,-.235,.355),(.031,.045,.035))
    elif kind == 'pick':
        b.prism('steel',[(.178,-.28,.47),(.178,-.315,.405),(.178,-.328,.355),(.178,-.315,.305),(.178,-.26,.245),(.178,-.295,.34)],(.024,0,0))
    elif kind == 'hammer':
        b.box('steel',end,(.075,.057,.103))
    elif kind == 'sickle':
        b.prism('steel',[(.178,-.25,.355),(.178,-.33,.34),(.178,-.385,.39),(.178,-.38,.47),(.178,-.35,.495),(.178,-.355,.416),(.178,-.31,.38)],(.018,0,0))
    elif kind == 'rod':
        b.beam('wood',end,end+Vector((0,-.08,.035)),.009,.009,(1,0,0),extend=0)
        tip=end+Vector((0,-.08,.035))
        b.beam('line',tip,tip+Vector((0,.06,-.13)),.003,.003,(1,0,0),extend=0)
        b.sphere('cap',tip+Vector((0,.06,-.13)),.014,scale=(.7,.7,1.4),segments=8)


def translation(rig, name, world):
    rig.pose.bones[name].location = rig.data.bones[name].matrix_local.to_quaternion().inverted() @ Vector(world)


def ground(rig):
    """Ground the lowest boot/body point; evaluated geometry includes the ink shell."""
    bpy.context.view_layer.update()
    graph = bpy.context.evaluated_depsgraph_get()
    low = 10
    for obj in rig.children:
        if obj.type != 'MESH' or obj.hide_render:
            continue
        evaluated = obj.evaluated_get(graph)
        mesh = evaluated.to_mesh()
        low = min(low, min((evaluated.matrix_world @ v.co).z for v in mesh.vertices))
        evaluated.to_mesh_clear()
    root = rig.pose.bones['root']
    root.location += rig.data.bones['root'].matrix_local.to_quaternion().inverted() @ Vector((0,0,.006-low))


def animate_character(rig, profile):
    def pose(t, mode):
        moving = mode in ('walk','carry')
        carrying = mode == 'carry'
        working = mode == 'work'
        rig.pose.bones['load'].scale = (1,1,1) if carrying else (.00001,)*3
        if 'tool' in rig.pose.bones:
            rig.pose.bones['tool'].scale = (.00001,)*3 if carrying else (1,1,1)
        bob = -.018*math.cos(2*t) if moving else .002*math.sin(t)
        translation(rig,'pelvis',(0,0,bob))
        rotate(rig,'spine',.10 if moving else .02,'X')
        rig.pose.bones['spine'].scale = (1,1+.010*math.sin(t),1)
        for side, phase in [('L',t),('R',t+math.pi)]:
            thigh = .58*math.cos(phase) if moving else -.06
            knee = .10 + .70*max(0,math.sin(phase)) if moving else .12
            rotate(rig,f'thigh.{side}',thigh)
            rotate(rig,f'shin.{side}',knee)
            rotate(rig,f'foot.{side}',-thigh-knee)
            swing = -.49*math.cos(phase) if moving else .025*math.sin(t)
            rotate(rig,f'upper_arm.{side}',-2.25 if carrying and side=='R' else swing)
            rotate(rig,f'forearm.{side}',-.40 if carrying and side=='R' else -.10)
        if working:
            # Raised anticipation, fast strike, held impact and slower recovery.
            key = int(round(t/math.tau*8)) % 8
            strokes = [-100,-135,-145,-100,-20,-18,-48,-75]
            arm = math.radians(strokes[key])
            if profile['tool'] == 'rod':
                arm = math.radians([-70,-100,-135,-125,-35,-30,-42,-55][key])
            if profile['tool'] == 'sickle':
                arm = math.radians([-35,-45,-55,-45,-30,-25,-30,-33][key])
                rotate(rig,'spine',math.radians([-18,-10,5,20,25,15,0,-12][key]),'Z')
            else:
                rotate(rig,'spine',.11 + (.13 if key in (4,5) else 0),'X')
            rotate(rig,'upper_arm.R',arm)
            rotate(rig,'forearm.R',-.20)
            rotate(rig,'upper_arm.L',.20 if key in (4,5) else -.35)
            rotate(rig,'forearm.L',-.5)
        ground(rig)
    actions = {name: make_action(rig,name,count,lambda t,name=name:pose(t,name))
               for name,count in [('walk',8),('idle',4),('carry',8)]}
    if profile.get('tool'):
        actions['work'] = make_action(rig,'work',8,lambda t:pose(t,'work'))
    def death(p):
        rig.pose.bones['load'].scale=(.00001,)*3
        if 'tool' in rig.pose.bones:
            rig.pose.bones['tool'].scale=(.00001,)*3
        translation(rig,'pelvis',(0,0,-.08*min(1,p*3)))
        rotate(rig,'spine',.35*math.sin(p*math.pi))
        rotate(rig,'root',p*math.pi/2,'Y')
        translation(rig,'root',(-.32*p,0,0))
        for side in ('L','R'):
            rotate(rig,'thigh.'+side,-.30*math.sin(p*math.pi))
            rotate(rig,'shin.'+side,.65*math.sin(p*math.pi))
            rotate(rig,'upper_arm.'+side,-.45*p)
            rotate(rig,'forearm.'+side,-.4*p)
        ground(rig)
    actions['death']=make_action(rig,'death',4,death,loop=False)
    return actions


def write_gif(path, paths, size=64, fps=10, loop=True):
    """Grass-backed GIF89a, using a shared adaptive palette and 9-bit LZW blocks.

    No Pillow installation in Blender is required. Clear codes every 200
    pixels keep the dictionary below 512 entries and code width at 9 bits.
    """
    frames=[]
    histogram=Counter()
    for frame_path in paths:
        image=bpy.data.images.load(str(frame_path),check_existing=False)
        image.scale(size,size)
        pixels=list(image.pixels[:])
        colors=[]
        for y in reversed(range(size)):
            for x in range(size):
                i=(y*size+x)*4
                alpha=pixels[i+3]
                rgb=[pixels[i+k]*alpha+(58,92,52)[k]/255*(1-alpha) for k in range(3)]
                color=tuple(max(0,min(31,round(c*31))) for c in rgb)
                colors.append(color)
                histogram[color]+=1
        bpy.data.images.remove(image)
        frames.append(colors)
    # Weighted median-cut over the whole animation prevents palette flicker.
    boxes=[list(histogram)]
    while len(boxes)<256:
        candidates=[(sum(histogram[c] for c in box)*max(max(c[k] for c in box)-min(c[k] for c in box) for k in range(3)),i)
                    for i,box in enumerate(boxes) if len(box)>1]
        if not candidates:
            break
        _,i=max(candidates)
        box=boxes.pop(i)
        axis=max(range(3),key=lambda k:max(c[k] for c in box)-min(c[k] for c in box))
        box.sort(key=lambda c:c[axis])
        half=sum(histogram[c] for c in box)/2
        running=0
        split=1
        for j,c in enumerate(box[:-1]):
            running+=histogram[c]
            split=j+1
            if running>=half:
                break
        boxes.extend((box[:split],box[split:]))
    palette=[]
    lookup={}
    for i,box in enumerate(boxes):
        total=sum(histogram[c] for c in box)
        palette.extend(round(sum(c[k]*histogram[c] for c in box)/total*255/31) for k in range(3))
        lookup.update((c,i) for c in box)
    palette.extend([0]*(768-len(palette)))
    payload=bytearray(b'GIF89a'+struct.pack('<HHBBB',size,size,0xf7,0,0))+bytearray(palette)
    if loop:
        payload.extend(b'!\xff\x0bNETSCAPE2.0\x03\x01\0\0\0')
    for colors in frames:
        indices=[lookup[c] for c in colors]
        codes=[]
        for i in range(0,len(indices),200):
            codes.append(256)
            codes.extend(indices[i:i+200])
        codes.append(257)
        packed=bytearray()
        acc=bits=0
        for code in codes:
            acc|=code<<bits
            bits+=9
            while bits>=8:
                packed.append(acc&255)
                acc>>=8
                bits-=8
        if bits:
            packed.append(acc&255)
        delay=round(100/fps)
        payload.extend(b'!\xf9\x04\x04'+struct.pack('<H',delay)+b'\0\0')
        payload.extend(b','+struct.pack('<HHHHB',0,0,size,size,0)+b'\x08')
        for i in range(0,len(packed),255):
            block=packed[i:i+255]
            payload.append(len(block))
            payload.extend(block)
        payload.append(0)
    payload.extend(b';')
    Path(path).write_bytes(payload)


def setup(profile, res, samples):
    lib.clear_scene()
    lib.setup_render(res,samples)
    scene=bpy.context.scene
    scene.render.resolution_percentage=100
    scene.render.use_persistent_data=True
    scene.render.fps=10
    scene.cycles.max_bounces=4
    scene.cycles.transparent_max_bounces=8
    lib.setup_world()
    lib.add_sun()
    rig=build_character(profile)
    actions=animate_character(rig,profile)
    camera=lib.add_game_camera()
    # Workers need fixed extra headroom for overhead tools; never reframe a
    # direction/action/frame. The serf retains a ~75%-height idle silhouette.
    height=profile.get('camera_height', .48 if profile.get('tool') else CAMERA_HEIGHT)
    scale=profile.get('camera_scale', 1.32 if profile.get('tool') else CAMERA_SCALE)
    camera.location+=Vector((0,0,height))
    camera.data.ortho_scale=scale
    scene['sprite_layout']='Rows: model yaw 0..315 step45, front=-Y; columns: frame1..N'
    scene['sprite_cell']=res
    scene['sprite_camera_scale']=scale
    scene['sprite_origin']=f'World feet=(0,0,0); fixed camera target Z={height}'
    scene['unit']=profile['name']
    return rig,actions


def run(profile):
    """CLI: <out.png> [resolution=256] [samples=32] [--preview-only].

    --quick renders direction zero of each action plus 64px grass GIFs,
    for visual review before the full eight-direction render.
    --actions=walk,work limits rendering (the blend still contains all actions).
    Sheets have transparent RGBA cells. Loop closure keys are not exported;
    death has four distinct keys and holds its final pose, without CYCLES.
    """
    args=sys.argv[sys.argv.index('--')+1:] if '--' in sys.argv else []
    here=Path(__file__).resolve().parent
    name=profile['name']
    out=Path(args[0]).resolve() if args else here.parent/'renders'/f'{name}_preview.png'
    res=int(args[1]) if len(args)>1 else 256
    samples=int(args[2]) if len(args)>2 else 32
    if res<32 or samples<1:
        raise ValueError('Resolution must be >=32 and samples >=1')
    out.parent.mkdir(parents=True,exist_ok=True)
    if out.name in [f'{name}_{a}.png' for a in ('walk','idle','carry','work','death')]:
        out=out.with_name(f'{name}_preview.png')
    rig,actions=setup(profile,res,samples)
    scene=bpy.context.scene
    rig.animation_data.action=actions['idle']
    scene.frame_set(1)
    lib.render(str(out))
    selected=next((a.split('=',1)[1].split(',') for a in args if a.startswith('--actions=')),list(actions))
    if '--preview-only' not in args:
        with tempfile.TemporaryDirectory(prefix=f'{name}_frames_') as tmp:
            for action_name in selected:
                action=actions[action_name]
                rig.animation_data.action=action
                count=int(action['sprite_frames'])
                frames=[]
                for direction in range(1 if '--quick' in args else 8):
                    rig.rotation_euler.z=math.radians(direction*45)
                    for frame in range(1,count+1):
                        scene.frame_set(frame)
                        path=Path(tmp)/f'{action_name}_{direction}_{frame}.png'
                        lib.render(str(path))
                        frames.append(path)
                    print(f'[{name}] {action_name}: direction {direction+1}',flush=True)
                if '--quick' not in args:
                    write_sheet(out.parent/f'{name}_{action_name}.png',frames,count,res)
                write_gif(out.parent/f'{name}_{action_name}_preview.gif',frames[:count],64,int(action['fps']),bool(action['loop']))
    rig.rotation_euler.z=0
    rig.animation_data.action=actions['walk']
    scene.frame_start,scene.frame_end=1,8
    scene.frame_set(1)
    scene.render.filepath=str(out)
    bpy.ops.object.select_all(action='DESELECT')
    rig.select_set(True)
    bpy.context.view_layer.objects.active=rig
    for screen in bpy.data.screens:
        for area in screen.areas:
            if area.type=='VIEW_3D':
                area.spaces.active.region_3d.view_distance=1.8
                area.spaces.active.region_3d.view_location=(0,0,.44)
    lib.save_outputs(str(here.parent/'models'),name)
    print(f'[{name}] finished',flush=True)


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
    objects = builder.build(bevel=0.005, outline=0.008, no_outline=('eye', 'gold', 'lamp'))
    for obj in objects:
        obj.name = bone + '_' + obj.name
        obj.parent = rig
        group = obj.vertex_groups.new(name=bone)
        group.add(list(range(len(obj.data.vertices))), 1.0, 'REPLACE')
        mod = obj.modifiers.new('Skeleton', 'ARMATURE')
        mod.object = rig
        # Skin first; the silhouette shell follows the animated mesh.
        bpy.context.view_layer.objects.active = obj
        while obj.modifiers.find(mod.name) > 0:
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


def make_action(rig, name, count, pose, loop=True):
    action = bpy.data.actions.new(name)
    action.use_fake_user = True
    rig.animation_data.action = action
    for frame in range(1, count + (2 if loop else 1)):
        for bone in rig.pose.bones:
            bone.location = (0, 0, 0)
            bone.scale = (1, 1, 1)
            bone.rotation_mode = 'QUATERNION'
            bone.rotation_quaternion = (1, 0, 0, 0)
        pose((frame - 1) / count * math.tau if loop else (frame - 1) / (count - 1))
        for bone in rig.pose.bones:
            for prop in ('location', 'rotation_quaternion', 'scale'):
                bone.keyframe_insert(prop, frame=frame, group=bone.name)
    for curve in curves(action):
        for key in curve.keyframe_points:
            key.interpolation = 'LINEAR'
        if loop:
            curve.modifiers.new('CYCLES')
    action.use_frame_range = True
    action.frame_start, action.frame_end = 1, count
    action['sprite_frames'] = count
    action['loop'] = loop
    action['fps'] = 3 if name == 'idle' else 10
    if loop:
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


def build_character(profile):
    spec = [('root', (0,0,0), (0,0,.10), None),
            ('pelvis', (0, 0, .36), (0, 0, .45), 'root'),
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
    rig = armature(spec)
    rig.name = profile['name'].title()
    rig.data.name = rig.name + 'Skeleton'
    materials = {
        'shirt': lib.mat_noise('ochre_linen', [(.2, tuple(v*.72 for v in profile['coat'])), (.8, profile['coat'])], scale=75, bump=.10),
        'cream': lib.mat_flat('linen_trim', (.73, .58, .35)),
        'pants': lib.mat_flat('umber_trousers', (.18, .095, .047)),
        'boot': lib.mat_flat('worn_leather', (.105, .044, .021)),
        'skin': lib.mat_flat('warm_skin', (.64, .35, .19)),
        'hair': lib.mat_flat('chestnut_hair', (.13, .047, .017)),
        'cap': lib.mat_noise('rust_cap', [(.25, tuple(v*.72 for v in profile['hat_color'])), (.75, profile['hat_color'])], scale=60, bump=.08),
        'eye': lib.mat_flat('eyes', (.025, .013, .007)),
        'gold': lib.mat_flat('brass_buckle', (.57, .33, .09), metallic=.55),
        'burlap': lib.mat_noise('sack_canvas', [(.3, (.36, .26, .13)), (.7, (.55, .43, .24))], scale=65, bump=.15),
    }
    materials.update(extra_materials())
    def part(bone, draw):
        builder = lib.Builder(materials)
        draw(builder)
        return bind(builder, rig, bone)
    def torso(b):
        b.sphere('shirt', (0, 0, .51), .15, scale=(1, .70, 1.01), segments=12)
        b.cylinder('shirt', (0, 0, .405), .152, .125, .09, segments=10)
        b.cylinder('boot', (0, 0, .442), .134, .134, .032, segments=12)
        b.box('gold', (0, -.133, .443), (.039, .015, .032))
        b.box('boot', (0, -.143, .443), (.020, .007, .016))
        b.sphere('burlap', (-.139, -.066, .403), .050, scale=(.8,.6,1.1), segments=8)
        b.box('boot', (-.139,-.071,.450), (.023,.024,.041))
        add_clothing(b, profile)
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
        add_hat(b, profile)
    part('head', head)
    for side, s in [('L', -1), ('R', 1)]:
        x = s * .087
        part(f'thigh.{side}', lambda b, x=x: b.sphere('pants', (x, 0, .286), .071, scale=(.88, .94, 1.30), segments=10))
        part(f'shin.{side}', lambda b, x=x: b.cylinder('boot', (x, 0, .147), .047, .056, .147, segments=10))
        def foot(b, x=x):
            b.sphere('boot', (x, -.029, .049), .057, scale=(.94, 1.48, .77), segments=10)
            b.box('boot', (x, -.027, .013), (.106, .157, .023))
        part(f'foot.{side}', foot)
        part(f'upper_arm.{side}', lambda b, s=s: b.cylinder('shirt', (s * .153, 0, .543), .052, .076, .148, segments=8))
        def forearm(b, s=s):
            b.cylinder('skin', (s * .182, 0, .420), .029, .038, .102, segments=8)
            b.box('skin', (s * .190, -.005, .355), (.061,.059,.067))
            b.cylinder('cream', (s * .176, 0, .476), .052, .052, .037, segments=8)
        part(f'forearm.{side}', forearm)
    def load(b):
        add_load(b, profile)
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
    if profile.get('tool'):
        spec_tool = ('tool', (.19,0,.355), (.19,-.12,.355), 'forearm.R')
        bpy.context.view_layer.objects.active = rig
        bpy.ops.object.mode_set(mode='EDIT')
        bone = rig.data.edit_bones.new('tool')
        bone.head, bone.tail = spec_tool[1], spec_tool[2]
        bone.parent = rig.data.edit_bones['forearm.R']
        bpy.ops.object.mode_set(mode='OBJECT')
        for obj in part('tool', lambda b: add_tool(b, profile)):
            for prop in ('hide_render','hide_viewport'):
                driver=obj.driver_add(prop).driver
                variable=driver.variables.new()
                variable.name='s'
                variable.type='SINGLE_PROP'
                variable.targets[0].id=rig
                variable.targets[0].data_path='pose.bones["tool"].scale[0]'
                driver.expression='s < 0.5'
    return rig

