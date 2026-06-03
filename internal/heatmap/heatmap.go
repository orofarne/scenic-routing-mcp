// Package heatmap builds a soft-maximum kernel density raster over a
// geographic bounding box from a set of scored POI features.
//
// Heat at each grid cell = max_i( sim_i^4 × (1 − d/cutoff_i)² )
//
// Quadratic kernel: 1 inside the object (d=0), drops to 0 at d=3σ.
// Squaring sharpens the falloff vs linear while keeping the same radius:
// at d=0.5×cutoff the value is 0.25 (vs 0.5 for linear).
// Plain max: the dominant nearest feature wins. Dense clusters don't accumulate.
// σ is fixed at SigmaM for all geometry types: equal similarity = equal peak
// weight and equal influence radius, regardless of feature size.
//
// The grid is rendered as a blue→cyan→yellow→red PNG with alpha proportional
// to heat, suitable for transparent overlay on a map.
package heatmap

import (
	"fmt"
	"math"
	"sort"

	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
)

const (
	defaultResolutionM = 50.0
	defaultSigmaM      = 150.0
	metersPerDegLat    = 111320.0
)

// Params configures heatmap generation. Zero values use defaults.
type Params struct {
	ResolutionM float64 // grid cell size in metres (default 50)
	SigmaM   float64 // fixed Gaussian σ in metres (default 150)
}

func (p *Params) setDefaults() {
	if p.ResolutionM <= 0 {
		p.ResolutionM = defaultResolutionM
	}
	if p.SigmaM <= 0 {
		p.SigmaM = defaultSigmaM
	}
}

// Grid holds the computed heat values and their spatial reference.
// Values[y][x]: y=0 is north (maxLat), x=0 is west (minLon).
type Grid struct {
	Values           [][]float64
	Width, Height    int
	MinLon, MinLat   float64
	MaxLon, MaxLat   float64
	CellLon, CellLat float64 // degrees per cell
	ResolutionM      float64
}

// Compute builds a heat grid from features over the given bbox.
func Compute(features []geodata.Feature, minLon, minLat, maxLon, maxLat float64, p Params) *Grid {
	p.setDefaults()

	avgLat := (minLat + maxLat) / 2
	cosLat := math.Cos(avgLat * math.Pi / 180)
	lonToM := metersPerDegLat * cosLat
	latToM := metersPerDegLat

	cellLat := p.ResolutionM / latToM
	cellLon := p.ResolutionM / lonToM

	width := max(1, int(math.Ceil((maxLon-minLon)/cellLon)))
	height := max(1, int(math.Ceil((maxLat-minLat)/cellLat)))

	// Initialize to -Inf: log-sum-exp identity element (log(0)).
	values := make([][]float64, height)
	for y := range values {
		values[y] = make([]float64, width)
		for x := range values[y] {
			values[y][x] = math.Inf(-1)
		}
	}

	g := &Grid{
		Values:      values,
		Width:       width,
		Height:      height,
		MinLon:      minLon,
		MinLat:      minLat,
		MaxLon:      maxLon,
		MaxLat:      maxLat,
		CellLon:     cellLon,
		CellLat:     cellLat,
		ResolutionM: p.ResolutionM,
	}

	for _, f := range features {
		if f.Similarity <= 0 {
			continue
		}

		if len(f.Geom) == 0 {
			continue
		}
		geom := parseGeom(f.Geom)
		if geom == nil {
			continue
		}

		sigmaM := p.SigmaM
		weight := math.Pow(f.Similarity, 4)

		cutoffM := 3 * sigmaM
		invCutoffM := 1.0 / cutoffM
		cutoffLon := cutoffM / lonToM
		cutoffLat := cutoffM / latToM

		fb := geom.bbox()
		x0 := clampInt(g.lonToX(fb[0]-cutoffLon), 0, width-1)
		x1 := clampInt(g.lonToX(fb[2]+cutoffLon), 0, width-1)
		y0 := clampInt(g.latToY(fb[3]+cutoffLat), 0, height-1)
		y1 := clampInt(g.latToY(fb[1]-cutoffLat), 0, height-1)

		for y := y0; y <= y1; y++ {
			lat := maxLat - (float64(y)+0.5)*cellLat
			for x := x0; x <= x1; x++ {
				lon := minLon + (float64(x)+0.5)*cellLon
				dM := geom.distM(lon, lat, lonToM, latToM)
				if dM >= cutoffM {
					continue
				}
				t := 1.0 - dM*invCutoffM
				values[y][x] = math.Max(values[y][x], weight*t*t)
			}
		}
	}

	return g
}

