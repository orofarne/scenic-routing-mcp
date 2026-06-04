package docgen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	raTileCache  = "/tmp/scenic_tile_cache"
	raTilePixels = 256
	raUserAgent  = "scenic-routing-mcp/docgen (+https://github.com/orofarne/scenic-routing-mcp)"
	raMCPBase    = "http://localhost:8080"
	// Fallback tile provider and attribution when MAP_TILES_URL / MAP_TILES_ATTR are unset.
	raDefaultTile = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	raDefaultAttr = "(c) OpenStreetMap contributors | ODbL"
)

// TestRoutingApproaches generates map-tile screenshots for docs/algorithm.md §1.
//
// Two cities, four images each (docs/img/):
//
//	approach_baseline_{city}.png  — §1.1 plain shortest path
//	approach_waypoints_{city}.png — §1.2 candidate peak waypoints
//	approach_heatmap_{city}.png   — §1.3 heat map field + baseline route
//	approach_scenic_{city}.png    — §1.4 heat map + scenic route + peaks
//
// Run via:
//
//	make screenshots
//
// Requires the full stack at localhost:8080 and internet access for map tiles.
// Tile URL and attribution are read from .env (MAP_TILES_URL / MAP_TILES_ATTR).
// Tiles are cached in /tmp/scenic_tile_cache between runs.
func TestRoutingApproaches(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode: requires running MCP server and map tile access")
	}

	tileURLTmpl := os.Getenv("MAP_TILES_URL")
	if tileURLTmpl == "" {
		tileURLTmpl = raDefaultTile
	}
	tilesAttr := os.Getenv("MAP_TILES_ATTR")
	if tilesAttr == "" {
		tilesAttr = raDefaultAttr
	}
	attribText := raStripHTML(tilesAttr)

	outDir := filepath.Join("..", "img")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	type routeConfig struct {
		name     string
		zoom     int
		padTiles int
		args     map[string]any
	}

	configs := []routeConfig{
		{
			name:     "yerevan",
			zoom:     13,
			padTiles: 1,
			// heat_score ~0.32 < defaultHeatThreshold 0.4 → peaks triggered naturally.
			args: map[string]any{
				"points":           [][2]float64{{40.18055, 44.5287}, {40.20084, 44.49306}},
				"poi_query":        "rivers, canals, ponds",
				"max_detour_ratio": 2.0,
			},
		},
		{
			name:     "london",
			zoom:     12,
			padTiles: 1,
			args: map[string]any{
				"points":           [][2]float64{{51.45912, -0.30602}, {51.48266, -0.14424}},
				"poi_query":        "rivers, canals, ponds",
				"max_detour_ratio": 2.0,
			},
		},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			raGenerateRouteImages(t, ctx, cfg.name, cfg.zoom, cfg.padTiles, cfg.args, tileURLTmpl, attribText, outDir)
		})
	}
}

