package handle

import "fmt"

// Plan computes the export plan for a cut range on a source clip.
//
// sourceDuration: total clip length in ms (must be > 0).
// in, out: user cut points in source time, ms; 0 <= in < out <= sourceDuration.
// handleMs: required transition length on each side, ms (>= 0).
func Plan(sourceDuration, in, out, handleMs int64) (RangePlan, error) {
	if sourceDuration <= 0 {
		return RangePlan{}, fmt.Errorf("handle: source duration must be positive, got %d", sourceDuration)
	}
	if handleMs < 0 {
		return RangePlan{}, fmt.Errorf("handle: handle must be >= 0, got %d", handleMs)
	}
	if in < 0 || out > sourceDuration || in >= out {
		return RangePlan{}, fmt.Errorf("handle: invalid range [%d,%d) for duration %d", in, out, sourceDuration)
	}

	p := RangePlan{
		SourceDuration: sourceDuration,
		In:             in,
		Out:            out,
		Handle:         handleMs,
	}

	// Head: real material before the in-point is [0, in).
	p.Head = classify(in, handleMs)
	// Tail: real material after the out-point is [out, sourceDuration).
	p.Tail = classify(sourceDuration-out, handleMs)

	// Exported timeline always spans the full handle on both sides;
	// the real-material part is clamped to the source, the rest is
	// synthesized (freeze/re-encode) according to each Boundary.
	p.ExportStart = in - min64(in, handleMs)
	p.ExportEnd = out + min64(sourceDuration-out, handleMs)
	return p, nil
}

// classify turns an available margin into a Boundary decision.
func classify(available, handle int64) Boundary {
	b := Boundary{Available: available}
	switch {
	case available >= handle:
		b.Status = Lossless
	case available > 0:
		b.Status = Freeze
		b.Deficit = handle - available
		b.FreezeMs = b.Deficit
	default: // available <= 0
		b.Status = Reencode
		b.Deficit = handle
		b.FreezeMs = handle
	}
	return b
}

// TotalDuration is the exported segment length in ms, including handles.
func (p RangePlan) TotalDuration() int64 {
	return p.ExportEnd - p.ExportStart
}

// NeedsReencode reports whether any boundary requires re-encoding.
func (p RangePlan) NeedsReencode() bool {
	return p.Head.Status == Reencode || p.Tail.Status == Reencode
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
