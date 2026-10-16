"""Serf: ochre linen, rolled sleeves, belt pouch and shoulder sack.
Run: blender -b --factory-startup --python serf.py -- <out.png> [256] [32]
See units.py for shared rig, fixed camera, sheet layout and preview export.
"""
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run
PROFILE = dict(name="serf", coat=(.63,.39,.15), hat="cap", hat_color=(.38,.13,.049), load="sack")
if __name__ == "__main__":
    run(PROFILE)
