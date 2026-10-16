"""Lumberjack: profession data for the shared rig and renderer.

Run: blender -b --factory-startup --python lumberjack.py -- <out.png> [256] [32]
Actions: walk (8), idle (4), carry (8), work (8), death (4).
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run

PROFILE = {
    'name': 'lumberjack',
    'coat': (0.16, 0.3, 0.065),
    'hat': 'hood',
    'hat_color': (0.13, 0.23, 0.045),
    'tool': 'axe',
    'load': 'log',
}

if __name__ == "__main__":
    run(PROFILE)
