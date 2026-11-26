package gameserver

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestCastWaterDepthInterpolation(t *testing.T) {
	g := castWaterDepthGrid{Version: 1, Level: "level_001_lake", X: 10, Z: -20, Step: .5, Width: 3, Height: 2,
		Millimeters: []byte{232, 3, 208, 7, 0, 0, 184, 11, 160, 15, 0, 0}}
	path := t.TempDir() + "/depth.json"
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded := readCastWaterDepth(path)
	if loaded == nil {
		t.Fatal("valid file rejected")
	}
	if got, ok := loaded.lookup(g.Level, 10.125, -19.75); !ok || math.Abs(got-2.25) > 1e-8 {
		t.Errorf("interpolated %g %v, want 2.25m", got, ok)
	}
	for _, p := range [][2]float64{{10.75, -19.75}, {9.99, -20}, {11.01, -20}, {10, -19.49}, {math.NaN(), -20}, {10, math.Inf(-1)}} {
		if _, ok := loaded.lookup(g.Level, p[0], p[1]); ok {
			t.Errorf("unexpected coverage %v", p)
		}
	}
	if _, ok := loaded.lookup("level_002_kirzah_river", 10, -20); ok {
		t.Fatal("leaked data to another map")
	}
	for _, invalid := range []string{"{}", string(data[:len(data)-1]), `{"version":1,"level":"level_001_lake","width":3,"height":2,"step":0.5}`} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if readCastWaterDepth(path) != nil {
			t.Fatalf("accepted invalid grid")
		}
	}
	if readCastWaterDepth(path+".missing") != nil {
		t.Fatal("accepted missing file")
	}
}

func TestLocalCastWaterDepthGeneratedLake(t *testing.T) {
	path := os.Getenv("RF4_TEST_CAST_WATER_DEPTH_FILE")
	if path == "" {
		t.Skip("generated lake grid not supplied")
	}
	g := readCastWaterDepth(path)
	if g == nil {
		t.Fatal("could not read generated lake grid")
	}
	for _, p := range [][3]float64{{369, 435, .241}, {378, 435, 1.010}, {360, 411, 4.000}} {
		if got, ok := g.lookup("level_001_lake", p[0], p[1]); !ok || math.Abs(got-p[2]) > .002 {
			t.Errorf("world (%v,%v) got %.6fm %v, want %.3fm", p[0], p[1], got, ok, p[2])
		} else {
			t.Logf("world (%v,%v) = %.6fm", p[0], p[1], got)
		}
	}
}
