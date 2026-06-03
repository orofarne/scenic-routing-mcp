// Package preview generates an interactive HTML route preview page.
package preview

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"math"

	"github.com/orofarne/scenic-routing-mcp/internal/format"
	"github.com/orofarne/scenic-routing-mcp/internal/scenic"
)

//go:embed page.html
var pageTmpl string

var tmpl = template.Must(template.New("preview").Parse(pageTmpl))

type pageData struct {
	Title     string
	DataJSON  template.JS
}

type routeData struct {
	Route     json.RawMessage `json:"route"`
	Start     [2]float64      `json:"start"`
	End       [2]float64      `json:"end"`
	Mid       [][2]float64    `json:"mid"`
	TilesURL  string          `json:"tilesURL"`
	TilesAttr string          `json:"tilesAttr"`
}

const osmTilesURL  = "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
const osmTilesAttr = `&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors`

// Page generates an HTML preview page for the given route.
func Page(params scenic.Params, result *scenic.Result, tilesURL, tilesAttr string) (string, error) {
	geojson, err := format.GeoJSON(result.ScenicRoute)
	if err != nil {
		return "", fmt.Errorf("encode geojson: %w", err)
	}

	if tilesURL == "" {
		tilesURL = osmTilesURL
		tilesAttr = osmTilesAttr
	}

	pts := params.Points
	var mid [][2]float64
	if len(pts) > 2 {
		mid = pts[1 : len(pts)-1]
	}

	rd := routeData{
		Route:     json.RawMessage(geojson),
		Start:     pts[0],
		End:       pts[len(pts)-1],
		Mid:       mid,
		TilesURL:  tilesURL,
		TilesAttr: tilesAttr,
	}
	dataJSON, err := json.Marshal(rd)
	if err != nil {
		return "", fmt.Errorf("marshal data: %w", err)
	}

	km := result.ScenicRoute.Trip.Summary.Length
	min := int(math.Round(result.ScenicRoute.Trip.Summary.Time / 60))
	title := fmt.Sprintf("Scenic route · %.1f km · ~%d min", km, min)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, pageData{
		Title:    title,
		DataJSON: template.JS(dataJSON),
	}); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	return buf.String(), nil
}
