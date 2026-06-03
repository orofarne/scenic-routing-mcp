// Package scenic implements the scenic pedestrian routing algorithm.
package scenic

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/orofarne/scenic-routing-mcp/internal/format"
	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
	"github.com/orofarne/scenic-routing-mcp/internal/heatmap"
	"github.com/orofarne/scenic-routing-mcp/internal/ollama"
	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

// Params holds all inputs for the scenic pedestrian routing algorithm.
type Params struct {
	// Points is a list of [lat, lon] waypoints (minimum 2). The first point is
	// the start and the last is the destination. Any intermediate points are
	// forced waypoints the route must pass through.
	Points [][2]float64 `json:"points"`
	// PoiQuery describes what should be nearby using natural language.
	// Embedded into a vector for semantic similarity ranking.
	// Optional if PoiInclude or PoiNameQuery is provided.
	PoiQuery string `json:"poi_query,omitempty"`
	// PoiInclude selects features by OSM tag key=value pairs (AND semantics).
	// Value "*" matches any value (key existence check).
	PoiInclude map[string]string `json:"poi_include,omitempty"`
	// PoiExclude filters out features matching these OSM tag key=value pairs.
	// Value "*" excludes features where the key exists with any value.
	PoiExclude map[string]string `json:"poi_exclude,omitempty"`
	// PoiNameQuery performs fuzzy substring search over OSM name/description tags.
	PoiNameQuery string `json:"poi_name_query,omitempty"`
	// MaxDetourRatio caps the route length relative to the straight-line distance.
	// Default 1.5 (route may be at most 50% longer than the direct path).
	MaxDetourRatio float64 `json:"max_detour_ratio,omitempty"`
	// MinSimilarity is the minimum cosine similarity threshold for POI features.
	MinSimilarity float64 `json:"min_similarity,omitempty"`

	// Valhalla pedestrian costing options (all optional; 0 = use default).
	WalkwayFactor       float64 `json:"walkway_factor,omitempty"`
	PathFactor          float64 `json:"path_factor,omitempty"`
	UseTracks           float64 `json:"use_tracks,omitempty"`
	UseLivingStreets    float64 `json:"use_living_streets,omitempty"`
	UseHills            float64 `json:"use_hills,omitempty"`
	StepPenalty         float64 `json:"step_penalty,omitempty"`
	UseFerry            float64 `json:"use_ferry,omitempty"`
	MaxHikingDifficulty int     `json:"max_hiking_difficulty,omitempty"`
}

// Result holds the full output of the scenic routing algorithm.
type Result struct {
	BaseRoute              *valhalla.RouteResult
	ScenicRoute            *valhalla.RouteResult
	Features               []geodata.Feature
	MinLon, MinLat         float64
	MaxLon, MaxLat         float64
	HeatGrid               *heatmap.Grid
	Peaks                  []heatmap.Peak
	UsedPeaks              []heatmap.Peak
}

const (
	// scenicWeight is the fixed heatmap discount strength passed to Valhalla.
	// At 1.0 the maximum discount per edge is 10× (factor clamped to 0.1).
	scenicWeight = 1.0

	// routeHeatThreshold is the minimum average normalised heat score a scenic
	// route must achieve before we consider adding explicit peak waypoints.
	routeHeatThreshold = 0.20

	// bboxExpandM is the buffer added on each side of the route bbox.
	// 1500 m gives a generous corridor for heatmap POI discovery.
	bboxExpandM = 1500.0
)

