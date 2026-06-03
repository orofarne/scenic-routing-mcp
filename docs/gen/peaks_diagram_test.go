package docgen

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestPeaksDiagram generates docs/img/peaks_diagram.png — a schematic
// illustrating how Peaks() finds representative waypoints.
//
//	go test ./docs/gen/ -run TestPeaksDiagram -v
func TestPeaksDiagram(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode: generates docs/img/peaks_diagram.png")
	}

	const W, H = 520, 200

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}

	// Principal axis: from lower-left to upper-right.
	const ax0, ay0 = 55.0, 158.0
	const ax1, ay1 = 465.0, 52.0

	adx := ax1 - ax0
	ady := ay1 - ay0
	axLen := math.Sqrt(adx*adx + ady*ady)
	ux, uy := adx/axLen, ady/axLen
	nx, ny := -uy, ux

	// ── 1. Heatmap band ──────────────────────────────────────────────────────
	const sigPx = 36.0
	const cutPx = 3 * sigPx

	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			tProj := (fx-ax0)*ux + (fy-ay0)*uy
			if tProj < -sigPx || tProj > axLen+sigPx {
				continue
			}
			nearX := ax0 + tProj*ux
			nearY := ay0 + tProj*uy
			dist := math.Sqrt((fx-nearX)*(fx-nearX) + (fy-nearY)*(fy-nearY))
			if dist >= cutPx {
				continue
			}
			tf := 1.0 - dist/cutPx
			heat := tf * tf
			img.SetRGBA(x, y, cmpHeatColor(math.Pow(heat, 0.4)))
		}
	}

	// ── Drawing helpers ───────────────────────────────────────────────────────

	set := func(x, y int, c color.RGBA) {
		if x >= 0 && x < W && y >= 0 && y < H {
			img.SetRGBA(x, y, c)
		}
	}

	bres := func(x0, y0, x1, y1 int, c color.RGBA, dashOn, dashOff int) {
		dx, dy := absInt(x1-x0), absInt(y1-y0)
		sx, sy := 1, 1
		if x0 > x1 {
			sx = -1
		}
		if y0 > y1 {
			sy = -1
		}
		err := dx - dy
		step := 0
		period := dashOn + dashOff
		for {
			if period == 0 || step%period < dashOn {
				set(x0, y0, c)
			}
			step++
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

	// ── 2. Bucket boundaries at 1/3 and 2/3 of the axis ─────────────────────
	divC := color.RGBA{30, 30, 30, 240}
	extend := cutPx + 18

	for _, frac := range []float64{1.0 / 3, 2.0 / 3} {
		tp := axLen * frac
		cx, cy := ax0+tp*ux, ay0+tp*uy
		x0 := int(cx + nx*extend)
		y0 := int(cy + ny*extend)
		x1 := int(cx - nx*extend)
		y1 := int(cy - ny*extend)
		bres(x0, y0, x1, y1, divC, 7, 4)
		bres(x0+1, y0, x1+1, y1, divC, 7, 4)
	}

	// ── 3. Peak circles at 1/6, 1/2, 5/6 of the axis ────────────────────────
	const r = 10
	peakFill := color.RGBA{255, 215, 30, 255}
	peakBorder := color.RGBA{25, 25, 25, 255}

	for _, frac := range []float64{1.0 / 6, 3.0 / 6, 5.0 / 6} {
		tPeak := axLen * frac
		cx, cy := int(math.Round(ax0+tPeak*ux)), int(math.Round(ay0+tPeak*uy))

		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx*dx+dy*dy <= r*r {
					set(cx+dx, cy+dy, peakFill)
				}
			}
		}
		for _, rr := range []float64{float64(r) - 0.5, float64(r) + 0.5} {
			for a := 0.0; a < 2*math.Pi; a += 0.02 {
				set(cx+int(math.Round(rr*math.Cos(a))),
					cy+int(math.Round(rr*math.Sin(a))),
					peakBorder)
			}
		}
	}

	// ── 4. Extent arrow above the band ───────────────────────────────────────
	const arrowGap = 8.0
	const arrowW = 6
	const arrowL = 12

	off := sigPx + arrowGap
	arrX0 := int(math.Round(ax0 - nx*off))
	arrY0 := int(math.Round(ay0 - ny*off))
	arrX1 := int(math.Round(ax1 - nx*off))
	arrY1 := int(math.Round(ay1 - ny*off))

	arrC := color.RGBA{40, 40, 40, 255}
	bres(arrX0, arrY0, arrX1, arrY1, arrC, 0, 0)
	bres(arrX0+1, arrY0, arrX1+1, arrY1, arrC, 0, 0)

	drawArrowhead := func(tx, ty, dirX, dirY int) {
		fdx, fdy := float64(dirX), float64(dirY)
		flen := math.Sqrt(fdx*fdx + fdy*fdy)
		fdx, fdy = fdx/flen, fdy/flen
		fpx, fpy := -fdy, fdx
		b1x := int(math.Round(float64(tx) - fdx*float64(arrowL) + fpx*float64(arrowW)))
		b1y := int(math.Round(float64(ty) - fdy*float64(arrowL) + fpy*float64(arrowW)))
		b2x := int(math.Round(float64(tx) - fdx*float64(arrowL) - fpx*float64(arrowW)))
		b2y := int(math.Round(float64(ty) - fdy*float64(arrowL) - fpy*float64(arrowW)))
		fillTriangle(img, W, H, tx, ty, b1x, b1y, b2x, b2y, arrC)
	}

	drawArrowhead(arrX0, arrY0, arrX0-arrX1, arrY0-arrY1)
	drawArrowhead(arrX1, arrY1, arrX1-arrX0, arrY1-arrY0)

	// ── Save ──────────────────────────────────────────────────────────────────
	outDir := filepath.Join("..", "img")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(filepath.Join(outDir, "peaks_diagram.png"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	t.Log("wrote docs/img/peaks_diagram.png")
}

func fillTriangle(img *image.RGBA, W, H, x0, y0, x1, y1, x2, y2 int, c color.RGBA) {
	if y0 > y1 {
		x0, x1, y0, y1 = x1, x0, y1, y0
	}
	if y0 > y2 {
		x0, x2, y0, y2 = x2, x0, y2, y0
	}
	if y1 > y2 {
		x1, x2, y1, y2 = x2, x1, y2, y1
	}
	lerp := func(ya, yb, xa, xb, y int) int {
		if yb == ya {
			return xa
		}
		return xa + (xb-xa)*(y-ya)/(yb-ya)
	}
	for y := y0; y <= y2; y++ {
		xa := lerp(y0, y2, x0, x2, y)
		var xb int
		if y < y1 {
			xb = lerp(y0, y1, x0, x1, y)
		} else {
			xb = lerp(y1, y2, x1, x2, y)
		}
		if xa > xb {
			xa, xb = xb, xa
		}
		for x := xa; x <= xb; x++ {
			if x >= 0 && x < W && y >= 0 && y < H {
				img.SetRGBA(x, y, c)
			}
		}
	}
}
