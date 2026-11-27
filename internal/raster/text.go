package raster

import (
	"strings"

	"frameburn/internal/ppm"
)

const (
	GlyphWidth   = 5
	GlyphHeight  = 7
	GlyphAdvance = 6
)

// TextSize returns the drawn width and height before clipping.
func TextSize(text string, scale int) (int, int) {
	if scale < 1 {
		scale = 1
	}
	runes := []rune(strings.ToUpper(text))
	if len(runes) == 0 {
		return 0, GlyphHeight * scale
	}
	return (len(runes)*GlyphAdvance - 1) * scale, GlyphHeight * scale
}

// DrawText draws uppercase text with the top-left corner at (x, y).
func DrawText(im *ppm.Image, x, y int, text string, color ppm.RGB, scale int) {
	if scale < 1 {
		scale = 1
	}
	cursor := x
	for _, ch := range strings.ToUpper(text) {
		glyph, ok := glyphs[ch]
		if !ok {
			glyph = glyphs['?']
		}
		for row, bits := range glyph {
			for col := 0; col < GlyphWidth; col++ {
				if bits&(1<<uint(GlyphWidth-1-col)) == 0 {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						im.Set(cursor+col*scale+dx, y+row*scale+dy, color)
					}
				}
			}
		}
		cursor += GlyphAdvance * scale
	}
}
