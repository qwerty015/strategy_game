"""Miner: profession data for the shared rig and renderer.

Run: blender -b --factory-startup --python miner.py -- <out.png> [256] [32]
Actions: walk (8), idle (4), carry (8), work (8), death (4).
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from units import run

PROFILE = {
    'name': 'miner',
    'coat': (0.095, 0.09, 0.075),
    'hat': 'helmet',
    'hat_color': (0.39, 0.29, 0.11),
    'tool': 'pick',
    'load': 'ore',
}

if __name__ == "__main__":
    run(PROFILE)
