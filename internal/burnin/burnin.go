// Package burnin burns a frame/timecode stamp into an image at a fixed origin.
package burnin

import (
	"fmt"

	"frameburn/internal/ppm"
	"frameburn/internal/raster"
	"frameburn/internal/timecode"
)

// Stamp is the single source of truth used for the frame and timecode overlay.
type Stamp struct {
	Frame    int64  `json:"frame"`
	FrameTag string `json:"frame_tag"`
	Timecode string `json:"timecode"`
	Rate     string `json:"rate"`
	Label    string `json:"label"`
}

func NewStamp(rate timecode.Rate, frame int64) (Stamp, error) {
	tc, err := rate.At(frame)
	if err != nil {
		return Stamp{}, err
	}
	frameTag := fmt.Sprintf("%06d", frame)
	return Stamp{
		Frame:    frame,
		FrameTag: frameTag,
		Timecode: tc.String(),
		Rate:     rate.String(),
		Label:    fmt.Sprintf("F%s TC %s %s", frameTag, tc, rate),
	}, nil
}

type Style struct {
	X           int
	Y           int
	Scale       int
	Padding     int
	Border      int
	TextColor   ppm.RGB
	BoxColor    ppm.RGB
	BorderColor ppm.RGB
}

func DefaultStyle(im *ppm.Image) Style {
	scale := 1
	if im.Width >= 480 && im.Height >= 270 {
		scale = 2
	}
	if im.Width >= 960 && im.Height >= 540 {
		scale = 3
	}
	return Style{
		X:           4 + 6*scale + 1,
		Y:           4 + 6*scale + 1,
		Scale:       scale,
		Padding:     6 * scale,
		Border:      1,
		TextColor:   ppm.White,
		BoxColor:    ppm.Black,
		BorderColor: ppm.RGB{R: 235, G: 190, B: 30},
	}
}

func (s Style) normalized() Style {
	if s.Scale < 1 {
		s.Scale = 1
	}
	if s.Padding < 0 {
		s.Padding = 0
	}
	if s.Border < 0 {
		s.Border = 0
	}
	return s
}

// PanelRect is the fixed outer rectangle occupied by a label.
func (s Style) PanelRect(text string) ppm.Rect {
	s = s.normalized()
	width, height := raster.TextSize(text, s.Scale)
	return ppm.Rect{
		X:      s.X - s.Padding - s.Border,
		Y:      s.Y - s.Padding - s.Border,
		Width:  width + 2*(s.Padding+s.Border),
		Height: height + 2*(s.Padding+s.Border),
	}
}

// ContentRect is the area outside the stamp panel. Content checks use this
// rectangle so changing label text cannot be mistaken for a picture edit.
func (s Style) ContentRect(im *ppm.Image, text string) ppm.Rect {
	s = s.normalized()
	panel := s.PanelRect(text)
	y := panel.Y + panel.Height + 2*s.Scale
	if y >= im.Height {
		return ppm.Rect{}
	}
	return ppm.Rect{X: 0, Y: y, Width: im.Width, Height: im.Height - y}
}

// Burn draws the panel and text and returns the occupied rectangle.
func Burn(im *ppm.Image, stamp Stamp, style Style) ppm.Rect {
	style = style.normalized()
	panel := style.PanelRect(stamp.Label)
	im.FillRect(panel, style.BorderColor)
	inner := panel
	inner.X += style.Border
	inner.Y += style.Border
	inner.Width -= 2 * style.Border
	inner.Height -= 2 * style.Border
	im.FillRect(inner, style.BoxColor)
	raster.DrawText(im, style.X, style.Y, stamp.Label, style.TextColor, style.Scale)
	return panel
}
