"""Stonemason: profession data for the shared rig and renderer.

Run: blender -b --factory-startup --python stonemason.py -- <out.png> [256] [32]
Actions: walk (8), idle (4), carry (8), work (8), death (4).
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run

PROFILE = {
    'name': 'stonemason',
    'coat': (0.39, 0.31, 0.22),
    'hat': 'cap',
    'hat_color': (0.22, 0.17, 0.12),
    'tool': 'pick',
    'load': 'stone',
    'apron': 'gray',
}

if __name__ == "__main__":
    run(PROFILE)
