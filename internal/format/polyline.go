package format

import "github.com/orofarne/scenic-routing-mcp/internal/valhalla"

// DecodeLegs concatenates the decoded polyline shapes from all route legs.
// Points are returned as [lat, lon].
func DecodeLegs(legs []valhalla.Leg) [][2]float64 {
	var all [][2]float64
	for i, leg := range legs {
		pts := decodePolyline(leg.Shape)
		if i > 0 && len(all) > 0 {
			pts = pts[1:] // drop duplicate junction point between legs
		}
		all = append(all, pts...)
	}
	return all
}

// decodePolyline decodes a Valhalla encoded polyline (1e6 precision).
// Points are returned as [lat, lon].
func decodePolyline(encoded string) [][2]float64 {
	var points [][2]float64
	index, lat, lng := 0, 0, 0
	for index < len(encoded) {
		lat += decodeChunk(encoded, &index)
		lng += decodeChunk(encoded, &index)
		points = append(points, [2]float64{float64(lat) / 1e6, float64(lng) / 1e6})
	}
	return points
}

func decodeChunk(s string, index *int) int {
	result, shift := 0, 0
	for {
		b := int(s[*index]) - 63
		*index++
		result |= (b & 0x1f) << shift
		shift += 5
		if b < 0x20 {
			break
		}
	}
	if result&1 != 0 {
		return ^(result >> 1)
	}
	return result >> 1
}
