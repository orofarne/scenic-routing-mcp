package docgen

import "image/color"

const metersPerDegLat = 111320.0

// cmpHeatColor maps t∈[0,1] to a blue→cyan→yellow→red heat colour
// composited over a white background at opacity t×0.86.
func cmpHeatColor(t float64) color.RGBA {
	if t <= 0 {
		return color.RGBA{255, 255, 255, 255}
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
	a := t * 0.86
	bl := func(c uint8) uint8 { return uint8(float64(c)*a + 255*(1-a)) }
	return color.RGBA{bl(r), bl(g), bl(b), 255}
}
