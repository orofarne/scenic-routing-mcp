package format

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"

	"github.com/orofarne/scenic-routing-mcp/internal/heatmap"
)

const renderGamma = 0.4 // compresses dynamic range: dims dense clusters, brightens sparse areas

// HeatmapPNG renders the grid as a blue→cyan→yellow→red PNG with alpha.
// Zero-heat cells are fully transparent (suitable for map overlay).
//
// Normalization: 95th-percentile ceiling so outlier hotspots don't suppress
// everything else. Gamma correction (^0.4) further spreads the dynamic range.
func HeatmapPNG(g *heatmap.Grid) ([]byte, error) {
	vals := make([]float64, 0, g.Width*g.Height)
	for _, row := range g.Values {
		for _, v := range row {
			if !math.IsInf(v, -1) {
				vals = append(vals, v)
			}
		}
	}

	ceiling := 0.0
	if len(vals) > 0 {
		sort.Float64s(vals)
		ceiling = vals[int(float64(len(vals))*0.95)]
		if ceiling <= 0 {
			ceiling = vals[len(vals)-1]
		}
	}

	img := image.NewNRGBA(image.Rect(0, 0, g.Width, g.Height))
	for y := 0; y < g.Height; y++ {
		for x := 0; x < g.Width; x++ {
			t := 0.0
			v := g.Values[y][x]
			if ceiling > 0 && !math.IsInf(v, -1) {
				t = math.Max(0, math.Min(v/ceiling, 1.0))
				t = math.Pow(t, renderGamma)
			}
			img.SetNRGBA(x, y, heatColor(t))
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// heatColor maps t∈[0,1] → blue→cyan→yellow→red NRGBA with alpha∈[0,220].
func heatColor(t float64) color.NRGBA {
	if t <= 0 {
		return color.NRGBA{}
	}
	if t > 1 {
		t = 1
	}
	var r, g, b uint8
	switch {
	case t < 1.0/3:
		s := t * 3
		r, g, b = 0, uint8(s*255), 255
	case t < 2.0/3:
		s := (t - 1.0/3) * 3
		r, g, b = uint8(s*255), 255, uint8((1-s)*255)
	default:
		s := (t - 2.0/3) * 3
		r, g, b = 255, uint8((1-s)*255), 0
	}
	// Alpha: ramps from 0 at t=0 to 220 at t=1 (keeps brightest areas non-opaque).
	a := uint8(t * 220)
	return color.NRGBA{r, g, b, a}
}
