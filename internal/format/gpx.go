package format

import (
	"fmt"
	"strings"

	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

// GPX encodes a route as a GPX 1.1 track.
func GPX(route *valhalla.RouteResult) string {
	coords := DecodeLegs(route.Trip.Legs)
	return gpx(coords, route.Trip.Summary.Length, route.Trip.Summary.Time)
}

func gpx(coords [][2]float64, lengthKm, timeSecs float64) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<gpx xmlns="http://www.topografix.com/GPX/1/1" version="1.1" creator="scenic-routing-mcp">` + "\n")
	fmt.Fprintf(&b, "  <metadata>\n    <desc>length_km=%.3f time_s=%.0f</desc>\n    <copyright author=%q><license>%s</license></copyright>\n  </metadata>\n", lengthKm, timeSecs, dataAttribution, dataLicense)
	b.WriteString("  <trk><trkseg>\n")
	for _, c := range coords {
		fmt.Fprintf(&b, "    <trkpt lat=\"%f\" lon=\"%f\"/>\n", c[0], c[1])
	}
	b.WriteString("  </trkseg></trk>\n</gpx>\n")
	return b.String()
}