// raGenerateRouteImages plans three routes and writes four PNG files for §1.1–§1.4.
//
//   - Plan A (min_heat_score=0.99): peaks always forced — for §1.1 (baseline) and §1.2.
//   - Plan B (min_heat_score=0.001): peaks never triggered — for §1.3 (pure heatmap route).
//   - Plan C (default min_heat_score=0.4): natural behavior — for §1.4 (combined approach).
//     Yerevan scores ~0.32 → peaks used. London scores ~0.93 → no peaks needed.
func raGenerateRouteImages(t *testing.T, ctx context.Context, name string, zoom, padTiles int, args map[string]any, tileURLTmpl, attribText, outDir string) {
	t.Helper()

	withMHS := func(mhs float64) map[string]any {
		m := make(map[string]any, len(args)+1)
		for k, v := range args {
			m[k] = v
		}
		m["min_heat_score"] = mhs
		return m
	}

	plan := func(label string, planArgs map[string]any) ([][2]float64, [][2]float64, *heatmapLayer, [][3]float64, int) {
		t.Helper()
		t.Logf("[%s] plan_scenic_route (%s)...", name, label)
		id, err := raCallPlan(ctx, raMCPBase+"/mcp", planArgs)
		if err != nil {
			t.Skipf("MCP server unavailable: %v", err)
		}
		t.Logf("[%s] route (%s): %s", name, label, id)
		fc, err := raFetchDebugFC(ctx, raMCPBase+"/debug/"+id)
		if err != nil {
			t.Fatalf("[%s] debug fetch (%s): %v", name, label, err)
		}
		base := raRouteCoords(fc, "baseline")
		sc := raRouteCoords(fc, "scenic")
		if len(base) == 0 || len(sc) == 0 {
			t.Fatalf("[%s] missing route (%s)", name, label)
		}
		wps, used := raAllPeaks(fc)
		t.Logf("[%s] (%s) peaks total: %d, used: %d", name, label, len(wps), used)
		return base, sc, raHeatmapLayer(fc), wps, used
	}

	// Plan A: forced peaks (§1.1 baseline + §1.2).
	baseline, scenicForced, heatLayer, waypointsForced, _ := plan("forced", withMHS(0.99))

	// Plan B: heatmap-only route (§1.3).
	_, scenicHeat, _, _, _ := plan("heatmap", withMHS(0.001))

	// Plan C: natural default (§1.4).
	_, scenicNatural, _, waypointsNatural, _ := plan("natural", args)

	all := append(append([][2]float64{}, baseline...), scenicForced...)
	all = append(all, scenicHeat...)
	all = append(all, scenicNatural...)
	minLon, minLat, maxLon, maxLat := raBBox(all)

	t.Logf("[%s] downloading map tiles (zoom %d)...", name, zoom)
	canvas, err := raBuildCanvas(minLon, minLat, maxLon, maxLat, zoom, padTiles, tileURLTmpl)
	if err != nil {
		t.Fatalf("[%s] tile canvas: %v", name, err)
	}

	const margin = 150
	cropRect := raPixelBBox(canvas, baseline, scenicForced, scenicHeat, scenicNatural).Inset(-margin).Intersect(canvas.img.Bounds())

	startLon, startLat := baseline[0][0], baseline[0][1]
	endLon, endLat := baseline[len(baseline)-1][0], baseline[len(baseline)-1][1]

	save := func(suffix string, img image.Image) {
		t.Helper()
		fname := fmt.Sprintf("approach_%s_%s.png", suffix, name)
		f, err := os.Create(filepath.Join(outDir, fname))
		if err != nil {
			t.Fatalf("create %s: %v", fname, err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatalf("encode %s: %v", fname, err)
		}
		t.Logf("wrote docs/img/%s", fname)
	}

	drawPeakSet := func(dst *image.RGBA, wps [][3]float64) {
		for _, wp := range wps {
			wx, wy := canvas.project(wp[0], wp[1])
			if wp[2] == 1 {
				raFillCircle(dst, wx, wy, 13, color.RGBA{255, 255, 255, 255})
				raFillCircle(dst, wx, wy, 10, color.RGBA{240, 140, 20, 255})
			} else {
				raFillCircle(dst, wx, wy, 10, color.RGBA{255, 255, 255, 200})
				raFillCircle(dst, wx, wy, 7, color.RGBA{220, 120, 20, 200})
			}
		}
	}

	// ── §1.1 Plain Shortest Path ─────────────────────────────────────────────
	{
		img := raCopyRGBA(canvas.img)
		canvas.drawPolyline(img, baseline, color.RGBA{50, 50, 50, 255}, 5)
		raEndMarkers(canvas, img, startLon, startLat, endLon, endLat)
		save("baseline", raWithAttrib(raCropImage(img, cropRect), attribText))
	}

	// ── §1.2 Intermediate Waypoints ──────────────────────────────────────────
	// Uses forced plan (min_heat_score=0.99) to always show candidate peaks.
	{
		img := raCopyRGBA(canvas.img)
		canvas.drawPolyline(img, scenicForced, color.RGBA{30, 100, 220, 200}, 4)
		drawPeakSet(img, waypointsForced)
		raEndMarkers(canvas, img, startLon, startLat, endLon, endLat)
		save("waypoints", raWithAttrib(raCropImage(img, cropRect), attribText))
	}

	// ── §1.3 Heat Map ─────────────────────────────────────────────────────────
	// Pure heatmap route (min_heat_score=0.001 — peaks never triggered).
	{
		img := raCopyRGBA(canvas.img)
		if heatLayer != nil {
			canvas.compositeHeatmap(img, heatLayer, 0.70)
		}
		canvas.drawPolyline(img, scenicHeat, color.RGBA{30, 100, 220, 255}, 6)
		raEndMarkers(canvas, img, startLon, startLat, endLon, endLat)
		save("heatmap", raWithAttrib(raCropImage(img, cropRect), attribText))
	}

	// ── §1.4 Combined Approach (natural default behavior) ────────────────────
	// min_heat_score=0.4 (default): Yerevan scores ~0.32 → peaks used;
	// London scores ~0.93 → no peaks needed.
	{
		img := raCopyRGBA(canvas.img)
		if heatLayer != nil {
			canvas.compositeHeatmap(img, heatLayer, 0.65)
		}
		canvas.drawPolyline(img, scenicNatural, color.RGBA{30, 100, 220, 255}, 6)
		drawPeakSet(img, waypointsNatural)
		raEndMarkers(canvas, img, startLon, startLat, endLon, endLat)
		save("scenic", raWithAttrib(raCropImage(img, cropRect), attribText))
	}
}

// ── MCP client ────────────────────────────────────────────────────────────────

// raCallPlan calls plan_scenic_route via MCP Streamable HTTP and returns the route UUID.
func raCallPlan(ctx context.Context, mcpURL string, args map[string]any) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": "plan_scenic_route", "arguments": args},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", mcpURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := (&http.Client{Timeout: 4 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var textContent string
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var msg map[string]any
			if json.Unmarshal([]byte(line[len("data: "):]), &msg) != nil {
				continue
			}
			if textContent = raExtractText(msg["result"]); textContent != "" {
				break
			}
		}
	} else {
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			return "", fmt.Errorf("json.Unmarshal: %w", err)
		}
		textContent = raExtractText(msg["result"])
	}

	idx := strings.Index(textContent, "/preview/")
	if idx < 0 {
		snip := textContent
		if len(snip) > 300 {
			snip = snip[:300]
		}
		return "", fmt.Errorf("no /preview/ URL in MCP response: %q", snip)
	}
	rest := textContent[idx+len("/preview/"):]
	if end := strings.IndexAny(rest, " \n\t)>\r"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest), nil
}

