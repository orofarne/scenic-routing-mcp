package format

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
	"github.com/orofarne/scenic-routing-mcp/internal/heatmap"
	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

// DebugGeoJSON builds a debug FeatureCollection with all layers embedded:
//   - _type="image_overlay": heatmap PNG as base64 with Leaflet-compatible _bounds
//   - _type="search_area":   the POI search polygon
//   - label="baseline":      gray baseline pedestrian route
//   - label="scenic":        blue scenic_pedestrian route
//   - POI features:          actual OSM geometries ordered by semantic similarity
//   - heatmap peaks:         _used=true if the peak was used as a waypoint
func DebugGeoJSON(baseRoute, scenicRoute *valhalla.RouteResult, features []geodata.Feature, minLon, minLat, maxLon, maxLat float64, grid *heatmap.Grid, allPeaks, usedPeaks []heatmap.Peak) (string, error) {
	pngBytes, err := HeatmapPNG(grid)
	if err != nil {
		return "", fmt.Errorf("heatmap png: %w", err)
	}

	items := make([]any, 0, len(features)+4)

	// Heatmap raster overlay — geometry is the bounding box polygon for GeoJSON validity;
	// _bounds is [[minLat,minLon],[maxLat,maxLon]] for L.imageOverlay.
	items = append(items, map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Polygon",
			"coordinates": [][][2]float64{{{minLon, minLat}, {maxLon, minLat}, {maxLon, maxLat}, {minLon, maxLat}, {minLon, minLat}}},
		},
		"properties": map[string]any{
			"_type":   "image_overlay",
			"_mime":   "image/png",
			"_data":   base64.StdEncoding.EncodeToString(pngBytes),
			"_bounds": [2][2]float64{{grid.MinLat, grid.MinLon}, {grid.MaxLat, grid.MaxLon}},
			"opacity": 0.65,
		},
	})

	// POI search area polygon (neutral name, shape may become non-rectangular in future).
	items = append(items, map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Polygon",
			"coordinates": [][][2]float64{{{minLon, minLat}, {maxLon, minLat}, {maxLon, maxLat}, {minLon, maxLat}, {minLon, minLat}}},
		},
		"properties": map[string]any{
			"_type":        "search_area",
			"stroke":       "#ff6600",
			"stroke-width": 1,
			"fill-opacity": 0,
		},
	})

	// Gray baseline pedestrian route.
	items = append(items, map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "LineString",
			"coordinates": routeLonLat(baseRoute),
		},
		"properties": map[string]any{
			"stroke":         "#888888",
			"stroke-width":   3,
			"stroke-opacity": 0.7,
			"label":          "baseline",
			"length_km":      baseRoute.Trip.Summary.Length,
			"time_s":         baseRoute.Trip.Summary.Time,
		},
	})

	// Blue scenic_pedestrian route.
	items = append(items, map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "LineString",
			"coordinates": routeLonLat(scenicRoute),
		},
		"properties": map[string]any{
			"stroke":       "#0000ff",
			"stroke-width": 5,
			"label":        "scenic",
			"length_km":    scenicRoute.Trip.Summary.Length,
			"time_s":       scenicRoute.Trip.Summary.Time,
		},
	})

	// POI features: actual OSM geometry (simplified), color by category, size by similarity.
	tagKeys := []string{"leisure", "natural", "waterway", "landuse", "amenity", "tourism", "historic", "man_made"}
	for _, f := range features {
		if len(f.Geom) == 0 {
			continue
		}
		var geomObj any
		if err := json.Unmarshal(f.Geom, &geomObj); err != nil {
			continue
		}

		props := map[string]any{
			"marker-color": featureColor(f.Tags),
			"marker-size":  markerSize(f.Similarity),
			"similarity":   f.Similarity,
		}
		if name := f.Tags["name"]; name != "" {
			props["name"] = name
		}
		for _, k := range tagKeys {
			if v := f.Tags[k]; v != "" {
				props[k] = v
				break
			}
		}
		items = append(items, map[string]any{
			"type":       "Feature",
			"geometry":   geomObj,
			"properties": props,
		})
	}

	// Heatmap peaks — top-10 local maxima as debug markers.
	// _used=true means the peak was selected as a forced waypoint in this route.
	usedSet := make(map[[2]float64]bool, len(usedPeaks))
	for _, pk := range usedPeaks {
		usedSet[[2]float64{pk.Lat, pk.Lon}] = true
	}
	for _, pk := range allPeaks {
		items = append(items, map[string]any{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "Point",
				"coordinates": [2]float64{pk.Lon, pk.Lat},
			},
			"properties": map[string]any{
				"_type": "heatmap_peak",
				"heat":  pk.Heat,
				"_used": usedSet[[2]float64{pk.Lat, pk.Lon}],
			},
		})
	}

	b, err := json.Marshal(map[string]any{
		"type":        "FeatureCollection",
		"attribution": dataAttribution,
		"license":     dataLicense,
		"features":    items,
	})
	return string(b), err
}

// routeLonLat converts a RouteResult's legs to a [lon, lat] coordinate slice
// as required by the GeoJSON spec.
func routeLonLat(r *valhalla.RouteResult) [][2]float64 {
	coords := DecodeLegs(r.Trip.Legs)
	out := make([][2]float64, len(coords))
	for i, c := range coords {
		out[i] = [2]float64{c[1], c[0]}
	}
	return out
}

// featureColor returns a map marker color based on the dominant OSM tag category.
func featureColor(tags map[string]string) string {
	switch {
	case tags["leisure"] != "":
		return "#00aa44"
	case tags["natural"] != "":
		return "#008888"
	case tags["waterway"] != "":
		return "#0066cc"
	case tags["amenity"] != "":
		return "#ff8800"
	case tags["tourism"] != "":
		return "#cc00cc"
	case tags["historic"] != "":
		return "#886600"
	default:
		return "#888888"
	}
}

// markerSize maps a similarity score to a GeoJSON marker-size string.
func markerSize(sim float64) string {
	switch {
	case sim >= 0.75:
		return "large"
	case sim >= 0.55:
		return "medium"
	default:
		return "small"
	}
}
