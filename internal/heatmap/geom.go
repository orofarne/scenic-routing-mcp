package heatmap

import (
	"encoding/json"
	"math"
)

// geomType abstracts all GeoJSON geometry variants for distance and area queries.
type geomType interface {
	// distM returns the distance in metres from (lon, lat) to the nearest point
	// on the geometry. Returns 0 if the point lies inside a polygon.
	distM(lon, lat, lonToM, latToM float64) float64
	// areaSqM returns an approximate area in m² used for weight and σ computation.
	areaSqM(lonToM, latToM float64) float64
	// bbox returns [minLon, minLat, maxLon, maxLat].
	bbox() [4]float64
}

type rawGeom struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
	Geometries  []rawGeom       `json:"geometries"`
}

func parseGeom(data []byte) geomType {
	if len(data) == 0 {
		return nil
	}
	var raw rawGeom
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	return buildGeom(raw)
}

func buildGeom(raw rawGeom) geomType {
	switch raw.Type {
	case "Point":
		var c [2]float64
		if json.Unmarshal(raw.Coordinates, &c) != nil {
			return nil
		}
		return pointGeom(c)
	case "LineString":
		var c [][2]float64
		if json.Unmarshal(raw.Coordinates, &c) != nil {
			return nil
		}
		return lineGeom(c)
	case "MultiLineString":
		var c [][][2]float64
		if json.Unmarshal(raw.Coordinates, &c) != nil {
			return nil
		}
		mg := make(multiGeom, len(c))
		for i, l := range c {
			mg[i] = lineGeom(l)
		}
		return mg
	case "Polygon":
		var c [][][2]float64
		if json.Unmarshal(raw.Coordinates, &c) != nil {
			return nil
		}
		return polyGeom(c)
	case "MultiPolygon":
		var c [][][][2]float64
		if json.Unmarshal(raw.Coordinates, &c) != nil {
			return nil
		}
		return multiPolyGeom(c)
	case "GeometryCollection":
		mg := make(multiGeom, 0, len(raw.Geometries))
		for _, g := range raw.Geometries {
			if built := buildGeom(g); built != nil {
				mg = append(mg, built)
			}
		}
		return mg
	}
	return nil
}

// ── pointGeom ──────────────────────────────────────────────────────────────

type pointGeom [2]float64 // [lon, lat]

func (p pointGeom) distM(lon, lat, lonToM, latToM float64) float64 {
	dx := (lon - p[0]) * lonToM
	dy := (lat - p[1]) * latToM
	return math.Sqrt(dx*dx + dy*dy)
}
func (p pointGeom) areaSqM(_, _ float64) float64 { return math.Pi * 5 * 5 } // nominal 5 m radius
func (p pointGeom) bbox() [4]float64              { return [4]float64{p[0], p[1], p[0], p[1]} }

// ── lineGeom ───────────────────────────────────────────────────────────────

type lineGeom [][2]float64

func (l lineGeom) distM(lon, lat, lonToM, latToM float64) float64 {
	min := math.MaxFloat64
	for i := 1; i < len(l); i++ {
		if d := segDistM(lon, lat, l[i-1], l[i], lonToM, latToM); d < min {
			min = d
		}
	}
	return min
}
func (l lineGeom) areaSqM(lonToM, latToM float64) float64 {
	total := 0.0
	for i := 1; i < len(l); i++ {
		dx := (l[i][0] - l[i-1][0]) * lonToM
		dy := (l[i][1] - l[i-1][1]) * latToM
		total += math.Sqrt(dx*dx + dy*dy)
	}
	return total * 3 // 3 m effective buffer width
}
func (l lineGeom) bbox() [4]float64 {
	if len(l) == 0 {
		return [4]float64{}
	}
	b := [4]float64{l[0][0], l[0][1], l[0][0], l[0][1]}
	for _, p := range l[1:] {
		if p[0] < b[0] {
			b[0] = p[0]
		}
		if p[1] < b[1] {
			b[1] = p[1]
		}
		if p[0] > b[2] {
			b[2] = p[0]
		}
		if p[1] > b[3] {
			b[3] = p[1]
		}
	}
	return b
}

// ── polyGeom ───────────────────────────────────────────────────────────────

type polyGeom [][][2]float64 // [ring][point]

