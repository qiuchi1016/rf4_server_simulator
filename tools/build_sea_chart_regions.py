"""Digitize the 4.0.25034 Norwegian Sea paper chart into a grayscale zone index.

Usage: python tools/build_sea_chart_regions.py original_chart.png [output.png]
Offline dependencies: numpy, Pillow, OpenCV, scipy, scikit-image.
"""

import argparse
import hashlib
import json
from pathlib import Path

import cv2
import numpy as np
from PIL import Image
from scipy import ndimage
from skimage.segmentation import felzenszwalb


ASSETS = Path(__file__).resolve().parents[1] / "internal/gameserver/assets"
RGB_SHA256 = "3707ed5ed667502a296ab394148ea5a8c604e07ce43a0637c27c6b258295de28"
# Representative RGB triplets for the eleven printed depth colors, light to dark.
PALETTE = np.float32([
    [186, 196, 201], [163, 180, 192], [147, 168, 185], [119, 145, 168],
    [94, 129, 157], [66, 106, 141], [43, 89, 129], [25, 72, 110],
    [17, 58, 96], [15, 44, 75], [1, 24, 47],
])


def remove_print(rgb, marks):
    # Remove the map grid and compass before comparing neighboring water colors.
    ink = np.zeros(rgb.shape[:2], bool)
    for pos in (267, 456, 645, 833, 1022, 1212, 1400, 1589, 1778):
        ink[:, pos-1:pos+3] = True
        ink[pos-1:pos+3, :] = True
    ink[1590:1930, 1902:1943] = True
    nearest = ndimage.distance_transform_edt(ink, return_distances=False, return_indices=True)
    clean = rgb[tuple(nearest)].copy()
    # Reconstruct just the numeral strokes using adjacent ink of the same area.
    for _, u, v in marks:
        patch = clean[v-24:v+25, u-33:u+34].copy()
        median = cv2.medianBlur(patch, 13).astype(np.int16)
        stroke = np.max(np.abs(patch.astype(np.int16) - median), axis=2) > 14
        mask = cv2.dilate(stroke.astype(np.uint8), np.ones((3, 3), np.uint8))
        mask[:14] = mask[-14:] = 0
        mask[:, :12] = mask[:, -12:] = 0
        clean[v-24:v+25, u-33:u+34] = cv2.inpaint(patch, mask, 4, cv2.INPAINT_NS)
    return cv2.medianBlur(clean, 7)


def water_mask(clean):
    r, g, b = np.moveaxis(clean.astype(np.int16), 2, 0)
    water = np.zeros(clean.shape[:2], bool)
    water[81:1967, 81:1967] = True
    water[80:308, 1183:1969] = False  # legend
    water[96:288, 97:626] = False     # title
    return water & (b > r+4) & (b >= g+1)


def hatched_banks(rgb, water):
    # The diagonal hatch repeats every 8.55 pixels along u+v. Measure its
    # local amplitude in both phases, so the outline is independent of a label.
    gray = cv2.cvtColor(rgb, cv2.COLOR_RGB2GRAY).astype(np.float32)
    gray -= cv2.GaussianBlur(gray, (0, 0), 3)
    y, x = np.ogrid[:gray.shape[0], :gray.shape[1]]
    phase = (x+y) * (2*np.pi/8.55)
    a = cv2.GaussianBlur((gray*np.cos(phase)).astype(np.float32), (0, 0), 3)
    b = cv2.GaussianBlur((gray*np.sin(phase)).astype(np.float32), (0, 0), 3)
    mask = ((np.hypot(a, b) > 4) & water).astype(np.uint8)
    kernel = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (7, 7))
    mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
    contours, _ = cv2.findContours(mask, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)
    banks = np.zeros_like(mask)
    for index, contour in enumerate(sorted(contours, key=cv2.contourArea, reverse=True), 1):
        if cv2.contourArea(contour) < 2000:
            break
        cv2.fillPoly(banks, [cv2.approxPolyDP(contour, 1.8, True)], index)
    return banks


def water_colors(clean):
    y, x = np.ogrid[:clean.shape[0], :clean.shape[1]]
    # The northwest harbor has a warm, visible paper underlay.
    coast = np.clip((540-x)/100, 0, 1) * np.clip((250-np.abs(y-1020))/100, 0, 1)
    pixels = clean.astype(np.float32)
    pixels -= coast[:, :, None] * np.float32([9, 7, 5])
    bands = np.zeros(clean.shape[:2], np.uint8)
    best = np.full(bands.shape, np.inf, np.float32)
    for band, color in enumerate(PALETTE, 1):
        # Allow for the darkening of the printed palette across the paper folds.
        gain = np.clip(sum(pixels[:, :, j]*color[j] for j in range(3)) / sum(color*color), .75, 1.16)
        dist = sum((pixels[:, :, j]-gain*color[j])**2 for j in range(3)) + (65*(gain-1))**2
        closer = dist < best
        bands[closer] = band
        best[closer] = dist[closer]
    # Give each continuous patch of ink the majority color of its pixels.
    parts = felzenszwalb(clean, scale=22, sigma=.8, min_size=22).astype(np.int32)
    colors = len(PALETTE)+1
    votes = np.bincount((parts*colors+bands).ravel(), minlength=(parts.max()+1)*colors)
    return votes.reshape(-1, colors).argmax(axis=1).astype(np.uint8)[parts]


def assign_soundings(bands, marks):
    components = np.zeros(bands.shape, np.int32)
    offset = 0
    for band in np.unique(bands[bands != 0]):
        count, ids = cv2.connectedComponents((bands == band).astype(np.uint8), connectivity=4)
        selected = ids != 0
        components[selected] = offset + ids[selected]
        offset += count-1
    mark_regions = components[marks[:, 2], marks[:, 1]]
    if np.any(mark_regions == 0):
        raise ValueError(f"Sounding outside mapped water: {marks[mark_regions == 0]}")
    result = np.zeros(bands.shape, np.uint8)
    for region in np.unique(mark_regions):
        ys, xs = np.nonzero(components == region)
        best = np.full(ys.shape, np.iinfo(np.int32).max, np.int32)
        # At a tie the smaller printed number wins, independent of file order.
        for depth, u, v in sorted(marks[mark_regions == region].tolist()):
            squared = (xs-u)**2 + (ys-v)**2
            closer = squared < best
            result[ys[closer], xs[closer]] = depth
            best[closer] = squared[closer]
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("chart", type=Path)
    parser.add_argument("output", nargs="?", type=Path, default=ASSETS/"sea_chart_regions.png")
    args = parser.parse_args()
    rgb = np.array(Image.open(args.chart).convert("RGB"))
    if hashlib.sha256(rgb.tobytes()).hexdigest() != RGB_SHA256 or rgb.shape != (2048, 2048, 3):
        parser.error("expected the 2048px Norwegian Sea chart from 4.0.25034")
    marks = np.array(json.loads((ASSETS/"sea_chart_marks.json").read_text()), dtype=int)
    clean = remove_print(rgb, marks)
    water = water_mask(clean)
    banks = hatched_banks(rgb, water)
    bands = water_colors(clean)
    bands[~water] = 0
    bands[banks > 0] = len(PALETTE) + banks[banks > 0]
    result = assign_soundings(bands, marks)
    if not np.array_equal(result[marks[:, 2], marks[:, 1]], marks[:, 0]):
        raise ValueError("a sounding disagrees with the resulting chart")
    Image.fromarray(result).save(args.output, optimize=True)
    print(f"{len(marks)} soundings, {len(np.unique(marks[:, 0]))} depths; wrote {args.output}")


if __name__ == "__main__":
    main()