// Plan runs the scenic routing algorithm and returns the full result.
func Plan(ctx context.Context, p Params, geo *geodata.Client, val *valhalla.Client, emb *ollama.Client) (*Result, error) {
	if p.MaxDetourRatio <= 0 {
		p.MaxDetourRatio = 1.5
	}

	costingOpts := pedestrianOpts(p)

	var poiVec []float32
	if p.PoiQuery != "" {
		t0 := time.Now()
		var err error
		poiVec, err = emb.EmbedOne(ctx, "search_query: "+p.PoiQuery)
		if err != nil {
			return nil, fmt.Errorf("embed: %w", err)
		}
		slog.DebugContext(ctx, "embed poi_query", "duration_ms", time.Since(t0).Milliseconds())
	}

	t0 := time.Now()
	baseRoute, err := val.Route(ctx, p.Points, costingOpts)
	if err != nil {
		return nil, fmt.Errorf("routing: %w", err)
	}
	slog.DebugContext(ctx, "valhalla route", "length_km", baseRoute.Trip.Summary.Length, "duration_ms", time.Since(t0).Milliseconds())

	coords := format.DecodeLegs(baseRoute.Trip.Legs)
	minLon, minLat, maxLon, maxLat := routeBBox(coords, bboxExpandM)

	t0 = time.Now()
	features, err := geo.Features(ctx, geodata.Query{
		MinLat: minLat, MinLon: minLon, MaxLat: maxLat, MaxLon: maxLon,
		QueryVec:   poiVec,
		TagInclude: p.PoiInclude,
		TagExclude: p.PoiExclude,
		NameQuery:  p.PoiNameQuery,
		MinSim:     p.MinSimilarity,
		Limit:      1000,
	})
	if err != nil {
		return nil, fmt.Errorf("features: %w", err)
	}
	slog.DebugContext(ctx, "features", "count", len(features), "duration_ms", time.Since(t0).Milliseconds())

	t0 = time.Now()
	heatGrid := heatmap.Compute(features, minLon, minLat, maxLon, maxLat, heatmap.Params{})
	slog.DebugContext(ctx, "heatmap", "width", heatGrid.Width, "height", heatGrid.Height, "duration_ms", time.Since(t0).Milliseconds())

	t0 = time.Now()
	scenicRoute, err := val.RouteScenic(ctx, p.Points, heatGrid, scenicWeight, costingOpts)
	if err != nil {
		slog.WarnContext(ctx, "scenic route (no peaks) failed, using baseline", "err", err)
		scenicRoute = baseRoute
	}
	noPeakScore := heatGrid.ScoreRoute(format.DecodeLegs(scenicRoute.Trip.Legs))
	slog.DebugContext(ctx, "scenic route",
		"peaks", 0,
		"heat_score", noPeakScore,
		"length_km", scenicRoute.Trip.Summary.Length,
		"duration_ms", time.Since(t0).Milliseconds())

	var peaks []heatmap.Peak
	var usedPeaks []heatmap.Peak
	if noPeakScore < routeHeatThreshold {
		t0 = time.Now()
		peaks = heatGrid.Peaks(10)
		slog.DebugContext(ctx, "heatmap peaks", "count", len(peaks), "duration_ms", time.Since(t0).Milliseconds())
	}
	if len(peaks) > 0 {
		matrixPts := make([][2]float64, 2+len(peaks))
		matrixPts[0] = p.Points[0]
		matrixPts[1] = p.Points[len(p.Points)-1]
		for i, pk := range peaks {
			matrixPts[2+i] = [2]float64{pk.Lat, pk.Lon}
		}
		t0 = time.Now()
		distMatrix, merr := val.Matrix(ctx, matrixPts, matrixPts)
		slog.DebugContext(ctx, "peak matrix", "peaks", len(peaks), "duration_ms", time.Since(t0).Milliseconds())
		if merr != nil {
			slog.WarnContext(ctx, "peak matrix failed, keeping no-peaks scenic route", "err", merr)
		} else {
			budgetKm := p.MaxDetourRatio * haversineKm(p.Points[0], p.Points[len(p.Points)-1])
			speedSPerKm := baseRoute.Trip.Summary.Time / baseRoute.Trip.Summary.Length
			budgetS := budgetKm * speedSPerKm
			heats := make([]float64, len(peaks))
			for i, pk := range peaks {
				heats[i] = pk.Heat
			}
			for _, nPeaks := range peakIterCounts(len(peaks)) {
				sub := make([][]float64, 2+nPeaks)
				for i := range sub {
					sub[i] = distMatrix[i][:2+nPeaks]
				}
				order := tspPeakOrder(nPeaks, sub, budgetS, heats[:nPeaks])
				if order == nil {
					continue
				}
				wps := make([][2]float64, 0, 2+len(order))
				wps = append(wps, p.Points[0])
				for _, idx := range order {
					wps = append(wps, [2]float64{peaks[idx].Lat, peaks[idx].Lon})
				}
				wps = append(wps, p.Points[len(p.Points)-1])
				t0 = time.Now()
				candidate, cerr := val.RouteScenic(ctx, wps, heatGrid, scenicWeight, costingOpts)
				if cerr != nil {
					slog.WarnContext(ctx, "scenic route with peaks failed", "n_peaks", nPeaks, "err", cerr)
					continue
				}
				score := heatGrid.ScoreRoute(format.DecodeLegs(candidate.Trip.Legs))
				slog.DebugContext(ctx, "scenic route",
					"peaks", nPeaks,
					"heat_score", score,
					"length_km", candidate.Trip.Summary.Length,
					"duration_ms", time.Since(t0).Milliseconds())
				scenicRoute = candidate
				cur := make([]heatmap.Peak, len(order))
				for i, idx := range order {
					cur[i] = peaks[idx]
				}
				usedPeaks = cur
				if score >= routeHeatThreshold {
					break
				}
			}
		}
	}

	return &Result{
		BaseRoute:   baseRoute,
		ScenicRoute: scenicRoute,
		Features:    features,
		MinLon:      minLon,
		MinLat:      minLat,
		MaxLon:      maxLon,
		MaxLat:      maxLat,
		HeatGrid:    heatGrid,
		Peaks:       peaks,
		UsedPeaks:   usedPeaks,
	}, nil
}