func raExtractText(result any) string {
	m, ok := result.(map[string]any)
	if !ok {
		return ""
	}
	content, _ := m["content"].([]any)
	for _, c := range content {
		item, ok := c.(map[string]any)
		if !ok || item["type"] != "text" {
			continue
		}
		if s, ok := item["text"].(string); ok {
			return s
		}
	}
	return ""
}

func raFetchDebugFC(ctx context.Context, url string) (map[string]any, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var fc map[string]any
	return fc, json.NewDecoder(resp.Body).Decode(&fc)
}

// raRouteCoords returns [lon, lat] pairs for the feature with the given label.
func raRouteCoords(fc map[string]any, label string) [][2]float64 {
	features, _ := fc["features"].([]any)
	for _, f := range features {
		feat, _ := f.(map[string]any)
		props, _ := feat["properties"].(map[string]any)
		if props["label"] != label {
			continue
		}
		geom, _ := feat["geometry"].(map[string]any)
		raw, _ := geom["coordinates"].([]any)
		coords := make([][2]float64, 0, len(raw))
		for _, c := range raw {
			pair, _ := c.([]any)
			if len(pair) < 2 {
				continue
			}
			lon, _ := pair[0].(float64)
			lat, _ := pair[1].(float64)
			coords = append(coords, [2]float64{lon, lat})
		}
		return coords
	}
	return nil
}

// raAllPeaks returns all heatmap peaks as [lon, lat, usedFlag] triples.
// usedFlag is 1.0 if the peak was a forced waypoint, 0.0 otherwise.
// Also returns the count of used peaks.
func raAllPeaks(fc map[string]any) ([][3]float64, int) {
	features, _ := fc["features"].([]any)
	var peaks [][3]float64
	usedCount := 0
	for _, f := range features {
		feat, _ := f.(map[string]any)
		props, _ := feat["properties"].(map[string]any)
		if props["_type"] != "heatmap_peak" {
			continue
		}
		geom, _ := feat["geometry"].(map[string]any)
		coords, _ := geom["coordinates"].([]any)
		if len(coords) < 2 {
			continue
		}
		lon, _ := coords[0].(float64)
		lat, _ := coords[1].(float64)
		used, _ := props["_used"].(bool)
		flag := 0.0
		if used {
			flag = 1.0
			usedCount++
		}
		peaks = append(peaks, [3]float64{lon, lat, flag})
	}
	return peaks, usedCount
}

