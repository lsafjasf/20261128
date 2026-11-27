package analyze

import (
	"math"
	"testing"

	"interlace-diag/internal/video"
)

type synthMode int

const (
	synthWeave synthMode = iota
	synthMotionBlur
	synthOverSmooth
	synthClean
	synthProgressiveBlur
)

func makeSynthetic(mode synthMode, bff bool, n int) []video.Frame {
	const w, h = 192, 128
	frames := make([]video.Frame, n)
	for i := 0; i < n; i++ {
		y := make([]byte, w*h)
		topTime := float64(2 * i)
		bottomTime := topTime + 1
		if bff {
			bottomTime = float64(2 * i)
			topTime = bottomTime + 1
		}
		blur := 1
		if mode == synthMotionBlur || mode == synthProgressiveBlur {
			blur = 7
		}
		for row := 0; row < h; row++ {
			pos := topTime * 2
			if row%2 == 1 {
				pos = bottomTime * 2
			}
			if mode == synthClean {
				pos = float64(2 * i)
			}
			if mode == synthProgressiveBlur {
				pos = float64(4 * i)
			}
			for x := 0; x < w; x++ {
				v := barPixel(x, int(pos)+40, 38, blur)
				if mode == synthOverSmooth {
					v = barPixel(x, 40+4*i+int(2.5*float64(row)), 38, 7)
				}
				y[row*w+x] = byte(v)
			}
		}
		if mode == synthOverSmooth {
			duplicateRows(y, w, h)
		}
		frames[i] = video.Frame{Index: i, TimeSeconds: float64(i) / 25, Width: w, Height: h, Y: y}
	}
	return frames
}

func barPixel(x int, left int, width int, blur int) int {
	if blur < 1 {
		blur = 1
	}
	var sum int
	for s := 0; s < blur; s++ {
		sx := x - blur/2 + s
		if sx >= left && sx < left+width {
			sum += 220
		} else {
			sum += 24
		}
	}
	return sum / blur
}

func duplicateRows(y []byte, w, h int) {
	for row := 2; row+1 < h; row += 2 {
		copy(y[(row+1)*w:(row+2)*w], y[row*w:(row+1)*w])
	}
}

func analyzeSynthetic(t *testing.T, frames []video.Frame) []FrameRecord {
	t.Helper()
	a := NewAnalyzer(Options{})
	var got []FrameRecord
	for _, f := range frames {
		if rec, err := a.Push(f); err != nil {
			t.Fatal(err)
		} else if rec != nil {
			got = append(got, *rec)
		}
	}
	if rec := a.Flush(); rec != nil {
		got = append(got, *rec)
	}
	return got
}

func TestSyntheticDiagnostics(t *testing.T) {
	cases := []struct {
		name    string
		mode    synthMode
		bff     bool
		verdict string
		order   string
	}{
		{"weave-tff", synthWeave, false, VerdictWeaveError, "tff"},
		{"weave-bff", synthWeave, true, VerdictWeaveError, "bff"},
		{"blur-tff", synthMotionBlur, false, VerdictMotionBlur, "tff"},
		{"blur-bff", synthMotionBlur, true, VerdictMotionBlur, "bff"},
		{"progressive-blur", synthProgressiveBlur, false, VerdictMotionBlur, ""},
		{"smooth", synthOverSmooth, false, VerdictOverSmooth, ""},
		{"clean", synthClean, false, VerdictClean, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records := analyzeSynthetic(t, makeSynthetic(tc.mode, tc.bff, 5))
			for _, rec := range records {
				t.Logf("frame=%d verdict=%s scores=%+v order=%s comb=%+v blur=%+v smooth=%+v motion=%+v",
					rec.Frame, rec.Verdict, rec.Scores, rec.FieldOrder.Order,
					rec.Evidence.Combing, rec.Evidence.Blur, rec.Evidence.Smoothing, rec.Evidence.Motion)
				if math.IsNaN(rec.Confidence) {
					t.Fatal("NaN confidence")
				}
			}
			if records[len(records)-1].Verdict != tc.verdict {
				t.Fatalf("verdict=%s, want %s", records[len(records)-1].Verdict, tc.verdict)
			}
			if tc.order != "" && records[len(records)-1].FieldOrder.Order != tc.order {
				t.Fatalf("order=%s, want %s", records[len(records)-1].FieldOrder.Order, tc.order)
			}
		})
	}
}

func TestEstimateShiftSignAndAxes(t *testing.T) {
	const w, h = 96, 96
	a := plane{W: w, H: h, Pix: make([]byte, w*h)}
	cols := make([]int, w)
	rows := make([]int, h)
	for x := range cols {
		cols[x] = (x*37 + (x/5)*113) % 190
	}
	for y := range rows {
		rows[y] = (y*53 + (y/7)*89) % 190
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.Pix[y*w+x] = byte(30 + (cols[x]+rows[y])/2)
		}
	}
	const dx, dy = 3, 4
	b := plane{W: w, H: h, Pix: make([]byte, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x >= dx && y >= dy {
				b.Pix[y*w+x] = a.Pix[(y-dy)*w+x-dx]
			}
		}
	}
	shift := estimateShift(a, b, 8, 8)
	if shift.X != dx || shift.Y != 2*dy {
		t.Fatalf("shift=(%.1f,%.1f), want (%d,%d)", shift.X, shift.Y, dx, 2*dy)
	}
	if shift.Speed < 5 || shift.Confidence < 0.5 {
		t.Fatalf("unexpected speed/confidence: %+v", shift)
	}
}
