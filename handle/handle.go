// Package handle computes transition-handle margins at export-range
// boundaries and keeps subtitle/audio offsets in sync when the range moves.
//
// All timestamps are integer milliseconds to avoid floating-point drift.
package handle

// HandleStatus classifies how a boundary transition must be produced.
type HandleStatus int

const (
	// Lossless: real material covers the full transition handle.
	Lossless HandleStatus = iota
	// Freeze: partial real material; the deficit is filled by freezing
	// the boundary frame (still lossless for the real part).
	Freeze
	// Reencode: no material at all beyond the cut; the whole transition
	// must be synthesized (freeze + re-encode of the handle segment).
	Reencode
)

func (s HandleStatus) String() string {
	switch s {
	case Lossless:
		return "lossless"
	case Freeze:
		return "freeze"
	case Reencode:
		return "reencode"
	}
	return "unknown"
}

// Boundary describes one edge (head or tail) of the export range.
type Boundary struct {
	// Available is the real material beyond the cut point, in ms.
	Available int64
	// Deficit is the part of the required handle not covered by real
	// material, in ms. Zero when Status is Lossless.
	Deficit int64
	// FreezeMs is how long the boundary frame must be held, in ms.
	FreezeMs int64
	Status   HandleStatus
}

// RangePlan is the full export plan for one clip.
type RangePlan struct {
	// SourceDuration is the total clip length in ms.
	SourceDuration int64
	// In/Out are the user cut points in source time, ms. Out is exclusive.
	In  int64
	Out int64
	// Handle is the required transition length on each side, ms.
	Handle int64

	Head Boundary
	Tail Boundary

	// ExportStart/ExportEnd are the timeline extents of the exported
	// segment in source time, ms (clamped to [0, SourceDuration]).
	ExportStart int64
	ExportEnd   int64
}
