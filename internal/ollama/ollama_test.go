package ollama

import (
	"math"
	"testing"
)

func TestTruncateNormalize(t *testing.T) {
	const eps = 1e-6

	t.Run("longer than dim is truncated", func(t *testing.T) {
		got := truncateNormalize([]float32{1, 2, 3, 4}, 2)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})

	t.Run("shorter than dim is unchanged in length", func(t *testing.T) {
		got := truncateNormalize([]float32{1, 0}, 10)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})

	t.Run("result has unit norm", func(t *testing.T) {
		got := truncateNormalize([]float32{3, 4}, 10)
		var norm float64
		for _, v := range got {
			norm += float64(v) * float64(v)
		}
		if math.Abs(math.Sqrt(norm)-1.0) > eps {
			t.Errorf("norm = %v, want 1.0", math.Sqrt(norm))
		}
	})

	t.Run("3-4-5 triangle: exact values", func(t *testing.T) {
		got := truncateNormalize([]float32{3, 4}, 10)
		if math.Abs(float64(got[0])-0.6) > eps {
			t.Errorf("got[0] = %v, want 0.6", got[0])
		}
		if math.Abs(float64(got[1])-0.8) > eps {
			t.Errorf("got[1] = %v, want 0.8", got[1])
		}
	})

	t.Run("zero vector is returned unchanged", func(t *testing.T) {
		got := truncateNormalize([]float32{0, 0, 0}, 10)
		for i, v := range got {
			if v != 0 {
				t.Errorf("got[%d] = %v, want 0", i, v)
			}
		}
	})

	t.Run("already unit vector is unchanged", func(t *testing.T) {
		got := truncateNormalize([]float32{1, 0}, 10)
		if math.Abs(float64(got[0])-1.0) > eps || math.Abs(float64(got[1])-0.0) > eps {
			t.Errorf("got = %v, want [1 0]", got)
		}
	})
}
