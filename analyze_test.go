package ghostdiag

import (
	"testing"

	"ghostdiag/internal/synth"
)

type sliceSource struct {
	frames []Frame
	format Format
	index  int
}

func (s *sliceSource) Next() (Frame, error) {
	if s.index >= len(s.frames) {
		return Frame{}, ErrEndOfStream
	}
	frame := s.frames[s.index]
	s.index++
	return frame, nil
}

func (s *sliceSource) Format() Format { return s.format }

func sourcesFor(seq synth.Sequence) (*sliceSource, *sliceSource) {
	format := Format{
		Width: seq.Width, Height: seq.Height, FrameRate: seq.FrameRate,
		PixelFormat: "gray", Origin: "synthetic",
	}
	original := &sliceSource{format: format}
	denoised := &sliceSource{format: format}
	for i := range seq.Original {
		original.frames = append(original.frames, Frame{
			Index: i, Timestamp: float64(i) / seq.FrameRate,
			Plane: Plane{Width: seq.Width, Height: seq.Height, Pix: seq.Original[i]},
		})
		denoised.frames = append(denoised.frames, Frame{
			Index: i, Timestamp: float64(i) / seq.FrameRate,
			Plane: Plane{Width: seq.Width, Height: seq.Height, Pix: seq.Denoised[i]},
		})
	}
	return original, denoised
}

func trajectoryFor(seq synth.Sequence) Trajectory {
	trajectory := Trajectory{Frames: make(map[int]Box)}
	for _, point := range seq.Trajectory {
		trajectory.Frames[point.Frame] = Box{
			X: point.X, Y: point.Y, Width: point.Width, Height: point.Height,
			VX: point.VX, VY: point.VY,
		}
	}
	return trajectory
}

