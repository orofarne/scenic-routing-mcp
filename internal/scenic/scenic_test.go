package scenic

import (
	"math"
	"reflect"
	"testing"
)

// ── pedestrianOpts ────────────────────────────────────────────────────────────

func TestPedestrianOpts(t *testing.T) {
	t.Run("defaults when all fields are zero", func(t *testing.T) {
		opts := pedestrianOpts(Params{})
		if opts["walkway_factor"] != 0.75 {
			t.Errorf("walkway_factor: got %v, want 0.75", opts["walkway_factor"])
		}
		if opts["path_factor"] != 0.75 {
			t.Errorf("path_factor: got %v, want 0.75", opts["path_factor"])
		}
		if opts["step_penalty"] != float64(10) {
			t.Errorf("step_penalty: got %v, want 10", opts["step_penalty"])
		}
		if opts["use_ferry"] != float64(0) {
			t.Errorf("use_ferry: got %v, want 0", opts["use_ferry"])
		}
		// Optional keys must be absent when their param is zero.
		for _, key := range []string{"use_tracks", "use_living_streets", "use_hills", "max_hiking_difficulty"} {
			if _, ok := opts[key]; ok {
				t.Errorf("key %q should be absent for zero param", key)
			}
		}
	})

	t.Run("explicit values override defaults", func(t *testing.T) {
		opts := pedestrianOpts(Params{
			WalkwayFactor: 2.0,
			PathFactor:    1.5,
			StepPenalty:   30,
			UseFerry:      0.8,
		})
		if opts["walkway_factor"] != 2.0 {
			t.Errorf("walkway_factor: got %v, want 2.0", opts["walkway_factor"])
		}
		if opts["path_factor"] != 1.5 {
			t.Errorf("path_factor: got %v, want 1.5", opts["path_factor"])
		}
		if opts["step_penalty"] != float64(30) {
			t.Errorf("step_penalty: got %v, want 30", opts["step_penalty"])
		}
		if opts["use_ferry"] != 0.8 {
			t.Errorf("use_ferry: got %v, want 0.8", opts["use_ferry"])
		}
	})

	t.Run("optional keys appear only when non-zero", func(t *testing.T) {
		opts := pedestrianOpts(Params{
			UseTracks:           0.5,
			UseLivingStreets:    0.3,
			UseHills:            0.7,
			MaxHikingDifficulty: 3,
		})
		if opts["use_tracks"] != 0.5 {
			t.Errorf("use_tracks: got %v, want 0.5", opts["use_tracks"])
		}
		if opts["use_living_streets"] != 0.3 {
			t.Errorf("use_living_streets: got %v, want 0.3", opts["use_living_streets"])
		}
		if opts["use_hills"] != 0.7 {
			t.Errorf("use_hills: got %v, want 0.7", opts["use_hills"])
		}
		if opts["max_hiking_difficulty"] != 3 {
			t.Errorf("max_hiking_difficulty: got %v, want 3", opts["max_hiking_difficulty"])
		}
	})
}

// ── routeBBox ─────────────────────────────────────────────────────────────────

const bboxEps = 1e-9

