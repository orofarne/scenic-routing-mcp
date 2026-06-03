package valhalla

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/orofarne/scenic-routing-mcp/internal/heatmap"
)

// Location is a lat/lon pair used by Valhalla requests.
// Type controls routing behaviour: "break" (default, allows U-turns) or
// "through" (pass-through, no forced stop or U-turn).
type Location struct {
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Type string  `json:"type,omitempty"`
}

// Leg is a single segment of a Valhalla route response.
type Leg struct {
	Summary struct {
		Length float64 `json:"length"` // km
		Time   float64 `json:"time"`   // seconds
	} `json:"summary"`
	Shape string `json:"shape"` // encoded polyline
}

// RouteResult holds the decoded Valhalla /route response.
type RouteResult struct {
	Trip struct {
		Legs    []Leg   `json:"legs"`
		Summary struct {
			Length float64 `json:"length"` // km
			Time   float64 `json:"time"`   // seconds
		} `json:"summary"`
	} `json:"trip"`
}

// Client calls the Valhalla HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client targeting the given Valhalla base URL (e.g. "http://valhalla:8002").
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// makeLocations converts lat/lon pairs to Valhalla Location objects.
// Start and end are "break" (hard stops); intermediates are "through" (no U-turn).
func makeLocations(waypoints [][2]float64) []Location {
	locs := make([]Location, len(waypoints))
	for i, wp := range waypoints {
		t := "through"
		if i == 0 || i == len(waypoints)-1 {
			t = "break"
		}
		locs[i] = Location{Lat: wp[0], Lon: wp[1], Type: t}
	}
	return locs
}

// Route requests a pedestrian route through the given waypoints (lat/lon pairs).
// costingOpts is merged into costing_options.pedestrian (caller sets walkway_factor etc.).
func (c *Client) Route(ctx context.Context, waypoints [][2]float64, costingOpts map[string]any) (*RouteResult, error) {
	body := map[string]any{
		"locations": makeLocations(waypoints),
		"costing":   "pedestrian",
		"costing_options": map[string]any{
			"pedestrian": costingOpts,
		},
		"directions_options": map[string]any{"units": "km"},
	}
	return c.postRoute(ctx, body)
}

// RouteScenic routes using the scenic_pedestrian costing, which discounts edges
// in areas with high heatmap density. weight controls the discount strength:
// 0 = no effect (pure pedestrian), 1 = maximum discount (edges 10× cheaper).
// costingOpts are shared with the pedestrian base costing (walkway_factor etc.).
func (c *Client) RouteScenic(ctx context.Context, waypoints [][2]float64, grid *heatmap.Grid, weight float64, costingOpts map[string]any) (*RouteResult, error) {
	raw := grid.Encode()
	opts := make(map[string]any, len(costingOpts)+8)
	for k, v := range costingOpts {
		opts[k] = v
	}
	opts["heatmap_min_lat"] = grid.MinLat
	opts["heatmap_min_lon"] = grid.MinLon
	opts["heatmap_max_lat"] = grid.MaxLat
	opts["heatmap_max_lon"] = grid.MaxLon
	opts["heatmap_width"] = grid.Width
	opts["heatmap_height"] = grid.Height
	opts["heatmap_data"] = base64.StdEncoding.EncodeToString(raw)
	opts["heatmap_scenic_weight"] = weight
	body := map[string]any{
		"locations": makeLocations(waypoints),
		"costing":   "scenic_pedestrian",
		"costing_options": map[string]any{
			"scenic_pedestrian": opts,
		},
		"directions_options": map[string]any{"units": "km"},
	}
	return c.postRoute(ctx, body)
}

// Matrix returns a many-to-many time/distance matrix between sources and targets.
// The returned slice is [len(sources)][len(targets)] seconds.
func (c *Client) Matrix(ctx context.Context, sources, targets [][2]float64) ([][]float64, error) {
	toLocs := func(pts [][2]float64) []Location {
		out := make([]Location, len(pts))
		for i, p := range pts {
			out[i] = Location{Lat: p[0], Lon: p[1]}
		}
		return out
	}
	body := map[string]any{
		"sources": toLocs(sources),
		"targets": toLocs(targets),
		"costing": "pedestrian",
		"costing_options": map[string]any{
			"pedestrian": map[string]any{
				"walkway_factor": 0.75,
				"use_ferry":      0,
			},
		},
	}
	data, err := c.post(ctx, "/sources_to_targets", body)
	if err != nil {
		return nil, err
	}

	var resp struct {
		SourcesToTargets [][]struct {
			Time float64 `json:"time"`
		} `json:"sources_to_targets"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("valhalla: matrix decode: %w", err)
	}
	result := make([][]float64, len(resp.SourcesToTargets))
	for i, row := range resp.SourcesToTargets {
		result[i] = make([]float64, len(row))
		for j, cell := range row {
			result[i][j] = cell.Time
		}
	}
	return result, nil
}

func (c *Client) postRoute(ctx context.Context, body any) (*RouteResult, error) {
	data, err := c.post(ctx, "/route", body)
	if err != nil {
		return nil, err
	}
	var result RouteResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("valhalla: route decode: %w", err)
	}
	return &result, nil
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("valhalla: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("valhalla: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("valhalla: %s: %w", path, err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("valhalla: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("valhalla: %s returned %d: %s", path, resp.StatusCode, buf.String())
	}
	return buf.Bytes(), nil
}
