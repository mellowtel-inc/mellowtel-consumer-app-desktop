package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// makeIcon renders a simple filled circle of the given colour as PNG bytes.
// Used for the connected (green) and paused (grey) tray states.
func makeIcon(c color.RGBA) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	radius := center - 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - center
			dy := float64(y) + 0.5 - center
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			} else {
				img.Set(x, y, color.RGBA{0, 0, 0, 0})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

var (
	iconConnected = makeIcon(color.RGBA{R: 0x4C, G: 0xAF, B: 0x50, A: 0xFF}) // green
	iconPaused    = makeIcon(color.RGBA{R: 0x9E, G: 0x9E, B: 0x9E, A: 0xFF}) // grey
)
