package geom

import (
	"encoding/json"
	"math"
)

// Geom abstracts all GeoJSON geometry variants for distance and bounds queries.
type Geom interface {
	DistM(lon, lat, lonToM, latToM float64) float64
	BBox() [4]float64
}

type rawGeom struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
	Geometries  []rawGeom       `json:"geometries"`
}

// Parse parses GeoJSON geometry bytes and returns a Geom, or nil on error.
func Parse(data []byte) Geom {
	if len(data) == 0 {
		return nil
	}
	var raw rawGeom
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	return build(raw)
}

func build(raw rawGeom) Geom {
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
			if built := build(g); built != nil {
				mg = append(mg, built)
			}
		}
		return mg
	}
	return nil
}

// ── pointGeom ──────────────────────────────────────────────────────────────

type pointGeom [2]float64 // [lon, lat]

func (p pointGeom) DistM(lon, lat, lonToM, latToM float64) float64 {
	dx := (lon - p[0]) * lonToM
	dy := (lat - p[1]) * latToM
	return math.Sqrt(dx*dx + dy*dy)
}
func (p pointGeom) BBox() [4]float64 { return [4]float64{p[0], p[1], p[0], p[1]} }

// ── lineGeom ───────────────────────────────────────────────────────────────

type lineGeom [][2]float64

func (l lineGeom) DistM(lon, lat, lonToM, latToM float64) float64 {
	min := math.MaxFloat64
	for i := 1; i < len(l); i++ {
		if d := segDistM(lon, lat, l[i-1], l[i], lonToM, latToM); d < min {
			min = d
		}
	}
	return min
}
func (l lineGeom) BBox() [4]float64 {
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

func (pg polyGeom) DistM(lon, lat, lonToM, latToM float64) float64 {
	if len(pg) == 0 {
		return math.MaxFloat64
	}
	if pointInRing(lon, lat, pg[0]) {
		return 0
	}
	return lineGeom(pg[0]).DistM(lon, lat, lonToM, latToM)
}
func (pg polyGeom) BBox() [4]float64 {
	if len(pg) == 0 {
		return [4]float64{}
	}
	return lineGeom(pg[0]).BBox()
}

// ── multiPolyGeom ──────────────────────────────────────────────────────────

type multiPolyGeom [][][][2]float64

func (mp multiPolyGeom) DistM(lon, lat, lonToM, latToM float64) float64 {
	min := math.MaxFloat64
	for _, p := range mp {
		if d := polyGeom(p).DistM(lon, lat, lonToM, latToM); d < min {
			min = d
		}
	}
	return min
}
func (mp multiPolyGeom) BBox() [4]float64 {
	if len(mp) == 0 {
		return [4]float64{}
	}
	b := polyGeom(mp[0]).BBox()
	for _, p := range mp[1:] {
		pb := polyGeom(p).BBox()
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

type multiGeom []Geom

func (mg multiGeom) DistM(lon, lat, lonToM, latToM float64) float64 {
	min := math.MaxFloat64
	for _, g := range mg {
		if d := g.DistM(lon, lat, lonToM, latToM); d < min {
			min = d
		}
	}
	return min
}
func (mg multiGeom) BBox() [4]float64 {
	if len(mg) == 0 {
		return [4]float64{}
	}
	b := mg[0].BBox()
	for _, g := range mg[1:] {
		gb := g.BBox()
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
