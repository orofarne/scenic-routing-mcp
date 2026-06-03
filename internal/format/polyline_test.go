package format

import (
	"math"
	"testing"

	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

const polylineEps = 1e-9

// coordsEqual fails the test if the two coordinate slices differ in length or
// if any element is further apart than polylineEps degrees.
func coordsEqual(t *testing.T, got, want [][2]float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d; got=%v", len(got), len(want), got)
	}
	for i, w := range want {
		if math.Abs(got[i][0]-w[0]) > polylineEps || math.Abs(got[i][1]-w[1]) > polylineEps {
			t.Errorf("point %d: got [%.9f, %.9f], want [%.9f, %.9f]",
				i, got[i][0], got[i][1], w[0], w[1])
		}
	}
}

// ── decodePolyline ────────────────────────────────────────────────────────────

func TestDecodePolyline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  [][2]float64
	}{
		{
			"empty string",
			"",
			nil,
		},
		{
			"origin (0, 0)",
			"??",
			[][2]float64{{0, 0}},
		},
		{
			"positive microdegree",
			"AA",
			// 'A'=65: b=2, result=2, 2<32 stop; result&1=0 → 2>>1=1 → 1/1e6
			[][2]float64{{1e-6, 1e-6}},
		},
		{
			"negative microdegree",
			"@@",
			// '@'=64: b=1, result=1, 1<32 stop; result&1=1 → ~(1>>1)=~0=-1 → -1/1e6
			[][2]float64{{-1e-6, -1e-6}},
		},
		{
			"three points: delta encoding accumulates",
			"??AAAA",
			// (0,0) then delta (+1,+1) then delta (+1,+1)
			[][2]float64{{0, 0}, {1e-6, 1e-6}, {2e-6, 2e-6}},
		},
		{
			"negative delta reverses direction",
			"??AA@@",
			// (0,0) → (+1e-6,+1e-6) → (-1e-6,-1e-6) net delta → back to (0,0)
			[][2]float64{{0, 0}, {1e-6, 1e-6}, {0, 0}},
		},
		{
			"multi-byte encoding: single London-area point",
			"_}hfaB~paF",
			// lat=51500000 → 51.5°, lon=-116000 → -0.116°
			// Verified by hand against the decoder loop.
			[][2]float64{{51.5, -0.116}},
		},
		{
			"multi-byte encoding: two London-area points",
			"_}hfaB~paFgEgE",
			// Second point adds delta +100/+100 microdegrees:
			// 'g'=103: b=40, result=8 (continue); 'E'=69: b=6, result=8|(6<<5)=200; 200>>1=100
			[][2]float64{{51.5, -0.116}, {51.5001, -0.1159}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decodePolyline(tc.input)
			coordsEqual(t, got, tc.want)
		})
	}
}

// ── DecodeLegs ────────────────────────────────────────────────────────────────

func TestDecodeLegs(t *testing.T) {
	leg := func(shape string) valhalla.Leg { return valhalla.Leg{Shape: shape} }

	t.Run("nil legs", func(t *testing.T) {
		if got := DecodeLegs(nil); got != nil {
			t.Errorf("want nil, got %v", got)
		}
	})

	t.Run("single empty leg", func(t *testing.T) {
		got := DecodeLegs([]valhalla.Leg{leg("")})
		if len(got) != 0 {
			t.Errorf("want empty, got %v", got)
		}
	})

	t.Run("single leg", func(t *testing.T) {
		// "??AA" → [[0,0], [1e-6, 1e-6]]
		got := DecodeLegs([]valhalla.Leg{leg("??AA")})
		coordsEqual(t, got, [][2]float64{{0, 0}, {1e-6, 1e-6}})
	})

	t.Run("two legs: junction point deduplicated", func(t *testing.T) {
		// Leg 1 "??AA" → [[0,0], [1e-6,1e-6]]
		// Leg 2 "AAAA" → [[1e-6,1e-6], [2e-6,2e-6]]; first point dropped as junction.
		// Result: [[0,0], [1e-6,1e-6], [2e-6,2e-6]]
		got := DecodeLegs([]valhalla.Leg{leg("??AA"), leg("AAAA")})
		coordsEqual(t, got, [][2]float64{{0, 0}, {1e-6, 1e-6}, {2e-6, 2e-6}})
	})

	t.Run("three legs: all junctions deduplicated", func(t *testing.T) {
		// Each leg encodes absolute coordinates from a fresh (0,0) state.
		// Leg 1 "??AA":  [[0,0],    [1e-6,1e-6]]
		// Leg 2 "AAAA":  [[1e-6,1e-6], [2e-6,2e-6]]  — junction dropped
		// Leg 3 "CCAA":  [[2e-6,2e-6], [3e-6,3e-6]]  — junction dropped
		//   'C'=67: b=4 → 4>>1=2 → 2e-6; 'A': delta +1e-6
		// Result: [[0,0], [1e-6,1e-6], [2e-6,2e-6], [3e-6,3e-6]]
		got := DecodeLegs([]valhalla.Leg{leg("??AA"), leg("AAAA"), leg("CCAA")})
		coordsEqual(t, got, [][2]float64{{0, 0}, {1e-6, 1e-6}, {2e-6, 2e-6}, {3e-6, 3e-6}})
	})
}
