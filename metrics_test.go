package ghostdiag

import "testing"

func TestDirectionalBlurWidthSeparatesStepAndRamp(t *testing.T) {
	step := makePlane(64, 8, 30)
	ramp := makePlane(64, 8, 30)
	for y := 0; y < 8; y++ {
		for x := 32; x < 64; x++ {
			step.Pix[y*64+x] = 200
		}
		for x := 32; x < 36; x++ {
			ramp.Pix[y*64+x] = uint8(30 + (x-32+1)*34)
		}
		for x := 36; x < 64; x++ {
			ramp.Pix[y*64+x] = 200
		}
	}
	box := Box{X: 20, Y: 1, Width: 24, Height: 6}
	stepWidth := directionalBlurWidth(step, box, 4, 0)
	rampWidth := directionalBlurWidth(ramp, box, 4, 0)
	if stepWidth >= 2 {
		t.Fatalf("step width=%f, want <2", stepWidth)
	}
	if rampWidth < 2.5 {
		t.Fatalf("ramp width=%f, want >=2.5", rampWidth)
	}
}

func TestBlockinessOnCodingGrid(t *testing.T) {
	blocked := makePlane(64, 32, 0)
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			value := uint8(20)
			if ((x/8)+(y/8))%2 == 0 {
				value = 4
			}
			blocked.Pix[y*64+x] = value
		}
	}
	if got := blockiness(blocked, Box{X: 8, Y: 8, Width: 32, Height: 16}, 8); got < 0.8 {
		t.Fatalf("grid blockiness=%f, want >=0.8", got)
	}
	flat := makePlane(64, 32, 10)
	if got := blockiness(flat, Box{X: 8, Y: 8, Width: 32, Height: 16}, 8); got != 0 {
		t.Fatalf("flat blockiness=%f, want 0", got)
	}
}

func TestClassifyEvidence(t *testing.T) {
	defaults := DefaultOptions()
	tests := []struct {
		name string
		in   Evidence
		want string
	}{
		{"ghost", Evidence{TemporalScore: 0.8, BlockScore: 0.1, BlurWidth: 1, MotionOverlap: 1}, "temporal_ghost"},
		{"block", Evidence{TemporalScore: 0.2, BlockScore: 0.7, BlurWidth: 1, MotionOverlap: 1}, "coding_block"},
		{"blur", Evidence{TemporalScore: 0.1, BlockScore: 0.1, BlurWidth: 3.2, MotionOverlap: 1}, "motion_blur"},
		{"weak", Evidence{TemporalScore: 0.1, BlockScore: 0.1, BlurWidth: 1, MotionOverlap: 1}, ""},
	}
	for _, tt := range tests {
		got, _, _, _ := classifyEvidence(tt.in, defaults)
		if got != tt.want {
			t.Errorf("%s: classify=%q, want %q", tt.name, got, tt.want)
		}
	}
}

func makePlane(width, height int, value uint8) Plane {
	p := Plane{Width: width, Height: height, Pix: make([]uint8, width*height)}
	for i := range p.Pix {
		p.Pix[i] = value
	}
	return p
}
