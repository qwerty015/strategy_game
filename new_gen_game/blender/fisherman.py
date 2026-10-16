"""Fisherman: profession data for the shared rig and renderer.

Run: blender -b --factory-startup --python fisherman.py -- <out.png> [256] [32]
Actions: walk (8), idle (4), carry (8), work (8), death (4).
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run

PROFILE = {
    'name': 'fisherman',
    'coat': (0.065, 0.19, 0.37),
    'hat': 'wide',
    'hat_color': (0.12, 0.21, 0.32),
    'tool': 'rod',
    'load': 'fish',
    'camera_scale': 1.38,
    'camera_height': 0.51,
}

if __name__ == "__main__":
    run(PROFILE)
