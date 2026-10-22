package remote

import (
	"fmt"

	"rsc.io/qr"
)

// quietZone is the white border a QR code needs around it, in modules.
const quietZone = 4

// QRModules encodes text as a QR code (error correction M) and returns its
// modules, true = dark, with the quiet zone included.
func QRModules(text string) ([][]bool, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, fmt.Errorf("qr code for %q: %w", text, err)
	}
	n := c.Size + 2*quietZone
	m := make([][]bool, n)
	for y := range m {
		m[y] = make([]bool, n)
		for x := range m[y] {
			qx, qy := x-quietZone, y-quietZone
			m[y][x] = qx >= 0 && qy >= 0 && qx < c.Size && qy < c.Size && c.Black(qx, qy)
		}
	}
	return m, nil
}

// QRPixels renders a QR code as opaque RGBA8 pixels, scale pixels per module
// (black on white): side is the image's width and height.
func QRPixels(text string, scale int) (side int, rgba []byte, err error) {
	m, err := QRModules(text)
	if err != nil {
		return 0, nil, err
	}
	scale = max(scale, 1)
	side = len(m) * scale
	rgba = make([]byte, side*side*4)
	for y := 0; y < side; y++ {
		row := m[y/scale]
		for x := 0; x < side; x++ {
			v := byte(255)
			if row[x/scale] {
				v = 0
			}
			i := (y*side + x) * 4
			rgba[i], rgba[i+1], rgba[i+2], rgba[i+3] = v, v, v, 255
		}
	}
	return side, rgba, nil
}
