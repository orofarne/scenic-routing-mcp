package heatmap

import (
	"math"
	"testing"
)

const testEps = 1e-9

// ── segDistM ─────────────────────────────────────────────────────────────────

func TestSegDistM(t *testing.T) {
	// lonToM = latToM = 1 so coordinates are treated as metres directly.
	const m = 1.0
	tests := []struct {
		name     string
		lon, lat float64
		ax, ay   float64
		bx, by   float64
		want     float64
	}{
		{
			"perpendicular to middle",
			2, 3, 0, 0, 4, 0,
			3,
		},
		{
			"projection before start (t < 0)",
			-1, 0, 0, 0, 4, 0,
			1,
		},
		{
			"projection past end (t > 1)",
			5, 0, 0, 0, 4, 0,
			1,
		},
		{
			"point on segment",
			2, 0, 0, 0, 4, 0,
			0,
		},
		{
			"point at start vertex",
			0, 0, 0, 0, 4, 0,
			0,
		},
		{
			"point at end vertex",
			4, 0, 0, 0, 4, 0,
			0,
		},
		{
			"zero-length segment (a == b)",
			3, 4, 0, 0, 0, 0,
			5,
		},
		{
			"diagonal segment, perpendicular projection",
			0, 1, 0, 0, 1, 1,
			math.Sqrt2 / 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := segDistM(
				tc.lon, tc.lat,
				[2]float64{tc.ax, tc.ay}, [2]float64{tc.bx, tc.by},
				m, m,
			)
			if math.Abs(got-tc.want) > testEps {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ── pointInRing ──────────────────────────────────────────────────────────────

func TestPointInRing(t *testing.T) {
	// Unit square (CCW winding): corners at (0,0), (1,0), (1,1), (0,1).
	square := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}

	// L-shape: a 2×2 square with the top-right quadrant removed.
	//
	//  (0,2)──(1,2)
	//    │      │
	//  (0,1)  (1,1)──(2,1)
	//    │              │
	//  (0,0)────────(2,0)
	lShape := [][2]float64{{0, 0}, {2, 0}, {2, 1}, {1, 1}, {1, 2}, {0, 2}}

	tests := []struct {
		name string
		lon  float64
		lat  float64
		ring [][2]float64
		want bool
	}{
		{"inside unit square", 0.5, 0.5, square, true},
		{"outside right", 1.5, 0.5, square, false},
		{"outside left", -0.5, 0.5, square, false},
		{"outside top", 0.5, 1.5, square, false},
		{"outside bottom", 0.5, -0.5, square, false},
		{"L-shape bottom-left quadrant", 0.5, 0.5, lShape, true},
		{"L-shape bottom-right quadrant", 1.5, 0.5, lShape, true},
		{"L-shape top-left quadrant", 0.5, 1.5, lShape, true},
		{"L-shape missing top-right quadrant", 1.5, 1.5, lShape, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pointInRing(tc.lon, tc.lat, tc.ring)
			if got != tc.want {
				t.Errorf("pointInRing(%v, %v) = %v, want %v", tc.lon, tc.lat, got, tc.want)
			}
		})
	}
}

// ── ringAreaSqM ──────────────────────────────────────────────────────────────

func TestRingAreaSqM(t *testing.T) {
	const m = 1.0
	tests := []struct {
		name string
		ring [][2]float64
		want float64
	}{
		{
			"unit square",
			[][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
			1.0,
		},
		{
			"right triangle with legs 3 and 4",
			[][2]float64{{0, 0}, {3, 0}, {0, 4}},
			6.0,
		},
		{
			"degenerate: fewer than 3 points",
			[][2]float64{{0, 0}, {1, 0}},
			0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ringAreaSqM(tc.ring, m, m)
			if math.Abs(got-tc.want) > testEps {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// ── parseGeom ────────────────────────────────────────────────────────────────

func TestParseGeom(t *testing.T) {
	t.Run("nil on empty input", func(t *testing.T) {
		if parseGeom(nil) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("nil on invalid JSON", func(t *testing.T) {
		if parseGeom([]byte("not-json")) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("nil on unknown type", func(t *testing.T) {
		if parseGeom([]byte(`{"type":"Donut","coordinates":[]}`)) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("Point", func(t *testing.T) {
		g := parseGeom([]byte(`{"type":"Point","coordinates":[1.5,2.5]}`))
		p, ok := g.(pointGeom)
		if !ok {
			t.Fatalf("expected pointGeom, got %T", g)
		}
		if p[0] != 1.5 || p[1] != 2.5 {
			t.Errorf("wrong coords: %v", p)
		}
	})

	t.Run("LineString", func(t *testing.T) {
		g := parseGeom([]byte(`{"type":"LineString","coordinates":[[0,0],[1,1],[2,0]]}`))
		l, ok := g.(lineGeom)
		if !ok {
			t.Fatalf("expected lineGeom, got %T", g)
		}
		if len(l) != 3 {
			t.Errorf("want 3 points, got %d", len(l))
		}
	})

	t.Run("MultiLineString", func(t *testing.T) {
		g := parseGeom([]byte(`{"type":"MultiLineString","coordinates":[[[0,0],[1,1]],[[2,2],[3,3]]]}`))
		mg, ok := g.(multiGeom)
		if !ok {
			t.Fatalf("expected multiGeom, got %T", g)
		}
		if len(mg) != 2 {
			t.Errorf("want 2 lines, got %d", len(mg))
		}
	})

	t.Run("Polygon", func(t *testing.T) {
		g := parseGeom([]byte(`{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`))
		if _, ok := g.(polyGeom); !ok {
			t.Fatalf("expected polyGeom, got %T", g)
		}
	})

	t.Run("MultiPolygon", func(t *testing.T) {
		g := parseGeom([]byte(`{"type":"MultiPolygon","coordinates":[[[[0,0],[1,0],[1,1],[0,0]]]]}`))
		if _, ok := g.(multiPolyGeom); !ok {
			t.Fatalf("expected multiPolyGeom, got %T", g)
		}
	})

	t.Run("GeometryCollection with two members", func(t *testing.T) {
		g := parseGeom([]byte(`{
			"type": "GeometryCollection",
			"geometries": [
				{"type": "Point", "coordinates": [0, 0]},
				{"type": "LineString", "coordinates": [[0,0],[1,1]]}
			]
		}`))
		mg, ok := g.(multiGeom)
		if !ok {
			t.Fatalf("expected multiGeom, got %T", g)
		}
		if len(mg) != 2 {
			t.Errorf("want 2 members, got %d", len(mg))
		}
		if _, ok := mg[0].(pointGeom); !ok {
			t.Errorf("first member: expected pointGeom, got %T", mg[0])
		}
		if _, ok := mg[1].(lineGeom); !ok {
			t.Errorf("second member: expected lineGeom, got %T", mg[1])
		}
	})
}

// ── pointGeom ────────────────────────────────────────────────────────────────

func TestPointGeom(t *testing.T) {
	const m = 1.0
	p := pointGeom([2]float64{0, 0})

	t.Run("distM to itself is 0", func(t *testing.T) {
		if d := p.distM(0, 0, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("distM 3-4-5 triangle", func(t *testing.T) {
		got := p.distM(3, 4, m, m)
		if math.Abs(got-5) > testEps {
			t.Errorf("got %v, want 5", got)
		}
	})

	t.Run("bbox is a degenerate point", func(t *testing.T) {
		q := pointGeom([2]float64{1.5, 2.5})
		want := [4]float64{1.5, 2.5, 1.5, 2.5}
		if b := q.bbox(); b != want {
			t.Errorf("got %v, want %v", b, want)
		}
	})
}

// ── lineGeom ─────────────────────────────────────────────────────────────────

func TestLineGeom(t *testing.T) {
	const m = 1.0

	t.Run("distM perpendicular to middle of segment", func(t *testing.T) {
		line := lineGeom{{0, 0}, {4, 0}}
		got := line.distM(2, 3, m, m)
		if math.Abs(got-3) > testEps {
			t.Errorf("got %v, want 3", got)
		}
	})

	t.Run("distM point on segment returns 0", func(t *testing.T) {
		line := lineGeom{{0, 0}, {4, 0}}
		got := line.distM(2, 0, m, m)
		if got > testEps {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("distM multi-segment picks minimum", func(t *testing.T) {
		// Polyline: (0,0)→(2,0)→(2,2). Point at (3,1): 1m from second segment.
		line := lineGeom{{0, 0}, {2, 0}, {2, 2}}
		got := line.distM(3, 1, m, m)
		if math.Abs(got-1) > testEps {
			t.Errorf("got %v, want 1", got)
		}
	})

	t.Run("areaSqM is length × 3", func(t *testing.T) {
		// Segment of length 3: areaSqM = 3 * 3 = 9.
		line := lineGeom{{0, 0}, {3, 0}}
		got := line.areaSqM(m, m)
		if math.Abs(got-9) > testEps {
			t.Errorf("got %v, want 9", got)
		}
	})

	t.Run("bbox across mixed coordinates", func(t *testing.T) {
		line := lineGeom{{0, 1}, {2, 3}, {-1, 4}, {5, -2}}
		want := [4]float64{-1, -2, 5, 4}
		if b := line.bbox(); b != want {
			t.Errorf("got %v, want %v", b, want)
		}
	})
}

// ── polyGeom ─────────────────────────────────────────────────────────────────

func TestPolyGeomDistM(t *testing.T) {
	const m = 1.0
	// Unit square as the outer ring.
	square := polyGeom{{{0, 0}, {1, 0}, {1, 1}, {0, 1}}}

	t.Run("point inside returns 0", func(t *testing.T) {
		if d := square.distM(0.5, 0.5, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("point outside returns distance to nearest edge", func(t *testing.T) {
		// (1.5, 0.5) is 0.5m from the right edge.
		got := square.distM(1.5, 0.5, m, m)
		if math.Abs(got-0.5) > testEps {
			t.Errorf("got %v, want 0.5", got)
		}
	})

	t.Run("empty polygon returns MaxFloat64", func(t *testing.T) {
		empty := polyGeom{}
		if d := empty.distM(0, 0, m, m); d != math.MaxFloat64 {
			t.Errorf("got %v, want MaxFloat64", d)
		}
	})
}

// ── multiPolyGeom ────────────────────────────────────────────────────────────

func TestMultiPolyGeomDistM(t *testing.T) {
	const m = 1.0
	// Two disjoint unit squares: x ∈ [0,1] and x ∈ [3,4].
	mp := multiPolyGeom{
		{{{0, 0}, {1, 0}, {1, 1}, {0, 1}}},
		{{{3, 0}, {4, 0}, {4, 1}, {3, 1}}},
	}

	t.Run("inside first polygon", func(t *testing.T) {
		if d := mp.distM(0.5, 0.5, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("inside second polygon", func(t *testing.T) {
		if d := mp.distM(3.5, 0.5, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("midpoint between polygons: distance 1 to each", func(t *testing.T) {
		got := mp.distM(2, 0.5, m, m)
		if math.Abs(got-1) > testEps {
			t.Errorf("got %v, want 1", got)
		}
	})
}

func TestMultiPolyGeomBbox(t *testing.T) {
	mp := multiPolyGeom{
		{{{0, 0}, {1, 0}, {1, 1}, {0, 1}}},
		{{{3, -1}, {4, 0}, {4, 1}, {3, 1}}},
	}
	got := mp.bbox()
	want := [4]float64{0, -1, 4, 1}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