// pedestrianOpts builds the Valhalla costing_options map from the params,
// applying defaults where the caller did not specify a value.
func pedestrianOpts(p Params) map[string]any {
	walkway := p.WalkwayFactor
	if walkway == 0 {
		walkway = 0.75
	}
	pathFactor := p.PathFactor
	if pathFactor == 0 {
		pathFactor = 0.75
	}
	stepPenalty := p.StepPenalty
	if stepPenalty == 0 {
		stepPenalty = 10
	}
	opts := map[string]any{
		"walkway_factor": walkway,
		"path_factor":    pathFactor,
		"step_penalty":   stepPenalty,
		"use_ferry":      p.UseFerry, // our default is 0 = never; 0 value is fine to pass
	}
	if p.UseTracks > 0 {
		opts["use_tracks"] = p.UseTracks
	}
	if p.UseLivingStreets > 0 {
		opts["use_living_streets"] = p.UseLivingStreets
	}
	if p.UseHills > 0 {
		opts["use_hills"] = p.UseHills
	}
	if p.StepPenalty > 0 {
		opts["step_penalty"] = p.StepPenalty
	}
	if p.MaxHikingDifficulty > 0 {
		opts["max_hiking_difficulty"] = p.MaxHikingDifficulty
	}
	return opts
}

// routeBBox returns the bounding box of the route coords expanded by expandM
// metres on each side. coords are [lat, lon].
func routeBBox(coords [][2]float64, expandM float64) (minLon, minLat, maxLon, maxLat float64) {
	minLat, minLon = coords[0][0], coords[0][1]
	maxLat, maxLon = coords[0][0], coords[0][1]
	for _, c := range coords[1:] {
		if c[0] < minLat {
			minLat = c[0]
		}
		if c[0] > maxLat {
			maxLat = c[0]
		}
		if c[1] < minLon {
			minLon = c[1]
		}
		if c[1] > maxLon {
			maxLon = c[1]
		}
	}
	avgLat := (minLat + maxLat) / 2
	latDelta := expandM / 111320.0
	lonDelta := expandM / (111320.0 * math.Cos(avgLat*math.Pi/180.0))
	minLat -= latDelta
	maxLat += latDelta
	minLon -= lonDelta
	maxLon += lonDelta
	return
}