// heatmapLayer holds a decoded heatmap overlay from the debug FeatureCollection.
type heatmapLayer struct {
	img    image.Image
	minLon float64
	minLat float64
	maxLon float64
	maxLat float64
}

// raHeatmapLayer parses the _type="image_overlay" feature from the debug FC.
func raHeatmapLayer(fc map[string]any) *heatmapLayer {
	features, _ := fc["features"].([]any)
	for _, f := range features {
		feat, _ := f.(map[string]any)
		props, _ := feat["properties"].(map[string]any)
		if props["_type"] != "image_overlay" {
			continue
		}
		b64data, _ := props["_data"].(string)
		if b64data == "" {
			continue
		}
		pngBytes, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			continue
		}
		// _bounds: [[minLat, minLon], [maxLat, maxLon]] (Leaflet convention)
		bounds, _ := props["_bounds"].([]any)
		if len(bounds) < 2 {
			continue
		}
		sw, _ := bounds[0].([]any)
		ne, _ := bounds[1].([]any)
		if len(sw) < 2 || len(ne) < 2 {
			continue
		}
		minLat, _ := sw[0].(float64)
		minLon, _ := sw[1].(float64)
		maxLat, _ := ne[0].(float64)
		maxLon, _ := ne[1].(float64)
		return &heatmapLayer{img: img, minLon: minLon, minLat: minLat, maxLon: maxLon, maxLat: maxLat}
	}
	return nil
}

// raBBox returns the [lon, lat] bounding box of a coordinate slice.
func raBBox(coords [][2]float64) (minLon, minLat, maxLon, maxLat float64) {
	minLon, minLat = coords[0][0], coords[0][1]
	maxLon, maxLat = coords[0][0], coords[0][1]
	for _, c := range coords[1:] {
		if c[0] < minLon {
			minLon = c[0]
		}
		if c[0] > maxLon {
			maxLon = c[0]
		}
		if c[1] < minLat {
			minLat = c[1]
		}
		if c[1] > maxLat {
			maxLat = c[1]
		}
	}
	return
}

// raPixelBBox returns the pixel bounding box of one or more route coordinate sets.
func raPixelBBox(c *raCanvas, routes ...[][2]float64) image.Rectangle {
	var x0, y0, x1, y1 int
	first := true
	for _, route := range routes {
		for _, coord := range route {
			px, py := c.project(coord[0], coord[1])
			if first {
				x0, y0, x1, y1 = px, py, px, py
				first = false
			} else {
				if px < x0 {
					x0 = px
				}
				if py < y0 {
					y0 = py
				}
				if px > x1 {
					x1 = px
				}
				if py > y1 {
					y1 = py
				}
			}
		}
	}
	return image.Rect(x0, y0, x1+1, y1+1)
}

// ── Tile canvas ───────────────────────────────────────────────────────────────

type raCanvas struct {
	img      *image.RGBA
	zoom     int
	minTileX int
	minTileY int
}

// raBuildCanvas downloads and stitches map tiles into a canvas.
func raBuildCanvas(minLon, minLat, maxLon, maxLat float64, zoom, padTiles int, urlTmpl string) (*raCanvas, error) {
	n := math.Pow(2, float64(zoom))
	txMin := int(raLonToTX(minLon, n)) - padTiles
	txMax := int(raLonToTX(maxLon, n)) + padTiles
	tyMin := int(raLatToTY(maxLat, n)) - padTiles
	tyMax := int(raLatToTY(minLat, n)) + padTiles

	nx := txMax - txMin + 1
	ny := tyMax - tyMin + 1

	canvas := image.NewRGBA(image.Rect(0, 0, nx*raTilePixels, ny*raTilePixels))

	for ty := tyMin; ty <= tyMax; ty++ {
		for tx := txMin; tx <= txMax; tx++ {
			tile, err := raFetchTile(zoom, tx, ty, urlTmpl)
			if err != nil {
				return nil, fmt.Errorf("tile %d/%d/%d: %w", zoom, tx, ty, err)
			}
			dstX := (tx - txMin) * raTilePixels
			dstY := (ty - tyMin) * raTilePixels
			draw.Draw(canvas, image.Rect(dstX, dstY, dstX+raTilePixels, dstY+raTilePixels), tile, image.Point{}, draw.Src)
			time.Sleep(100 * time.Millisecond)
		}
	}
	return &raCanvas{img: canvas, zoom: zoom, minTileX: txMin, minTileY: tyMin}, nil
}

