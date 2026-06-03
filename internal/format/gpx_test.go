package format

import (
	"strings"
	"testing"
)

func TestGpx(t *testing.T) {
	t.Run("contains required XML structure", func(t *testing.T) {
		out := gpx([][2]float64{{51.5, -0.116}}, 1.2, 900)
		for _, want := range []string{
			`<?xml version="1.0"`,
			`<gpx `,
			`<metadata>`,
			`<trk><trkseg>`,
			`</trkseg></trk>`,
			`</gpx>`,
		} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q", want)
			}
		}
	})

	t.Run("metadata contains length and time", func(t *testing.T) {
		out := gpx([][2]float64{{0, 0}}, 5.25, 3600)
		if !strings.Contains(out, "length_km=5.250") {
			t.Errorf("length not found in %q", out)
		}
		if !strings.Contains(out, "time_s=3600") {
			t.Errorf("time not found in %q", out)
		}
	})

	t.Run("single trkpt with correct lat/lon", func(t *testing.T) {
		// coords are [lat, lon]; gpx outputs lat="%f" lon="%f"
		out := gpx([][2]float64{{51.5, -0.116}}, 0, 0)
		if !strings.Contains(out, `lat="51.500000" lon="-0.116000"`) {
			t.Errorf("trkpt not found in %q", out)
		}
	})

	t.Run("two trkpt elements for two-point route", func(t *testing.T) {
		out := gpx([][2]float64{{51.5, -0.116}, {51.6, -0.1}}, 0, 0)
		if strings.Count(out, "<trkpt") != 2 {
			t.Errorf("expected 2 trkpt elements, got %d", strings.Count(out, "<trkpt"))
		}
	})
}