// haversineKm returns the great-circle distance in kilometres between two [lat,lon] points.
func haversineKm(a, b [2]float64) float64 {
	const R = 6371.0
	lat1, lon1 := a[0]*math.Pi/180, a[1]*math.Pi/180
	lat2, lon2 := b[0]*math.Pi/180, b[1]*math.Pi/180
	dlat, dlon := lat2-lat1, lon2-lon1
	s := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return R * 2 * math.Atan2(math.Sqrt(s), math.Sqrt(1-s))
}

// peakIterCounts returns the sequence of peak counts to try for the given
// total n, e.g. n=10 → [3, 6, 10], n=4 → [3, 4], n=2 → [2].
func peakIterCounts(n int) []int {
	steps := []int{3, 6, 10}
	var out []int
	for _, s := range steps {
		if s < n {
			out = append(out, s)
		}
	}
	if len(out) == 0 || out[len(out)-1] != n {
		out = append(out, n)
	}
	return out
}

// tspPeakOrder selects the subset of peaks with maximum total heat that fits
// within budgetS seconds of travel, and returns their indices in optimal
// visiting order (minimum time), using a bitmask DP.
//
// matrix layout: index 0 = start, 1 = end, 2..n+1 = peaks.
// Returns nil when n == 0 or no peak fits within the budget.
func tspPeakOrder(n int, matrix [][]float64, budgetS float64, heats []float64) []int {
	if n == 0 {
		return nil
	}
	const inf = math.MaxFloat64 / 2
	dp := make([][]float64, 1<<n)
	prev := make([][]int, 1<<n)
	for mask := range dp {
		dp[mask] = make([]float64, n)
		prev[mask] = make([]int, n)
		for i := range dp[mask] {
			dp[mask][i] = inf
			prev[mask][i] = -1
		}
	}
	for i := 0; i < n; i++ {
		dp[1<<i][i] = matrix[0][2+i]
	}
	for mask := 1; mask < 1<<n; mask++ {
		for i := 0; i < n; i++ {
			if mask&(1<<i) == 0 || dp[mask][i] == inf {
				continue
			}
			for j := 0; j < n; j++ {
				if mask&(1<<j) != 0 {
					continue
				}
				if c := dp[mask][i] + matrix[2+i][2+j]; c < dp[mask|(1<<j)][j] {
					dp[mask|(1<<j)][j] = c
					prev[mask|(1<<j)][j] = i
				}
			}
		}
	}

	// Precompute heat sums for all subsets.
	heatSum := make([]float64, 1<<n)
	for mask := 1; mask < 1<<n; mask++ {
		for b := 0; b < n; b++ {
			if mask&(1<<b) != 0 {
				heatSum[mask] += heats[b]
			}
		}
	}

	// Find the feasible subset (fits in budget) with maximum total heat.
	bestHeat, bestMask, bestLast := -1.0, 0, 0
	for mask := 1; mask < 1<<n; mask++ {
		for i := 0; i < n; i++ {
			if mask&(1<<i) == 0 || dp[mask][i] == inf {
				continue
			}
			if dp[mask][i]+matrix[2+i][1] > budgetS {
				continue
			}
			if h := heatSum[mask]; h > bestHeat {
				bestHeat, bestMask, bestLast = h, mask, i
			}
		}
	}
	if bestMask == 0 {
		return nil
	}

	order := make([]int, 0, n)
	mask, cur := bestMask, bestLast
	for {
		order = append(order, cur)
		prevMask := mask ^ (1 << cur)
		if prevMask == 0 {
			break
		}
		cur = prev[mask][cur]
		mask = prevMask
	}
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}