func (c *raCanvas) project(lon, lat float64) (int, int) {
	n := math.Pow(2, float64(c.zoom))
	px := raLonToTX(lon, n)*raTilePixels - float64(c.minTileX)*raTilePixels
	latR := lat * math.Pi / 180
	py := (1-math.Log(math.Tan(latR)+1/math.Cos(latR))/math.Pi)/2*n*raTilePixels - float64(c.minTileY)*raTilePixels
	b := c.img.Bounds()
	return clampInt(int(px), 0, b.Max.X-1), clampInt(int(py), 0, b.Max.Y-1)
}

func (c *raCanvas) pixelToGeo(px, py int) (lon, lat float64) {
	n := math.Pow(2, float64(c.zoom))
	absPx := float64(px) + float64(c.minTileX)*raTilePixels
	absPy := float64(py) + float64(c.minTileY)*raTilePixels
	lon = absPx/(n*raTilePixels)*360 - 180
	y := math.Pi * (1 - 2*absPy/(n*raTilePixels))
	lat = math.Atan(math.Sinh(y)) * 180 / math.Pi
	return
}

func (c *raCanvas) drawPolyline(dst *image.RGBA, coords [][2]float64, col color.RGBA, thick int) {
	b := dst.Bounds()
	h := thick / 2

	bres := func(x0, y0, x1, y1 int) {
		dx := absInt(x1 - x0)
		dy := absInt(y1 - y0)
		sx, sy := 1, 1
		if x0 > x1 {
			sx = -1
		}
		if y0 > y1 {
			sy = -1
		}
		err := dx - dy
		for {
			for ty := -h; ty <= h; ty++ {
				for tx := -h; tx <= h; tx++ {
					px, py := x0+tx, y0+ty
					if px >= b.Min.X && px < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
						if col.A == 255 {
							dst.SetRGBA(px, py, col)
						} else {
							raBlend(dst, px, py, col)
						}
					}
				}
			}
			if x0 == x1 && y0 == y1 {
				break
			}
			if e2 := 2 * err; e2 > -dy {
				err -= dy
				x0 += sx
			} else if e2 < dx {
				err += dx
				y0 += sy
			}
		}
	}

	for i := 1; i < len(coords); i++ {
		x0, y0 := c.project(coords[i-1][0], coords[i-1][1])
		x1, y1 := c.project(coords[i][0], coords[i][1])
		bres(x0, y0, x1, y1)
	}
}

func (c *raCanvas) compositeHeatmap(dst *image.RGBA, layer *heatmapLayer, opacity float64) {
	hw := float64(layer.img.Bounds().Dx())
	hh := float64(layer.img.Bounds().Dy())
	lonRange := layer.maxLon - layer.minLon
	latRange := layer.maxLat - layer.minLat
	if lonRange <= 0 || latRange <= 0 || hw <= 0 || hh <= 0 {
		return
	}

	x0, y0 := c.project(layer.minLon, layer.maxLat)
	x1, y1 := c.project(layer.maxLon, layer.minLat)
	b := dst.Bounds()
	x0 = clampInt(x0, 0, b.Max.X)
	y0 = clampInt(y0, 0, b.Max.Y)
	x1 = clampInt(x1, 0, b.Max.X)
	y1 = clampInt(y1, 0, b.Max.Y)

	nrgbaModel := color.NRGBAModel
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			lon, lat := c.pixelToGeo(px, py)
			hx := int((lon - layer.minLon) / lonRange * hw)
			hy := int((layer.maxLat - lat) / latRange * hh)
			if hx < 0 || hx >= int(hw) || hy < 0 || hy >= int(hh) {
				continue
			}
			hc := nrgbaModel.Convert(layer.img.At(hx, hy)).(color.NRGBA)
			if hc.A == 0 {
				continue
			}
			effA := float64(hc.A) / 255.0 * opacity
			ex := dst.RGBAAt(px, py)
			dst.SetRGBA(px, py, color.RGBA{
				R: uint8(float64(hc.R)*effA + float64(ex.R)*(1-effA)),
				G: uint8(float64(hc.G)*effA + float64(ex.G)*(1-effA)),
				B: uint8(float64(hc.B)*effA + float64(ex.B)*(1-effA)),
				A: 255,
			})
		}
	}
}

