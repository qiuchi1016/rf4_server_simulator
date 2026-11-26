import base64
import unittest
from types import SimpleNamespace

import numpy as np

from cast_water_depth import build_cast_water_depth


class CastWaterDepthTests(unittest.TestCase):
    def test_export_matches_native_terrain_and_excludes_dry_vertices(self):
        heights = np.array([[0, .25, .5], [.625, .75, .875], [1, .9, .875]])
        terrain = (heights, [-1, -8, 12], {'x': .5, 'z': .5, 'y': 8})
        def water(points, ground, bounds):
            # World X/Z footprint clips first column; third row contains dry ground.
            return (points[:, 0] > -1) & (ground < -1), [{'level': -1}]
        scene = SimpleNamespace(terrain_grids=lambda: iter([terrain]), sample_water=water)
        result = build_cast_water_depth(scene)
        self.assertEqual((result['x'], result['z'], result['step']), (-1, 12, .5))
        self.assertEqual((result['width'], result['height']), (3, 3))
        samples = np.frombuffer(base64.b64decode(result['millimeters']), dtype='<u2')
        self.assertEqual(samples.tolist(), [0, 5000, 3000, 0, 1000, 0, 0, 0, 0])
        scene.sample_water = lambda p, g, b: (g < 0, [{'level': 0}, {'level': 1}])
        with self.assertRaisesRegex(ValueError, 'baseline'):
            build_cast_water_depth(scene)
        scene.terrain_grids = lambda: iter([terrain, terrain])
        with self.assertRaisesRegex(ValueError, 'single'):
            build_cast_water_depth(scene)
