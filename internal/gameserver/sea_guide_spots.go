package gameserver

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"image"
	"image/png"
	"math"
)

const seaTerrainOriginZ = -1957

// 挪威海 4.0.25034 level24 的 WorldShifter（组件 51259）有五个预设偏移，
// 加上初始未平移的坐标系。14/2 的钓组落点随原点平移，同包的姿态使用地图相对坐标。
var seaFrameOffsets = [...][2]float64{
	{0, 0}, {248.9757233, 474.9511719}, {2342.3054199, -1039.1062012},
	{2891.2993164, 227.2830048}, {3928.8159180, 689.5770264}, {3640.3603516, -575.5109253},
}

func seaChartLanding(rawX, rawZ, poseX, poseZ, castM float64) (x, z float64, known bool) {
	if !finiteSonarNumber(castM) || castM < 0 || !finiteSonarNumber(poseX) || !finiteSonarNumber(poseZ) {
		return 0, 0, false
	}
	// 用同竿的抛距和姿态识别唯一可达的坐标系。2 米余量覆盖竿位与姿态原点差。
	// 最终参与判鱼的是钓组落点加偏移，姿态只用于选坐标系。
	for _, offset := range seaFrameOffsets {
		cx, cz := rawX+offset[0], rawZ+offset[1]
		if math.Hypot(cx-poseX, cz-(poseZ+seaTerrainOriginZ)) > castM+2 {
			continue
		}
		if known {
			return 0, 0, false
		}
		x, z, known = cx, cz, true
	}
	return x, z, known
}

// Pixels in the derived grayscale map contain a printed sounding, or zero.
// The map and its reference points share the same 4.0.25034 chart coordinates.
//go:embed assets/sea_chart_regions.png
var seaRegionPNG []byte

//go:embed assets/sea_chart_marks.json
var seaMarkJSON []byte

type seaChartMark struct {
	spot int
	x, z float64
}

var seaRegions, seaGuideMarks = loadSeaChart()

func loadSeaChart() (*image.Gray, []seaChartMark) {
	img, err := png.Decode(bytes.NewReader(seaRegionPNG))
	if err != nil {
		panic(err)
	}
	regions, ok := img.(*image.Gray)
	if !ok || regions.Bounds() != image.Rect(0, 0, 2048, 2048) {
		panic("unexpected Norwegian Sea chart dimensions or pixel format")
	}
	var rows [][3]int
	if err := json.Unmarshal(seaMarkJSON, &rows); err != nil {
		panic(err)
	}
	marks := make([]seaChartMark, 0, len(rows))
	for _, row := range rows {
		d, u, v := row[0], row[1], row[2]
		if d <= 0 || int(regions.GrayAt(u, v).Y) != d {
			panic("Norwegian Sea chart mark disagrees with region map")
		}
		marks = append(marks, seaChartMark{d, (float64(u) - 78) / .38, (1970-float64(v))/.38 + seaTerrainOriginZ})
	}
	return regions, marks
}

func seaGuideSpotForPoint(x, z float64) int {
	u, v := math.Round(78+x*.38), math.Round(1970-(z-seaTerrainOriginZ)*.38)
	if !(u >= 0 && u < float64(seaRegions.Rect.Max.X) && v >= 0 && v < float64(seaRegions.Rect.Max.Y)) {
		return 0
	}
	return int(seaRegions.GrayAt(int(u), int(v)).Y)
}