func (pg polyGeom) distM(lon, lat, lonToM, latToM float64) float64 {
	if len(pg) == 0 {
		return math.MaxFloat64
	}
	if pointInRing(lon, lat, pg[0]) {
		return 0
	}
	return lineGeom(pg[0]).distM(lon, lat, lonToM, latToM)
}
func (pg polyGeom) areaSqM(lonToM, latToM float64) float64 {
	if len(pg) == 0 {
		return 0
	}
	return ringAreaSqM(pg[0], lonToM, latToM)
}
func (pg polyGeom) bbox() [4]float64 {
	if len(pg) == 0 {
		return [4]float64{}
	}
	return lineGeom(pg[0]).bbox()
}

// ── multiPolyGeom ──────────────────────────────────────────────────────────

type multiPolyGeom [][][][2]float64

func (mp multiPolyGeom) distM(lon, lat, lonToM, latToM float64) float64 {
	min := math.MaxFloat64
	for _, p := range mp {
		if d := polyGeom(p).distM(lon, lat, lonToM, latToM); d < min {
			min = d
		}
	}
	return min
}
func (mp multiPolyGeom) areaSqM(lonToM, latToM float64) float64 {
	total := 0.0
	for _, p := range mp {
		total += polyGeom(p).areaSqM(lonToM, latToM)
	}
	return total
}
func (mp multiPolyGeom) bbox() [4]float64 {
	if len(mp) == 0 {
		return [4]float64{}
	}
	b := polyGeom(mp[0]).bbox()
	for _, p := range mp[1:] {
		pb := polyGeom(p).bbox()
		if pb[0] < b[0] {
			b[0] = pb[0]
		}
		if pb[1] < b[1] {
			b[1] = pb[1]
		}
		if pb[2] > b[2] {
			b[2] = pb[2]
		}
		if pb[3] > b[3] {
			b[3] = pb[3]
		}
	}
	return b
}

// ── multiGeom (collection) ─────────────────────────────────────────────────

type multiGeom []geomType

func (mg multiGeom) distM(lon, lat, lonToM, latToM float64) float64 {
	min := math.MaxFloat64
	for _, g := range mg {
		if d := g.distM(lon, lat, lonToM, latToM); d < min {
			min = d
		}
	}
	return min
}
func (mg multiGeom) areaSqM(lonToM, latToM float64) float64 {
	total := 0.0
	for _, g := range mg {
		total += g.areaSqM(lonToM, latToM)
	}
	return total
}
func (mg multiGeom) bbox() [4]float64 {
	if len(mg) == 0 {
		return [4]float64{}
	}
	b := mg[0].bbox()
	for _, g := range mg[1:] {
		gb := g.bbox()
		if gb[0] < b[0] {
			b[0] = gb[0]
		}
		if gb[1] < b[1] {
			b[1] = gb[1]
		}
		if gb[2] > b[2] {
			b[2] = gb[2]
		}
		if gb[3] > b[3] {
			b[3] = gb[3]
		}
	}
	return b
}

// ── helpers ────────────────────────────────────────────────────────────────

// segDistM returns the distance in metres from (lon, lat) to the segment a→b.
func segDistM(lon, lat float64, a, b [2]float64, lonToM, latToM float64) float64 {
	px := (lon - a[0]) * lonToM
	py := (lat - a[1]) * latToM
	dx := (b[0] - a[0]) * lonToM
	dy := (b[1] - a[1]) * latToM
	lenSq := dx*dx + dy*dy
	t := 0.0
	if lenSq > 0 {
		t = (px*dx + py*dy) / lenSq
		if t < 0 {
			t = 0
		} else if t > 1 {
			t = 1
		}
	}
	ex := px - t*dx
	ey := py - t*dy
	return math.Sqrt(ex*ex + ey*ey)
}

// pointInRing reports whether (lon, lat) is inside the ring using ray casting.
func pointInRing(lon, lat float64, ring [][2]float64) bool {
	inside := false
	n := len(ring)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if ((yi > lat) != (yj > lat)) && (lon < (xj-xi)*(lat-yi)/(yj-yi)+xi) {
			inside = !inside
		}
	}
	return inside
}

// ringAreaSqM computes the area of a ring in m² via the Shoelace formula.
func ringAreaSqM(ring [][2]float64, lonToM, latToM float64) float64 {
	n := len(ring)
	if n < 3 {
		return 0
	}
	area := 0.0
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi := ring[i][0] * lonToM
		yi := ring[i][1] * latToM
		xj := ring[j][0] * lonToM
		yj := ring[j][1] * latToM
		area += xj*yi - xi*yj
	}
	return math.Abs(area) / 2
}
