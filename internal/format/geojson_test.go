package format

import (
	"encoding/json"
	"testing"
)

func TestGeojson(t *testing.T) {
	t.Run("returns valid JSON", func(t *testing.T) {
		out, err := geojson([][2]float64{{51.5, -0.116}}, 1.0, 600)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !json.Valid([]byte(out)) {
			t.Errorf("not valid JSON: %q", out)
		}
	})

	t.Run("top-level type is FeatureCollection", func(t *testing.T) {
		out, _ := geojson([][2]float64{{0, 0}}, 0, 0)
		var fc map[string]any
		if err := json.Unmarshal([]byte(out), &fc); err != nil {
			t.Fatal(err)
		}
		if fc["type"] != "FeatureCollection" {
			t.Errorf("type = %v, want FeatureCollection", fc["type"])
		}
	})

	t.Run("geometry type is LineString", func(t *testing.T) {
		out, _ := geojson([][2]float64{{0, 0}, {1, 1}}, 0, 0)
		var fc struct {
			Features []struct {
				Geometry struct {
					Type string `json:"type"`
				} `json:"geometry"`
			} `json:"features"`
		}
		if err := json.Unmarshal([]byte(out), &fc); err != nil {
			t.Fatal(err)
		}
		if len(fc.Features) != 1 || fc.Features[0].Geometry.Type != "LineString" {
			t.Errorf("unexpected geometry: %+v", fc)
		}
	})

	t.Run("coordinates are [lon, lat] (GeoJSON spec: lon first)", func(t *testing.T) {
		// Input [lat=51.5, lon=-0.116]; output coordinate must be [-0.116, 51.5].
		out, _ := geojson([][2]float64{{51.5, -0.116}}, 0, 0)
		var fc struct {
			Features []struct {
				Geometry struct {
					Coordinates [][2]float64 `json:"coordinates"`
				} `json:"geometry"`
			} `json:"features"`
		}
		if err := json.Unmarshal([]byte(out), &fc); err != nil {
			t.Fatal(err)
		}
		c := fc.Features[0].Geometry.Coordinates[0]
		if c[0] != -0.116 || c[1] != 51.5 {
			t.Errorf("coordinate = [%v, %v], want [-0.116, 51.5]", c[0], c[1])
		}
	})

	t.Run("properties contain length_km and time_s", func(t *testing.T) {
		out, _ := geojson([][2]float64{{0, 0}}, 3.75, 1200)
		var fc struct {
			Features []struct {
				Properties map[string]any `json:"properties"`
			} `json:"features"`
		}
		if err := json.Unmarshal([]byte(out), &fc); err != nil {
			t.Fatal(err)
		}
		props := fc.Features[0].Properties
		if props["length_km"] != 3.75 {
			t.Errorf("length_km = %v, want 3.75", props["length_km"])
		}
		if props["time_s"] != float64(1200) {
			t.Errorf("time_s = %v, want 1200", props["time_s"])
		}
	})
}
