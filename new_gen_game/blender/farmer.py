"""Farmer: profession data for the shared rig and renderer.

Run: blender -b --factory-startup --python farmer.py -- <out.png> [256] [32]
Actions: walk (8), idle (4), carry (8), work (8), death (4).
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run

PROFILE = {
    'name': 'farmer',
    'coat': (0.45, 0.36, 0.18),
    'hat': 'straw',
    'hat_color': (0.7, 0.51, 0.19),
    'tool': 'sickle',
    'load': 'sack',
}

if __name__ == "__main__":
    run(PROFILE)
