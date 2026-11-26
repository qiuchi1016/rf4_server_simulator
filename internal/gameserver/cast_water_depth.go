package gameserver

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"os"
	"sync"
)

// Samples are unsigned little-endian millimeters, X increasing within each Z row.
// Zero marks uncovered, dry or unrepresentable vertices.
type castWaterDepthGrid struct {
	Version     int     `json:"version"`
	Level       string  `json:"level"`
	X           float64 `json:"x"`
	Z           float64 `json:"z"`
	Step        float64 `json:"step"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Millimeters []byte  `json:"millimeters"`
}

func readCastWaterDepth(path string) *castWaterDepthGrid {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	const limit = 12 << 20 // 2049² uint16 samples in base64, plus JSON metadata.
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(data) > limit {
		return nil
	}
	var g castWaterDepthGrid
	if json.Unmarshal(data, &g) != nil || g.Version != 1 || g.Level != "level_001_lake" ||
		g.Step <= 0 || g.Step > 1 || g.Width < 2 || g.Height < 2 || g.Width > 2049 || g.Height > 2049 ||
		len(g.Millimeters) != 2*g.Width*g.Height {
		return nil
	}
	return &g
}

func (g *castWaterDepthGrid) lookup(level string, x, z float64) (float64, bool) {
	if g == nil || level != g.Level {
		return 0, false
	}
	gx, gz := (x-g.X)/g.Step, (z-g.Z)/g.Step
	if !(gx >= 0 && gx < float64(g.Width-1) && gz >= 0 && gz < float64(g.Height-1)) {
		return 0, false
	}
	ix, iz := int(gx), int(gz)
	fx, fz := gx-float64(ix), gz-float64(iz)
	at := func(x, z int) float64 {
		return float64(binary.LittleEndian.Uint16(g.Millimeters[(z*g.Width+x)*2:]))
	}
	a, b, c, d := at(ix, iz), at(ix+1, iz), at(ix, iz+1), at(ix+1, iz+1)
	if a == 0 || b == 0 || c == 0 || d == 0 {
		return 0, false
	}
	top, bottom := math.FMA(fx, b-a, a), math.FMA(fx, d-c, c)
	return math.FMA(fz, bottom-top, top) / 1000, true
}

var loadCastWaterDepth = sync.OnceValue(func() *castWaterDepthGrid {
	path := os.Getenv("RF4_CAST_WATER_DEPTH_FILE")
	if path == "" {
		path = "data/cast_water_depth.json"
	}
	return readCastWaterDepth(path)
})

func localCastWaterDepthM(levelKey string, x, z float64) (float64, bool) {
	if levelKey != "level_001_lake" {
		return 0, false
	}
	return loadCastWaterDepth().lookup(levelKey, x, z)
}
