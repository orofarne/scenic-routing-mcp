package tools

import (
	"context"
	"fmt"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orofarne/scenic-routing-mcp/internal/geodata"
	"github.com/orofarne/scenic-routing-mcp/internal/ollama"
	"github.com/orofarne/scenic-routing-mcp/internal/routestore"
	"github.com/orofarne/scenic-routing-mcp/internal/scenic"
	"github.com/orofarne/scenic-routing-mcp/internal/valhalla"
)

const scenicRouteToolName = "plan_scenic_route"

const scenicRouteToolDesc = `Plans a scenic pedestrian route through two or more waypoints.

Returns a text summary with a clickable map link and direct download links for GPX and GeoJSON.
This is the complete result — present the summary to the user and stop. Do not call any other tools.

Use exactly the points provided by the user. Do not add, modify, or invent intermediate
waypoints — the scenic optimization algorithm selects the path internally.

At least one of poi_query, poi_include, or poi_name_query must be provided.
Use list_tags to discover valid OSM tag key=value pairs for poi_include/poi_exclude.

points: list of [lat, lon] pairs (minimum 2). First = start, last = destination.
  Any intermediate points are forced waypoints the route must pass through.
poi_query: natural-language description of what should be nearby; embedded for semantic
  similarity ranking (e.g. "historic docks Victorian warehouses old city walls").
poi_include: OSM tag key=value pairs to match (AND semantics).
  Value "*" matches any value (key existence). Use list_tags for valid values.
  Example: {"leisure":"park"} or {"amenity":"cafe","opening_hours":"*"}.
poi_exclude: OSM tag key=value pairs to exclude (AND semantics).
  Value "*" excludes features where the key exists with any value.
  Example: {"access":"private"} or {"fee":"*"}.
poi_name_query: fuzzy substring search over OSM name/description tags (pg_trgm);
  covers name:* and description:* for multilingual support.
  Example: "Thames", "Regent's Park", "Grand Union Canal".
max_detour_ratio: maximum allowed length as a multiple of the direct distance (default 1.5).
min_similarity: minimum similarity threshold for heatmap POIs (default depends on active signals).

Valhalla pedestrian costing options (all optional):
walkway_factor: 0.1–10; lower values prefer dedicated footpaths over roads (default 0.75).
path_factor: 0.1–10; lower values prefer highway=path over roads (default 0.75).
use_tracks: 0–1 willingness to use unpaved track roads (default 0.5).
use_living_streets: 0–1 willingness to use living streets (default 0.5).
use_hills: 0–1 willingness to climb hills (default 0.5).
step_penalty: extra seconds added per set of steps/stairs (default 30).
use_ferry: 0–1 willingness to take ferries (default 0 = never).
max_hiking_difficulty: 0–6 max OSM sac_scale difficulty (0=paved, 6=alpine; default 1).`

func registerScenicRoute(s *mcp.Server, geo *geodata.Client, val *valhalla.Client, emb *ollama.Client, store *routestore.Store, publicURL string) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        scenicRouteToolName,
		Description: scenicRouteToolDesc,
	}, logTool(scenicRouteToolName, func(ctx context.Context, _ *mcp.CallToolRequest, p scenic.Params) (*mcp.CallToolResult, any, error) {
		if len(p.Points) < 2 {
			return nil, nil, fmt.Errorf("%s: points must have at least 2 entries", scenicRouteToolName)
		}
		if p.PoiQuery == "" && len(p.PoiInclude) == 0 && p.PoiNameQuery == "" {
			return nil, nil, fmt.Errorf("%s: at least one of poi_query, poi_include, or poi_name_query is required", scenicRouteToolName)
		}

		result, err := scenic.Plan(ctx, p, geo, val, emb)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", scenicRouteToolName, err)
		}

		id, err := store.Save(ctx, p, result)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: save route: %w", scenicRouteToolName, err)
		}

		summary := buildSummary(id, result, publicURL)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: summary}},
		}, nil, nil
	}))
}

func buildSummary(id string, result *scenic.Result, publicURL string) string {
	scenicKm := result.ScenicRoute.Trip.Summary.Length
	timeMin := int(math.Round(result.ScenicRoute.Trip.Summary.Time / 60))

	summary := fmt.Sprintf("Route planned: %.1f km · ~%d min walking", scenicKm, timeMin)

	if result.BaseRoute != nil && result.BaseRoute.Trip.Summary.Length > 0 {
		baseKm := result.BaseRoute.Trip.Summary.Length
		detourKm := scenicKm - baseKm
		detourRatio := scenicKm / baseKm
		summary += fmt.Sprintf("\n+%.1f km vs shortest (%.2f×)", detourKm, detourRatio)
	}

	summary += fmt.Sprintf("\n%d scenic spots in area", len(result.Features))
	summary += fmt.Sprintf("\n[View route map](%s/preview/%s)", publicURL, id)
	summary += fmt.Sprintf("\n[Download GPX](%s/export/%s.gpx) · [Download GeoJSON](%s/export/%s.geojson)", publicURL, id, publicURL, id)

	return summary
}