func TestRouteBBox(t *testing.T) {
	t.Run("single point, no expansion", func(t *testing.T) {
		minLon, minLat, maxLon, maxLat := routeBBox([][2]float64{{51.5, -0.116}}, 0)
		if minLat != 51.5 || maxLat != 51.5 || minLon != -0.116 || maxLon != -0.116 {
			t.Errorf("got (%.6f,%.6f,%.6f,%.6f), want (51.5,-0.116,51.5,-0.116) for no expansion",
				minLon, minLat, maxLon, maxLat)
		}
	})

	t.Run("multiple points, no expansion: tight bbox", func(t *testing.T) {
		coords := [][2]float64{{1, 2}, {3, 4}, {-1, -2}}
		minLon, minLat, maxLon, maxLat := routeBBox(coords, 0)
		if minLat != -1 || maxLat != 3 || minLon != -2 || maxLon != 4 {
			t.Errorf("got minLat=%v maxLat=%v minLon=%v maxLon=%v", minLat, maxLat, minLon, maxLon)
		}
	})

	t.Run("at equator, expand 111320 m adds exactly 1° in all directions", func(t *testing.T) {
		// At lat=0: latDelta = 111320/111320 = 1°, lonDelta = 111320/(111320*cos(0)) = 1°.
		minLon, minLat, maxLon, maxLat := routeBBox([][2]float64{{0, 0}}, 111320)
		if math.Abs(minLat-(-1)) > bboxEps {
			t.Errorf("minLat: got %.9f, want -1", minLat)
		}
		if math.Abs(maxLat-1) > bboxEps {
			t.Errorf("maxLat: got %.9f, want 1", maxLat)
		}
		if math.Abs(minLon-(-1)) > bboxEps {
			t.Errorf("minLon: got %.9f, want -1", minLon)
		}
		if math.Abs(maxLon-1) > bboxEps {
			t.Errorf("maxLon: got %.9f, want 1", maxLon)
		}
	})

	t.Run("at 60° lat, lon expansion is twice the lat expansion", func(t *testing.T) {
		// cos(60°) = 0.5, so lonDelta = 2 × latDelta.
		const expandM = 111320.0
		minLon, minLat, maxLon, maxLat := routeBBox([][2]float64{{60, 0}}, expandM)
		latDelta := maxLat - 60
		lonDelta := maxLon - 0
		if math.Abs(latDelta-1) > 1e-6 {
			t.Errorf("latDelta: got %.9f, want 1", latDelta)
		}
		if math.Abs(lonDelta-2) > 1e-4 {
			t.Errorf("lonDelta: got %.9f, want ~2", lonDelta)
		}
		_ = minLon
		_ = minLat
	})
}

// ── haversineKm ───────────────────────────────────────────────────────────────

func TestHaversineKm(t *testing.T) {
	const R = 6371.0
	const haversineEps = 0.001 // 1 metre tolerance

	tests := []struct {
		name string
		a, b [2]float64
		want float64
	}{
		{
			"same point",
			[2]float64{0, 0}, [2]float64{0, 0},
			0,
		},
		{
			"90° lon change at equator = quarter sphere",
			[2]float64{0, 0}, [2]float64{0, 90},
			R * math.Pi / 2,
		},
		{
			"90° lat change = quarter sphere",
			[2]float64{0, 0}, [2]float64{90, 0},
			R * math.Pi / 2,
		},
		{
			"180° lon change = half sphere",
			[2]float64{0, 0}, [2]float64{0, 180},
			R * math.Pi,
		},
		{
			"symmetric: swapping a and b gives same distance",
			[2]float64{51.5, -0.116}, [2]float64{48.85, 2.35},
			// Just a consistency check; want == forward direction result.
			// We call haversineKm both ways and compare below.
			0, // placeholder; tested differently
		},
	}

	for i, tc := range tests {
		if i == len(tests)-1 {
			// Symmetry test: compare forward vs reverse.
			t.Run(tc.name, func(t *testing.T) {
				fwd := haversineKm(tc.a, tc.b)
				rev := haversineKm(tc.b, tc.a)
				if math.Abs(fwd-rev) > haversineEps {
					t.Errorf("asymmetry: fwd=%.6f rev=%.6f diff=%.9f", fwd, rev, math.Abs(fwd-rev))
				}
			})
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			got := haversineKm(tc.a, tc.b)
			if math.Abs(got-tc.want) > haversineEps {
				t.Errorf("got %.6f km, want %.6f km", got, tc.want)
			}
		})
	}
}

// ── peakIterCounts ────────────────────────────────────────────────────────────

