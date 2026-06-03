package format

import "testing"

func TestFeatureColor(t *testing.T) {
	tests := []struct {
		tags map[string]string
		want string
	}{
		{map[string]string{"leisure": "park"}, "#00aa44"},
		{map[string]string{"natural": "water"}, "#008888"},
		{map[string]string{"waterway": "river"}, "#0066cc"},
		{map[string]string{"amenity": "cafe"}, "#ff8800"},
		{map[string]string{"tourism": "attraction"}, "#cc00cc"},
		{map[string]string{"historic": "castle"}, "#886600"},
		{map[string]string{"shop": "bakery"}, "#888888"},  // no matching category
		{map[string]string{}, "#888888"},                  // empty tags
	}
	for _, tc := range tests {
		if got := featureColor(tc.tags); got != tc.want {
			t.Errorf("featureColor(%v) = %q, want %q", tc.tags, got, tc.want)
		}
	}
}

func TestMarkerSize(t *testing.T) {
	tests := []struct {
		sim  float64
		want string
	}{
		{1.0, "large"},
		{0.75, "large"},  // boundary: >= 0.75
		{0.74, "medium"},
		{0.55, "medium"}, // boundary: >= 0.55
		{0.54, "small"},
		{0.0, "small"},
	}
	for _, tc := range tests {
		if got := markerSize(tc.sim); got != tc.want {
			t.Errorf("markerSize(%.2f) = %q, want %q", tc.sim, got, tc.want)
		}
	}
}
