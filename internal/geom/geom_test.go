package geom

import (
	"math"
	"testing"
)

const testEps = 1e-9

// ── segDistM ─────────────────────────────────────────────────────────────────

func TestSegDistM(t *testing.T) {
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
	square := [][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}

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

// ── Parse ─────────────────────────────────────────────────────────────────────

func TestParse(t *testing.T) {
	t.Run("nil on empty input", func(t *testing.T) {
		if Parse(nil) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("nil on invalid JSON", func(t *testing.T) {
		if Parse([]byte("not-json")) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("nil on unknown type", func(t *testing.T) {
		if Parse([]byte(`{"type":"Donut","coordinates":[]}`)) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("Point", func(t *testing.T) {
		g := Parse([]byte(`{"type":"Point","coordinates":[1.5,2.5]}`))
		p, ok := g.(pointGeom)
		if !ok {
			t.Fatalf("expected pointGeom, got %T", g)
		}
		if p[0] != 1.5 || p[1] != 2.5 {
			t.Errorf("wrong coords: %v", p)
		}
	})

	t.Run("LineString", func(t *testing.T) {
		g := Parse([]byte(`{"type":"LineString","coordinates":[[0,0],[1,1],[2,0]]}`))
		l, ok := g.(lineGeom)
		if !ok {
			t.Fatalf("expected lineGeom, got %T", g)
		}
		if len(l) != 3 {
			t.Errorf("want 3 points, got %d", len(l))
		}
	})

	t.Run("MultiLineString", func(t *testing.T) {
		g := Parse([]byte(`{"type":"MultiLineString","coordinates":[[[0,0],[1,1]],[[2,2],[3,3]]]}`))
		mg, ok := g.(multiGeom)
		if !ok {
			t.Fatalf("expected multiGeom, got %T", g)
		}
		if len(mg) != 2 {
			t.Errorf("want 2 lines, got %d", len(mg))
		}
	})

	t.Run("Polygon", func(t *testing.T) {
		g := Parse([]byte(`{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`))
		if _, ok := g.(polyGeom); !ok {
			t.Fatalf("expected polyGeom, got %T", g)
		}
	})

	t.Run("MultiPolygon", func(t *testing.T) {
		g := Parse([]byte(`{"type":"MultiPolygon","coordinates":[[[[0,0],[1,0],[1,1],[0,0]]]]}`))
		if _, ok := g.(multiPolyGeom); !ok {
			t.Fatalf("expected multiPolyGeom, got %T", g)
		}
	})

	t.Run("GeometryCollection with two members", func(t *testing.T) {
		g := Parse([]byte(`{
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

	t.Run("DistM to itself is 0", func(t *testing.T) {
		if d := p.DistM(0, 0, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("DistM 3-4-5 triangle", func(t *testing.T) {
		got := p.DistM(3, 4, m, m)
		if math.Abs(got-5) > testEps {
			t.Errorf("got %v, want 5", got)
		}
	})

	t.Run("BBox is a degenerate point", func(t *testing.T) {
		q := pointGeom([2]float64{1.5, 2.5})
		want := [4]float64{1.5, 2.5, 1.5, 2.5}
		if b := q.BBox(); b != want {
			t.Errorf("got %v, want %v", b, want)
		}
	})
}

// ── lineGeom ─────────────────────────────────────────────────────────────────

func TestLineGeom(t *testing.T) {
	const m = 1.0

	t.Run("DistM perpendicular to middle of segment", func(t *testing.T) {
		line := lineGeom{{0, 0}, {4, 0}}
		got := line.DistM(2, 3, m, m)
		if math.Abs(got-3) > testEps {
			t.Errorf("got %v, want 3", got)
		}
	})

	t.Run("DistM point on segment returns 0", func(t *testing.T) {
		line := lineGeom{{0, 0}, {4, 0}}
		got := line.DistM(2, 0, m, m)
		if got > testEps {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("DistM multi-segment picks minimum", func(t *testing.T) {
		line := lineGeom{{0, 0}, {2, 0}, {2, 2}}
		got := line.DistM(3, 1, m, m)
		if math.Abs(got-1) > testEps {
			t.Errorf("got %v, want 1", got)
		}
	})

	t.Run("BBox across mixed coordinates", func(t *testing.T) {
		line := lineGeom{{0, 1}, {2, 3}, {-1, 4}, {5, -2}}
		want := [4]float64{-1, -2, 5, 4}
		if b := line.BBox(); b != want {
			t.Errorf("got %v, want %v", b, want)
		}
	})
}

// ── polyGeom ─────────────────────────────────────────────────────────────────

func TestPolyGeomDistM(t *testing.T) {
	const m = 1.0
	square := polyGeom{{{0, 0}, {1, 0}, {1, 1}, {0, 1}}}

	t.Run("point inside returns 0", func(t *testing.T) {
		if d := square.DistM(0.5, 0.5, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("point outside returns distance to nearest edge", func(t *testing.T) {
		got := square.DistM(1.5, 0.5, m, m)
		if math.Abs(got-0.5) > testEps {
			t.Errorf("got %v, want 0.5", got)
		}
	})

	t.Run("empty polygon returns MaxFloat64", func(t *testing.T) {
		empty := polyGeom{}
		if d := empty.DistM(0, 0, m, m); d != math.MaxFloat64 {
			t.Errorf("got %v, want MaxFloat64", d)
		}
	})
}

// ── multiPolyGeom ────────────────────────────────────────────────────────────

func TestMultiPolyGeomDistM(t *testing.T) {
	const m = 1.0
	mp := multiPolyGeom{
		{{{0, 0}, {1, 0}, {1, 1}, {0, 1}}},
		{{{3, 0}, {4, 0}, {4, 1}, {3, 1}}},
	}

	t.Run("inside first polygon", func(t *testing.T) {
		if d := mp.DistM(0.5, 0.5, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("inside second polygon", func(t *testing.T) {
		if d := mp.DistM(3.5, 0.5, m, m); d != 0 {
			t.Errorf("got %v, want 0", d)
		}
	})

	t.Run("midpoint between polygons: distance 1 to each", func(t *testing.T) {
		got := mp.DistM(2, 0.5, m, m)
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
	got := mp.BBox()
	want := [4]float64{0, -1, 4, 1}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
