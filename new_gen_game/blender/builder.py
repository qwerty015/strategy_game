"""Builder: profession data for the shared rig and renderer.

Run: blender -b --factory-startup --python builder.py -- <out.png> [256] [32]
Actions: walk (8), idle (4), carry (8), work (8), death (4).
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run

PROFILE = {
    'name': 'builder',
    'coat': (0.52, 0.32, 0.13),
    'hat': 'cap',
    'hat_color': (0.32, 0.12, 0.047),
    'tool': 'hammer',
    'load': 'planks',
    'apron': 'leather',
}

if __name__ == "__main__":
    run(PROFILE)