func TestAnalyzeFindsEarliestTemporalGhost(t *testing.T) {
	seq := synth.Build()
	original, denoised := sourcesFor(seq)
	report, err := Analyze(original, denoised, trajectoryFor(seq), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstGhost == nil {
		t.Fatalf("no ghost found; counts=%v", report.Summary.Counts)
	}
	if report.FirstGhost.Frame != seq.FirstGhost {
		t.Fatalf("first ghost frame=%d, want %d; findings=%+v",
			report.FirstGhost.Frame, seq.FirstGhost, report.FirstGhost)
	}
	region := report.FirstGhost.Region
	expected := Box{X: 24, Y: 20, Width: 24, Height: 56}
	if !boxesOverlap(Box{X: region.X, Y: region.Y, Width: region.Width, Height: region.Height}, expected) {
		t.Fatalf("ghost region=%+v does not overlap departed region %+v", region, expected)
	}
	if report.FirstGhost.Evidence.TemporalScore < DefaultOptions().GhostThreshold {
		t.Fatalf("ghost temporal score too low: %+v", report.FirstGhost.Evidence)
	}
	if !containsString(report.FirstGhost.Secondary, "motion_blur") {
		t.Fatalf("blurred source edge was not retained as secondary evidence: %+v", report.FirstGhost)
	}
	if report.Summary.Counts["coding_block"] == 0 {
		t.Fatalf("block-coded patch was not distinguished; counts=%v", report.Summary.Counts)
	}
	if report.Frames[3].BackgroundResidual >= report.Frames[3].SourceNoise {
		t.Fatalf("background was not cleaner: residual=%f source_noise=%f",
			report.Frames[3].BackgroundResidual, report.Frames[3].SourceNoise)
	}
}

func TestParameterRegressionUsesSamePictures(t *testing.T) {
	seq := synth.Build()
	variants := []Options{
		DefaultOptions(),
		{GhostThreshold: 0.60, NoiseThreshold: 9, MotionThreshold: 14, HistoryFrames: 3,
			MinArea: 40, BlockSize: 8, BlockThreshold: 0.45, BlurWidthThreshold: 2.8, MaxFindings: 6},
		{GhostThreshold: 0.46, NoiseThreshold: 6, MotionThreshold: 10, HistoryFrames: 5,
			MinArea: 20, BlockSize: 16, BlockThreshold: 0.35, BlurWidthThreshold: 2.2, MaxFindings: 10},
	}
	var fingerprint string
	for i, options := range variants {
		original, denoised := sourcesFor(seq)
		report, err := Analyze(original, denoised, trajectoryFor(seq), options)
		if err != nil {
			t.Fatalf("variant %d: %v", i, err)
		}
		if fingerprint == "" {
			fingerprint = report.Input.SHA256
		} else if report.Input.SHA256 != fingerprint {
			t.Fatalf("variant %d used different pictures: %s != %s", i, report.Input.SHA256, fingerprint)
		}
		if report.Input.Frames != len(seq.Original) {
			t.Fatalf("variant %d frame count=%d", i, report.Input.Frames)
		}
	}
}

func TestAnalyzeEstimatesTrajectoryWhenOmitted(t *testing.T) {
	seq := synth.Build()
	original, denoised := sourcesFor(seq)
	report, err := Analyze(original, denoised, Trajectory{}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstGhost == nil || report.FirstGhost.Frame != seq.FirstGhost {
		t.Fatalf("auto trajectory first ghost=%+v, want frame %d", report.FirstGhost, seq.FirstGhost)
	}
	frame := report.Frames[seq.FirstGhost]
	if frame.Trajectory.VX <= 0 {
		t.Fatalf("auto trajectory did not estimate rightward motion: %+v", frame.Trajectory)
	}
}

func TestDetectsGhostAfterActorLeaves(t *testing.T) {
	const width, height = 64, 40
	background := makePlane(width, height, 60)
	original := [][]byte{
		withRectangle(background, 5, 10, 10, 20),
		withRectangle(background, 15, 10, 10, 20),
		append([]byte(nil), background.Pix...),
	}
	denoised := [][]byte{
		withRectangle(background, 5, 10, 10, 20),
		withRectangle(background, 15, 10, 10, 20),
		withRectangle(background, 5, 10, 10, 20),
	}
	format := Format{Width: width, Height: height, FrameRate: 25}
	originalSource := &sliceSource{format: format}
	denoisedSource := &sliceSource{format: format}
	for i := range original {
		originalSource.frames = append(originalSource.frames, Frame{
			Index: i, Timestamp: float64(i) / 25,
			Plane: Plane{Width: width, Height: height, Pix: original[i]},
		})
		denoisedSource.frames = append(denoisedSource.frames, Frame{
			Index: i, Timestamp: float64(i) / 25,
			Plane: Plane{Width: width, Height: height, Pix: denoised[i]},
		})
	}
	report, err := Analyze(originalSource, denoisedSource, Trajectory{}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if report.FirstGhost == nil || report.FirstGhost.Frame != 2 {
		t.Fatalf("first ghost=%+v, want frame 2 after actor leaves", report.FirstGhost)
	}
}

func TestFingerprintChangesWithOnePictureSample(t *testing.T) {
	first := synth.Build()
	original, denoised := sourcesFor(first)
	baseline, err := Analyze(original, denoised, trajectoryFor(first), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	second := synth.Build()
	second.Denoised[4][100] ^= 1
	original, denoised = sourcesFor(second)
	changed, err := Analyze(original, denoised, trajectoryFor(second), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Input.SHA256 == changed.Input.SHA256 {
		t.Fatal("fingerprint did not change after modifying a picture sample")
	}
}

func TestFingerprintIgnoresFrameRateMetadata(t *testing.T) {
	first := synth.Build()
	second := first
	second.FrameRate = 30
	original, denoised := sourcesFor(first)
	a, err := Analyze(original, denoised, trajectoryFor(first), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	original, denoised = sourcesFor(second)
	b, err := Analyze(original, denoised, trajectoryFor(second), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if a.Input.SHA256 != b.Input.SHA256 {
		t.Fatal("frame-rate metadata changed the picture fingerprint")
	}
	if a.Input.FrameRate == b.Input.FrameRate {
		t.Fatal("test did not vary frame-rate metadata")
	}
}

func withRectangle(background Plane, x, y, width, height int) []byte {
	pix := append([]byte(nil), background.Pix...)
	for py := y; py < y+height; py++ {
		for px := x; px < x+width; px++ {
			pix[py*background.Width+px] = 200
		}
	}
	return pix
}

func boxesOverlap(a, b Box) bool {
	return a.X < b.X+b.Width && b.X < a.X+a.Width &&
		a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
}