// Encode normalizes the grid to uint8 (row-major, y=0=north) using the same
// 95th-percentile ceiling as PNG rendering, without gamma correction.
// The result is suitable for passing to Valhalla's scenic_pedestrian costing.
func (g *Grid) Encode() []byte {
	var nonzero []float64
	for _, row := range g.Values {
		for _, v := range row {
			if !math.IsInf(v, -1) {
				nonzero = append(nonzero, v)
			}
		}
	}
	ceiling := 0.0
	if len(nonzero) > 0 {
		sort.Float64s(nonzero)
		ceiling = nonzero[int(float64(len(nonzero))*0.95)]
		if ceiling <= 0 {
			ceiling = nonzero[len(nonzero)-1]
		}
	}
	out := make([]byte, g.Width*g.Height)
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			t := 0.0
			v := g.Values[y][x]
			if ceiling > 0 && !math.IsInf(v, -1) {
				t = math.Max(0, math.Min(v/ceiling, 1.0))
			}
			out[y*g.Width+x] = byte(t * 255)
		}
	}
	return out
}

// Peak is a representative heatmap waypoint for routing.
type Peak struct {
	Lat, Lon float64
	Heat     float64 // normalised [0,1]
}

// Peaks parameters.
const (
	// peakFlatGini is the Gini threshold below which the heatmap is considered
	// too uniformly distributed to need explicit waypoints. Computed over all
	// grid cells (zeros included): a concentrated river corridor gives Gini ≈ 0.85,
	// a uniform park layer gives Gini ≈ 0.40.
	peakFlatGini = 0.50
	// peakHotFrac: cells above this percentile of non-zero values are "hot".
	peakHotFrac = 0.75
	// peakSpacingM is the target distance between peaks within one elongated component.
	peakSpacingM = 500.0
	// peakMinSepM is the minimum distance between any two returned peaks.
	peakMinSepM = 300.0
	// peakMaxPerComp caps peaks generated from a single connected component.
	peakMaxPerComp = 4
)