func raEndMarkers(c *raCanvas, dst *image.RGBA, startLon, startLat, endLon, endLat float64) {
	sx, sy := c.project(startLon, startLat)
	ex, ey := c.project(endLon, endLat)
	raFillCircle(dst, sx, sy, 9, color.RGBA{255, 255, 255, 255})
	raFillCircle(dst, sx, sy, 7, color.RGBA{34, 160, 34, 255})
	raFillCircle(dst, ex, ey, 9, color.RGBA{255, 255, 255, 255})
	raFillCircle(dst, ex, ey, 7, color.RGBA{200, 40, 40, 255})
}

func raFetchTile(z, x, y int, urlTmpl string) (image.Image, error) {
	if err := os.MkdirAll(raTileCache, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(raTileCache, fmt.Sprintf("%d_%d_%d.png", z, x, y))
	if data, err := os.ReadFile(path); err == nil {
		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	}
	url := raTileURL(urlTmpl, z, x, y)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", raUserAgent)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(path, data, 0o644)
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

func raTileURL(tmpl string, z, x, y int) string {
	subdomains := []string{"a", "b", "c", "d"}
	u := strings.ReplaceAll(tmpl, "{s}", subdomains[(x+y)%len(subdomains)])
	u = strings.ReplaceAll(u, "{z}", fmt.Sprintf("%d", z))
	u = strings.ReplaceAll(u, "{x}", fmt.Sprintf("%d", x))
	u = strings.ReplaceAll(u, "{y}", fmt.Sprintf("%d", y))
	u = strings.ReplaceAll(u, "{r}", "")
	return u
}

func raLonToTX(lon, n float64) float64 {
	return (lon + 180) / 360 * n
}

func raLatToTY(lat, n float64) float64 {
	latR := lat * math.Pi / 180
	return (1 - math.Log(math.Tan(latR)+1/math.Cos(latR))/math.Pi) / 2 * n
}

// ── Drawing helpers ───────────────────────────────────────────────────────────

func raFillCircle(img *image.RGBA, cx, cy, r int, col color.RGBA) {
	b := img.Bounds()
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px, py := cx+dx, cy+dy
				if px >= b.Min.X && px < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
					img.SetRGBA(px, py, col)
				}
			}
		}
	}
}

func raBlend(img *image.RGBA, x, y int, col color.RGBA) {
	a := float64(col.A) / 255.0
	ex := img.RGBAAt(x, y)
	img.SetRGBA(x, y, color.RGBA{
		R: uint8(float64(col.R)*a + float64(ex.R)*(1-a)),
		G: uint8(float64(col.G)*a + float64(ex.G)*(1-a)),
		B: uint8(float64(col.B)*a + float64(ex.B)*(1-a)),
		A: 255,
	})
}

func raCopyRGBA(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}

func raCropImage(src *image.RGBA, rect image.Rectangle) *image.RGBA {
	rect = rect.Intersect(src.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), src, rect.Min, draw.Src)
	return dst
}

func raWithAttrib(img *image.RGBA, text string) *image.RGBA {
	const barH = 18
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h+barH))
	draw.Draw(out, img.Bounds(), img, image.Point{}, draw.Src)
	for y := h; y < h+barH; y++ {
		for x := 0; x < w; x++ {
			out.SetRGBA(x, y, color.RGBA{30, 30, 30, 220})
		}
	}
	(&font.Drawer{
		Dst:  out,
		Src:  image.NewUniform(color.RGBA{210, 210, 210, 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(6, h+barH-4),
	}).DrawString(text)
	return out
}

func raStripHTML(s string) string {
	var buf strings.Builder
	inTag := false
	for _, ch := range s {
		switch {
		case ch == '<':
			inTag = true
		case ch == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(ch)
		}
	}
	out := buf.String()
	out = strings.ReplaceAll(out, "&copy;", "(c)")
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return strings.TrimSpace(out)
}
