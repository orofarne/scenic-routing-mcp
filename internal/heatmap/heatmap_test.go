package heatmap

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
)

// makeGrid creates a Width×Height grid over the unit square [0,1]×[0,1] (degrees)
// with all cells initialised to -Inf. Callers set Values directly for specific tests.
func makeGrid(width, height int) *Grid {
	values := make([][]float64, height)
	for y := range values {
		values[y] = make([]float64, width)
		for x := range values[y] {
			values[y][x] = math.Inf(-1)
		}
	}
	cellLon := 1.0 / float64(width)
	cellLat := 1.0 / float64(height)
	return &Grid{
		Values:      values,
		Width:       width,
		Height:      height,
		MinLon:      0,
		MinLat:      0,
		MaxLon:      1,
		MaxLat:      1,
		CellLon:     cellLon,
		CellLat:     cellLat,
		ResolutionM: cellLat * metersPerDegLat,
	}
}

// pointFeature builds a minimal geodata.Feature with a Point geometry.
func pointFeature(lon, lat, sim float64) geodata.Feature {
	geom, _ := json.Marshal(map[string]any{
		"type":        "Point",
		"coordinates": [2]float64{lon, lat},
	})
	return geodata.Feature{Geom: geom, Similarity: sim}
}

// ── clampInt ──────────────────────────────────────────────────────────────────

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},   // in range
		{-1, 0, 10, 0},  // below lo
		{11, 0, 10, 10}, // above hi
		{0, 0, 10, 0},   // equal to lo
		{10, 0, 10, 10}, // equal to hi
	}
	for _, tc := range tests {
		if got := clampInt(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampInt(%d,%d,%d) = %d, want %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}

// ── BoundsJSON ────────────────────────────────────────────────────────────────

func TestBoundsJSON(t *testing.T) {
	g := &Grid{
		MinLon: 1, MinLat: 2, MaxLon: 3, MaxLat: 4,
		Width: 10, Height: 20, ResolutionM: 50,
	}
	s := g.BoundsJSON()
	for _, want := range []string{`"minLon":1`, `"minLat":2`, `"maxLon":3`, `"maxLat":4`,
		`"width":10`, `"height":20`, `"resolution_m":50`} {
		if !strings.Contains(s, want) {
			t.Errorf("BoundsJSON() missing %q in %q", want, s)
		}
	}
}

// ── Encode ────────────────────────────────────────────────────────────────────

func TestEncode(t *testing.T) {
	t.Run("all -Inf grid encodes to all zeros", func(t *testing.T) {
		g := makeGrid(3, 3)
		out := g.Encode()
		for i, b := range out {
			if b != 0 {
				t.Errorf("out[%d] = %d, want 0", i, b)
			}
		}
	})

	t.Run("single hot cell encodes to 255", func(t *testing.T) {
		g := makeGrid(3, 3)
		g.Values[1][1] = 1.0
		out := g.Encode()
		// ceiling = nonzero[0] = 1.0 → byte(1.0*255) = 255
		if out[1*3+1] != 255 {
			t.Errorf("hot cell = %d, want 255", out[1*3+1])
		}
		// All other cells are -Inf → 0
		for y := 0; y < 3; y++ {
			for x := 0; x < 3; x++ {
				if y == 1 && x == 1 {
					continue
				}
				if out[y*3+x] != 0 {
					t.Errorf("cold cell (%d,%d) = %d, want 0", y, x, out[y*3+x])
				}
			}
		}
	})

	t.Run("two cells: 95th percentile ceiling clips lower value", func(t *testing.T) {
		// 1×2 grid (height=1, width=2): values [0.5, 1.0]
		// nonzero sorted = [0.5, 1.0]; ceiling = nonzero[int(2*0.95)] = nonzero[1] = 1.0
		// → [byte(127), byte(255)]
		g := makeGrid(2, 1)
		g.Values[0][0] = 0.5
		g.Values[0][1] = 1.0
		out := g.Encode()
		if out[0] != 127 {
			t.Errorf("out[0] = %d, want 127", out[0])
		}
		if out[1] != 255 {
			t.Errorf("out[1] = %d, want 255", out[1])
		}
	})

	t.Run("output length equals Width×Height", func(t *testing.T) {
		g := makeGrid(7, 5)
		if got := len(g.Encode()); got != 35 {
			t.Errorf("len = %d, want 35", got)
		}
	})
}

// ── ScoreRoute ────────────────────────────────────────────────────────────────

func TestScoreRoute(t *testing.T) {
	// 3×3 grid over unit square; only cell (y=1, x=1) is hot.
	// Cell centres (lat, lon): y=1,x=1 → (0.5, 0.5); y=2,x=0 → (1/6, 1/6).
	newHotGrid := func() *Grid {
		g := makeGrid(3, 3)
		g.Values[1][1] = 1.0
		return g
	}

	t.Run("empty coords returns 0", func(t *testing.T) {
		if got := newHotGrid().ScoreRoute(nil); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("all-Inf grid returns 0", func(t *testing.T) {
		g := makeGrid(3, 3) // all cells -Inf
		if got := g.ScoreRoute([][2]float64{{0.5, 0.5}}); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("single point in hot cell returns 1.0", func(t *testing.T) {
		got := newHotGrid().ScoreRoute([][2]float64{{0.5, 0.5}})
		if math.Abs(got-1.0) > 1e-9 {
			t.Errorf("got %v, want 1.0", got)
		}
	})

	t.Run("single point in cold cell returns 0", func(t *testing.T) {
		// (1/6, 1/6) maps to cell (y=2, x=0) which is -Inf.
		got := newHotGrid().ScoreRoute([][2]float64{{1.0 / 6, 1.0 / 6}})
		if got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("two-point route: one hot sample, one cold sample → 0.5", func(t *testing.T) {
		// (0.5, 0.5) → hot cell; (1/6, 1/6) → cold cell.
		// Distance ≈ 52 km >> 50 m, so both endpoints are sampled once each.
		got := newHotGrid().ScoreRoute([][2]float64{{0.5, 0.5}, {1.0 / 6, 1.0 / 6}})
		if math.Abs(got-0.5) > 1e-9 {
			t.Errorf("got %v, want 0.5", got)
		}
	})
}

// ── Compute ───────────────────────────────────────────────────────────────────

func TestCompute(t *testing.T) {
	// Use a 3°×3° bbox centred near the equator.
	const minLon, minLat, maxLon, maxLat = 0.0, 0.0, 3.0, 3.0

	countNonInf := func(g *Grid) int {
		n := 0
		for _, row := range g.Values {
			for _, v := range row {
				if !math.IsInf(v, -1) {
					n++
				}
			}
		}
		return n
	}

	t.Run("no features: all cells -Inf", func(t *testing.T) {
		g := Compute(nil, minLon, minLat, maxLon, maxLat, Params{ResolutionM: metersPerDegLat})
		if n := countNonInf(g); n != 0 {
			t.Errorf("%d non-Inf cells, want 0", n)
		}
	})

	t.Run("feature with Similarity=0 is skipped", func(t *testing.T) {
		f := pointFeature(1.5, 1.5, 0)
		g := Compute([]geodata.Feature{f}, minLon, minLat, maxLon, maxLat, Params{ResolutionM: metersPerDegLat})
		if n := countNonInf(g); n != 0 {
			t.Errorf("%d non-Inf cells, want 0 for zero-similarity feature", n)
		}
	})

	t.Run("point feature with Similarity=1 heats nearby cells", func(t *testing.T) {
		// Large sigma (150 km) so the influence radius covers several cells.
		f := pointFeature(1.5, 1.5, 1.0)
		g := Compute([]geodata.Feature{f}, minLon, minLat, maxLon, maxLat,
			Params{ResolutionM: metersPerDegLat, SigmaM: 150000})
		if n := countNonInf(g); n == 0 {
			t.Error("expected at least one heated cell, got none")
		}
	})

	t.Run("grid dimensions match bbox and resolution", func(t *testing.T) {
		// ResolutionM = 1°-equivalent → 3×3 grid expected.
		g := Compute(nil, minLon, minLat, maxLon, maxLat, Params{ResolutionM: metersPerDegLat})
		if g.Width != 3 || g.Height != 3 {
			t.Errorf("grid %d×%d, want 3×3", g.Width, g.Height)
		}
	})
}

// ── Peaks ─────────────────────────────────────────────────────────────────────

func TestPeaks(t *testing.T) {
	t.Run("fewer than 4 non-zero cells returns nil", func(t *testing.T) {
		g := makeGrid(10, 10)
		// Set 3 hot cells — one below the minimum required.
		g.Values[5][5] = 1.0
		g.Values[5][6] = 1.0
		g.Values[5][7] = 1.0
		if got := g.Peaks(10); got != nil {
			t.Errorf("got %v, want nil for < 4 non-zero cells", got)
		}
	})

	t.Run("uniform heat (Gini ≈ 0) returns nil", func(t *testing.T) {
		// All cells equal → Gini = 0 < peakFlatGini.
		g := makeGrid(5, 5)
		for y := range g.Values {
			for x := range g.Values[y] {
				g.Values[y][x] = 1.0
			}
		}
		if got := g.Peaks(10); got != nil {
			t.Errorf("got %v, want nil for uniform grid", got)
		}
	})

	t.Run("concentrated heat returns peaks with Heat in [0,1]", func(t *testing.T) {
		// 10×10 grid; 5 adjacent hot cells in a row (Gini ≈ 0.95 >> 0.5).
		g := makeGrid(10, 10)
		for x := 3; x <= 7; x++ {
			g.Values[5][x] = 1.0
		}
		peaks := g.Peaks(10)
		if peaks == nil {
			t.Fatal("expected peaks, got nil")
		}
		for i, p := range peaks {
			if p.Heat < 0 || p.Heat > 1 {
				t.Errorf("peaks[%d].Heat = %v outside [0,1]", i, p.Heat)
			}
		}
	})

	t.Run("result never exceeds requested n", func(t *testing.T) {
		g := makeGrid(10, 10)
		for x := 3; x <= 7; x++ {
			g.Values[5][x] = 1.0
		}
		for _, n := range []int{1, 2, 3} {
			peaks := g.Peaks(n)
			if peaks != nil && len(peaks) > n {
				t.Errorf("Peaks(%d) returned %d peaks", n, len(peaks))
			}
		}
	})
}