// Peaks returns up to n representative waypoints from the heatmap using
// connected-component analysis with PCA-based splitting for elongated features.
//
// Algorithm:
//  1. Compute the Gini coefficient over all cells (zeros included). A low Gini
//     means heat is spread uniformly — explicit waypoints add no value, return nil.
//  2. Threshold at the top (1−peakHotFrac) of non-zero cells → flood-fill
//     8-connected components.
//  3. For each component find the principal axis via weighted PCA, project cells
//     onto it, and divide into k = round(extent/peakSpacingM) buckets. Each
//     bucket contributes one peak at its weighted centroid.
//  4. Sort candidates by heat, apply peakMinSepM suppression, return top n.
func (g *Grid) Peaks(n int) []Peak {
	total := g.Width * g.Height

	// Build flat value array (−Inf → 0) and collect non-zero values for threshold.
	flat := make([]float64, total)
	var nonzero []float64
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			v := g.Values[y][x]
			if !math.IsInf(v, -1) && v > 0 {
				flat[y*g.Width+x] = v
				nonzero = append(nonzero, v)
			}
		}
	}
	if len(nonzero) < 4 {
		return nil
	}

	// Gini coefficient over all cells (including zeros).
	// G = (2·Σᵢ(i+1)·vᵢ − (n+1)·Σvᵢ) / (n·Σvᵢ), values sorted ascending.
	sorted := make([]float64, total)
	copy(sorted, flat)
	sort.Float64s(sorted)
	var sumV, sumWeighted float64
	for i, v := range sorted {
		sumV += v
		sumWeighted += float64(i+1) * v
	}
	if sumV == 0 {
		return nil
	}
	gini := (2*sumWeighted/sumV - float64(total+1)) / float64(total)
	if gini < peakFlatGini {
		return nil
	}

	// Hot threshold: top (1 − peakHotFrac) of non-zero cells.
	sort.Float64s(nonzero)
	threshold := nonzero[int(float64(len(nonzero))*peakHotFrac)]

	// Build hot mask.
	hot := make([][]bool, g.Height)
	for y := range hot {
		hot[y] = make([]bool, g.Width)
		for x := 0; x < g.Width; x++ {
			if g.Values[y][x] >= threshold {
				hot[y][x] = true
			}
		}
	}

	avgLat := (g.MinLat + g.MaxLat) / 2
	lonToM := metersPerDegLat * math.Cos(avgLat*math.Pi/180.0)

	type cell struct{ y, x int }
	visited := make([][]bool, g.Height)
	for y := range visited {
		visited[y] = make([]bool, g.Width)
	}

	type candidate struct{ lat, lon, heat float64 }
	var cands []candidate

	for sy := 0; sy < g.Height; sy++ {
		for sx := 0; sx < g.Width; sx++ {
			if !hot[sy][sx] || visited[sy][sx] {
				continue
			}
			// BFS — collect 8-connected component.
			var comp []cell
			queue := []cell{{sy, sx}}
			visited[sy][sx] = true
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				comp = append(comp, cur)
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						ny, nx := cur.y+dy, cur.x+dx
						if ny < 0 || ny >= g.Height || nx < 0 || nx >= g.Width {
							continue
						}
						if !hot[ny][nx] || visited[ny][nx] {
							continue
						}
						visited[ny][nx] = true
						queue = append(queue, cell{ny, nx})
					}
				}
			}

			// Weighted centroid in metre space.
			var totalW, cx, cy float64
			for _, c := range comp {
				w := g.Values[c.y][c.x]
				lon := g.MinLon + (float64(c.x)+0.5)*g.CellLon
				lat := g.MaxLat - (float64(c.y)+0.5)*g.CellLat
				cx += w * lon * lonToM
				cy += w * lat * metersPerDegLat
				totalW += w
			}
			if totalW == 0 {
				continue
			}
			cx /= totalW
			cy /= totalW

			// Weighted covariance matrix for PCA.
			var cxx, cxy, cyy float64
			for _, c := range comp {
				w := g.Values[c.y][c.x]
				lon := g.MinLon + (float64(c.x)+0.5)*g.CellLon
				lat := g.MaxLat - (float64(c.y)+0.5)*g.CellLat
				dx := lon*lonToM - cx
				dy := lat*metersPerDegLat - cy
				cxx += w * dx * dx
				cxy += w * dx * dy
				cyy += w * dy * dy
			}
			cxx /= totalW
			cxy /= totalW
			cyy /= totalW

			// Principal axis: largest eigenvector of 2×2 covariance matrix.
			hdiff := (cxx - cyy) / 2
			disc := math.Sqrt(hdiff*hdiff + cxy*cxy)
			axX, axY := cxy, disc-hdiff
			if l := math.Sqrt(axX*axX + axY*axY); l > 1e-9 {
				axX /= l
				axY /= l
			} else {
				axX, axY = 1, 0
			}

			// Project cells onto principal axis; find range.
			type pCell struct {
				proj     float64
				lat, lon float64
				heat     float64
			}
			pcells := make([]pCell, len(comp))
			minP, maxP := math.MaxFloat64, -math.MaxFloat64
			for i, c := range comp {
				lon := g.MinLon + (float64(c.x)+0.5)*g.CellLon
				lat := g.MaxLat - (float64(c.y)+0.5)*g.CellLat
				p := (lon*lonToM-cx)*axX + (lat*metersPerDegLat-cy)*axY
				pcells[i] = pCell{p, lat, lon, g.Values[c.y][c.x]}
				if p < minP {
					minP = p
				}
				if p > maxP {
					maxP = p
				}
			}
			extentM := maxP - minP

			// Number of peaks for this component.
			k := 1
			if extentM > 1e-9 {
				k = max(1, min(peakMaxPerComp, int(math.Round(extentM/peakSpacingM))))
			}

			// Two-pass bucket scoring:
			//   Pass 1 — weighted centroid per bucket.
			//   Pass 2 — score each cell as heat × exp(-d²/2σ²) where d is distance
			//            to the bucket centroid. This prefers cells that are both hot
			//            AND near the centre of the bucket, avoiding boundary artefacts
			//            while still snapping to a real heat maximum.
			type bucketState struct {
				totalW              float64
				n                   int
				centLonM, centLatM  float64 // weighted centroid in metre coords
				bestScore           float64
				bestLat, bestLon    float64
				bestHeat            float64
			}
			buckets := make([]bucketState, k)

			bucketOf := func(pc pCell) int {
				if extentM <= 1e-9 {
					return 0
				}
				b := int((pc.proj - minP) / extentM * float64(k))
				if b >= k {
					b = k - 1
				}
				return b
			}

			// Pass 1: accumulate weighted centroid.
			for _, pc := range pcells {
				b := bucketOf(pc)
				buckets[b].totalW += pc.heat
				buckets[b].n++
				buckets[b].centLonM += pc.heat * pc.lon * lonToM
				buckets[b].centLatM += pc.heat * pc.lat * metersPerDegLat
			}
			bucketExtentM := extentM / float64(k)
			sigmaM := math.Max(bucketExtentM*0.4, g.ResolutionM*2)
			inv2s2 := 1.0 / (2 * sigmaM * sigmaM)
			for b := range buckets {
				if buckets[b].totalW > 0 {
					buckets[b].centLonM /= buckets[b].totalW
					buckets[b].centLatM /= buckets[b].totalW
				}
			}

			// Pass 2: find the cell maximising heat × gaussian(dist_to_centroid).
			for _, pc := range pcells {
				b := bucketOf(pc)
				dx := pc.lon*lonToM - buckets[b].centLonM
				dy := pc.lat*metersPerDegLat - buckets[b].centLatM
				score := pc.heat * math.Exp(-(dx*dx+dy*dy)*inv2s2)
				if score > buckets[b].bestScore {
					buckets[b].bestScore = score
					buckets[b].bestLat = pc.lat
					buckets[b].bestLon = pc.lon
					buckets[b].bestHeat = pc.heat
				}
			}
			for b := range buckets {
				if buckets[b].n == 0 {
					continue
				}
				cands = append(cands, candidate{
					lat:  buckets[b].bestLat,
					lon:  buckets[b].bestLon,
					heat: buckets[b].totalW / float64(buckets[b].n),
				})
			}
		}
	}

	if len(cands) == 0 {
		return nil
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].heat > cands[j].heat })
	maxHeat := cands[0].heat

	var peaks []Peak
	for _, c := range cands {
		if len(peaks) >= n {
			break
		}
		tooClose := false
		for _, p := range peaks {
			dlat := (c.lat - p.Lat) * metersPerDegLat
			dlon := (c.lon - p.Lon) * lonToM
			if math.Sqrt(dlat*dlat+dlon*dlon) < peakMinSepM {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		heat := 0.0
		if maxHeat > 0 {
			heat = c.heat / maxHeat
		}
		peaks = append(peaks, Peak{Lat: c.lat, Lon: c.lon, Heat: heat})
	}
	return peaks
}

// ScoreRoute returns the average normalised heat along a route.
// coords are [lat, lon] pairs. Points are sampled every ~50 m.
// Returns 0 for an empty grid or empty route.
func (g *Grid) ScoreRoute(coords [][2]float64) float64 {
	if len(coords) == 0 {
		return 0
	}

	var nonzero []float64
	for _, row := range g.Values {
		for _, v := range row {
			if !math.IsInf(v, -1) {
				nonzero = append(nonzero, v)
			}
		}
	}
	if len(nonzero) == 0 {
		return 0
	}
	sort.Float64s(nonzero)
	ceiling := nonzero[int(float64(len(nonzero))*0.95)]
	if ceiling <= 0 {
		ceiling = nonzero[len(nonzero)-1]
	}
	if ceiling <= 0 {
		return 0
	}

	const sampleEveryM = 50.0
	var sum float64
	var n int
	var accumulated float64

	for i, c := range coords {
		if i > 0 {
			prev := coords[i-1]
			dlat := (c[0] - prev[0]) * metersPerDegLat
			cosLat := math.Cos(prev[0] * math.Pi / 180)
			dlon := (c[1] - prev[1]) * metersPerDegLat * cosLat
			accumulated += math.Sqrt(dlat*dlat + dlon*dlon)
		}
		if i == 0 || accumulated >= sampleEveryM || i == len(coords)-1 {
			x := clampInt(g.lonToX(c[1]), 0, g.Width-1)
			y := clampInt(g.latToY(c[0]), 0, g.Height-1)
			v := g.Values[y][x]
			if !math.IsInf(v, -1) {
				sum += math.Max(0, math.Min(v/ceiling, 1.0))
			}
			n++
			if accumulated >= sampleEveryM {
				accumulated = 0
			}
		}
	}

	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// BoundsJSON returns a JSON string describing the geographic extent and
// resolution of the grid, for use as georeferencing metadata.
func (g *Grid) BoundsJSON() string {
	return fmt.Sprintf(
		`{"type":"heatmap_bounds","minLon":%g,"minLat":%g,"maxLon":%g,"maxLat":%g,"width":%d,"height":%d,"resolution_m":%g}`,
		g.MinLon, g.MinLat, g.MaxLon, g.MaxLat, g.Width, g.Height, g.ResolutionM,
	)
}

func (g *Grid) lonToX(lon float64) int { return int((lon - g.MinLon) / g.CellLon) }
func (g *Grid) latToY(lat float64) int { return int((g.MaxLat - lat) / g.CellLat) }

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
