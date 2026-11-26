"""Generate the optional local casting-depth grid for level_001_lake."""
import argparse
import base64
import json
import math
from pathlib import Path, PurePosixPath
from tempfile import NamedTemporaryFile

import numpy as np
import UnityPy

from fish_query_water import Scene


LEVEL = 'level_001_lake'


def build_cast_water_depth(scene):
    # This map has a single terrain and two alternatives for the same flat lake.
    # Fail closed if a new client changes that layout.
    terrains = list(scene.terrain_grids())
    if len(terrains) != 1:
        raise ValueError('Expected a single terrain')
    heights, origin, scale = terrains[0]
    rows, cols = heights.shape
    step = scale['x']
    if not (2 <= rows <= 2049 and 2 <= cols <= 2049 and 0 < step <= 1 and step == scale['z']):
        raise ValueError('Unsupported terrain grid')
    if not np.isfinite(origin).all() or not math.isfinite(scale['y']) or scale['y'] <= 0:
        raise ValueError('Unsupported terrain transform')
    x = origin[0] + np.arange(cols) * step
    z = origin[2] + np.arange(rows) * step
    bounds = [x[0], z[0], x[-1], z[-1]]
    samples = np.zeros((rows, cols), dtype='<u2')
    for start in range(0, rows, 128):
        end = min(start + 128, rows)
        xs, zs = np.meshgrid(x, z[start:end])
        points = np.column_stack([xs.ravel(), zs.ravel()])
        ground = origin[1] + heights[start:end].ravel() * scale['y']
        wet, bodies = scene.sample_water(points, ground, bounds)
        levels = {b['level'] for b in bodies}
        if len(levels) != 1 or not math.isfinite(next(iter(levels))):
            raise ValueError('Unsupported lake water baseline')
        depth_mm = np.rint((levels.pop() - ground) * 1000)
        valid = wet & (depth_mm > 0) & (depth_mm <= 65535)
        samples[start:end] = np.where(valid, depth_mm, 0).reshape(end - start, cols)
    if not samples.any():
        raise ValueError('No wet terrain vertices')
    return {'version': 1, 'level': LEVEL, 'x': float(x[0]), 'z': float(z[0]), 'step': step,
            'width': cols, 'height': rows,
            'millimeters': base64.b64encode(samples.tobytes()).decode('ascii')}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client-data', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    if args.output.resolve().is_relative_to(args.client_data.resolve().parent):
        parser.error('Output must not be inside the client installation')
    build = UnityPy.load(str(args.client_data / 'globalgamemanagers'))
    paths = next(o.read_typetree()['scenes'] for o in build.objects if o.type.name == 'BuildSettings')
    indices = [i for i, p in enumerate(paths) if PurePosixPath(p).stem == LEVEL]
    if len(indices) != 1:
        parser.error('Expected one matching lake scene')
    result = build_cast_water_depth(Scene(args.client_data, indices[0]))
    with NamedTemporaryFile(mode='w', encoding='utf8', dir=args.output.parent, delete=False) as f:
        tmp = Path(f.name)
    try:
        tmp.write_text(json.dumps(result, separators=(',', ':')), encoding='utf8')
        tmp.replace(args.output)
    finally:
        tmp.unlink(missing_ok=True)
    print(f"{LEVEL}: {result['width']} x {result['height']}, step {result['step']} m -> {args.output}")


if __name__ == '__main__':
    main()
