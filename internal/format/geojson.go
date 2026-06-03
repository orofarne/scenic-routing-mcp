package format

import (
	"encoding/json"

	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

// GeoJSON encodes a route as a GeoJSON FeatureCollection with a single LineString feature.
func GeoJSON(route *valhalla.RouteResult) (string, error) {
	coords := DecodeLegs(route.Trip.Legs)
	return geojson(coords, route.Trip.Summary.Length, route.Trip.Summary.Time)
}

// geojson encodes coords as a GeoJSON FeatureCollection.
// Coordinates are [lon, lat] per GeoJSON spec.
func geojson(coords [][2]float64, lengthKm, timeSecs float64) (string, error) {
	lonLat := make([][2]float64, len(coords))
	for i, c := range coords {
		lonLat[i] = [2]float64{c[1], c[0]} // swap lat/lon → lon/lat
	}
	fc := map[string]any{
		"type":        "FeatureCollection",
		"attribution": dataAttribution,
		"license":     dataLicense,
		"features": []any{
			map[string]any{
				"type": "Feature",
				"geometry": map[string]any{
					"type":        "LineString",
					"coordinates": lonLat,
				},
				"properties": map[string]any{
					"length_km": lengthKm,
					"time_s":    timeSecs,
				},
			},
		},
	}
	b, err := json.Marshal(fc)
	return string(b), err
}
