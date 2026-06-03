package docgen

import (
	"math"

	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
	"github.com/orofarne/scenic-routing-mcp/internal/geom"
	"github.com/orofarne/scenic-routing-mcp/internal/heatmap"
)

// computeVariant builds a heat grid with pluggable kernel, aggregation, and
// similarity weight functions. All cells start at 0 (not −∞).
// Mirrors heatmap.Compute but with pluggable behaviour for algorithm comparison.
func computeVariant(
	features []geodata.Feature,
	minLon, minLat, maxLon, maxLat float64,
	p heatmap.Params,
	simWeight func(float64) float64,
	kernel func(dM, cutoffM float64) float64,
	agg func(cur, cand float64) float64,
) *heatmap.Grid {
	// Inline Params defaults.
	if p.ResolutionM <= 0 {
		p.ResolutionM = 50
	}
	if p.SigmaM <= 0 {
		p.SigmaM = 150
	}

	avgLat := (minLat + maxLat) / 2
	cosLat := math.Cos(avgLat * math.Pi / 180)
	lonToM := metersPerDegLat * cosLat
	latToM := metersPerDegLat

	cellLat := p.ResolutionM / latToM
	cellLon := p.ResolutionM / lonToM
	width := max(1, int(math.Ceil((maxLon-minLon)/cellLon)))
	height := max(1, int(math.Ceil((maxLat-minLat)/cellLat)))

	values := make([][]float64, height)
	for y := range values {
		values[y] = make([]float64, width)
	}
	g := &heatmap.Grid{
		Values: values, Width: width, Height: height,
		MinLon: minLon, MinLat: minLat, MaxLon: maxLon, MaxLat: maxLat,
		CellLon: cellLon, CellLat: cellLat, ResolutionM: p.ResolutionM,
	}

	cutoffM := 3 * p.SigmaM
	cutoffLon := cutoffM / lonToM
	cutoffLat := cutoffM / latToM

	lonToX := func(lon float64) int { return int((lon - g.MinLon) / g.CellLon) }
	latToY := func(lat float64) int { return int((g.MaxLat - lat) / g.CellLat) }

	for _, f := range features {
		if f.Similarity <= 0 || len(f.Geom) == 0 {
			continue
		}
		feat := geom.Parse(f.Geom)
		if feat == nil {
			continue
		}
		w := simWeight(f.Similarity)

		fb := feat.BBox()
		x0 := clampInt(lonToX(fb[0]-cutoffLon), 0, width-1)
		x1 := clampInt(lonToX(fb[2]+cutoffLon), 0, width-1)
		y0 := clampInt(latToY(fb[3]+cutoffLat), 0, height-1)
		y1 := clampInt(latToY(fb[1]-cutoffLat), 0, height-1)

		for y := y0; y <= y1; y++ {
			lat := maxLat - (float64(y)+0.5)*cellLat
			for x := x0; x <= x1; x++ {
				lon := minLon + (float64(x)+0.5)*cellLon
				dM := feat.DistM(lon, lat, lonToM, latToM)
				if dM >= cutoffM {
					continue
				}
				values[y][x] = agg(values[y][x], w*kernel(dM, cutoffM))
			}
		}
	}
	return g
}