func TestPeakIterCounts(t *testing.T) {
	tests := []struct {
		n    int
		want []int
	}{
		{1, []int{1}},
		{2, []int{2}},
		{3, []int{3}},
		{4, []int{3, 4}},
		{5, []int{3, 5}},
		{6, []int{3, 6}},
		{7, []int{3, 6, 7}},
		{10, []int{3, 6, 10}},
		{11, []int{3, 6, 10, 11}},
		{15, []int{3, 6, 10, 15}},
	}

	for _, tc := range tests {
		got := peakIterCounts(tc.n)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("peakIterCounts(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

// ── tspPeakOrder ──────────────────────────────────────────────────────────────

// mat builds a square matrix of size (2+n) × (2+n) from flat row-major values.
func mat(rows ...[]float64) [][]float64 { return rows }

func TestTSPPeakOrder(t *testing.T) {
	t.Run("n=0 returns nil", func(t *testing.T) {
		if got := tspPeakOrder(0, nil, 100, nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("n=1, peak fits in budget", func(t *testing.T) {
		// Nodes: 0=start, 1=end, 2=peak0
		// start→peak0 = 1s, peak0→end = 5s; total = 6s ≤ budget 10s.
		m := mat(
			[]float64{0, 0, 1},
			[]float64{0, 0, 0},
			[]float64{0, 5, 0},
		)
		got := tspPeakOrder(1, m, 10, []float64{1.0})
		if !reflect.DeepEqual(got, []int{0}) {
			t.Errorf("got %v, want [0]", got)
		}
	})

	t.Run("n=1, peak does not fit in budget", func(t *testing.T) {
		m := mat(
			[]float64{0, 0, 1},
			[]float64{0, 0, 0},
			[]float64{0, 5, 0},
		)
		// total 1+5=6 > budget 5
		got := tspPeakOrder(1, m, 5, []float64{1.0})
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("n=2, only peak0 fits in budget", func(t *testing.T) {
		// peak0: start→p0=1, p0→end=1, total=2 ≤ 5.
		// peak1: start→p1=100, p1→end=100, total=200 > 5.
		// Even though peak1 has more heat (2.0), it is unreachable within budget.
		m := mat(
			[]float64{0, 0, 1, 100},
			[]float64{0, 0, 0, 0},
			[]float64{0, 1, 0, 50},
			[]float64{0, 100, 50, 0},
		)
		got := tspPeakOrder(2, m, 5, []float64{1.0, 2.0})
		if !reflect.DeepEqual(got, []int{0}) {
			t.Errorf("got %v, want [0]", got)
		}
	})

	t.Run("n=2, both fit, directed costs force order [0,1]", func(t *testing.T) {
		// Only the path start→p0→p1→end is within budget:
		//   1 + 1 + 1 = 3 ≤ 6
		// The reverse path start→p1→p0→end costs:
		//   10 + 1 + 10 = 21 > 6
		m := mat(
			[]float64{0, 0, 1, 10},
			[]float64{0, 0, 0, 0},
			[]float64{0, 10, 0, 1},
			[]float64{0, 1, 1, 0},
		)
		got := tspPeakOrder(2, m, 6, []float64{1.0, 1.0})
		if !reflect.DeepEqual(got, []int{0, 1}) {
			t.Errorf("got %v, want [0 1]", got)
		}
	})

	t.Run("n=2, higher-heat subset beats single peak", func(t *testing.T) {
		// peak0: heat=1, peak1: heat=3. Both fit within budget.
		// Algorithm must select both (total heat 4 > 3) over just peak1.
		m := mat(
			[]float64{0, 0, 1, 1},
			[]float64{0, 0, 0, 0},
			[]float64{0, 1, 0, 1},
			[]float64{0, 1, 1, 0},
		)
		got := tspPeakOrder(2, m, 10, []float64{1.0, 3.0})
		if len(got) != 2 {
			t.Errorf("got %v (len %d), want both peaks selected", got, len(got))
		}
	})
}
