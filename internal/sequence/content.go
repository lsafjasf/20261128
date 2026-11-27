package sequence

import (
	"fmt"

	"frameburn/internal/ppm"
	"frameburn/internal/timeline"
)

var clipPalette = []ppm.RGB{
	{R: 30, G: 66, B: 112},
	{R: 158, G: 72, B: 30},
	{R: 26, G: 108, B: 72},
	{R: 118, G: 38, B: 104},
	{R: 98, G: 92, B: 30},
}

func renderPicture(plan *timeline.Plan, frame timeline.Frame) (*ppm.Image, error) {
	im, err := ppm.New(plan.Config.Width, plan.Config.Height, ppm.Black)
	if err != nil {
		return nil, err
	}
	if frame.Black {
		return im, nil
	}
	base := clipPalette[frame.Clip%len(clipPalette)]
	im.Fill(base)

	barColor := lighten(base, 105)
	barWidth := max(4, plan.Config.Width/24)
	x := int((frame.ContentFrame * 3) % int64(plan.Config.Width))
	im.FillRect(ppm.Rect{
		X: x - barWidth/2, Y: 0, Width: barWidth, Height: plan.Config.Height,
	}, barColor)

	if x+barWidth/2 >= plan.Config.Width {
		im.FillRect(ppm.Rect{
			X: 0, Y: 0, Width: x + barWidth/2 - plan.Config.Width + 1,
			Height: plan.Config.Height,
		}, barColor)
	}
	if x-barWidth/2 < 0 {
		im.FillRect(ppm.Rect{
			X: plan.Config.Width + x - barWidth/2, Y: 0,
			Width: barWidth, Height: plan.Config.Height,
		}, barColor)
	}
	return im, nil
}

func lighten(c ppm.RGB, amount int) ppm.RGB {
	return ppm.RGB{
		R: clamp(int(c.R) + amount),
		G: clamp(int(c.G) + amount),
		B: clamp(int(c.B) + amount),
	}
}

func clamp(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func validatePanel(im *ppm.Image, panel ppm.Rect) error {
	if panel.X < 0 || panel.Y < 0 ||
		panel.X+panel.Width > im.Width || panel.Y+panel.Height > im.Height {
		return fmt.Errorf("stamp panel %+v does not fit in %dx%d frame", panel, im.Width, im.Height)
	}
	return nil
}
