package fieldcut

// Action is the verdict of the joint field-order/cut/output-rate model.
type Action int

const (
	// KeepOddTrim: the cut lands on an odd field index and that is legal
	// for this output mode (each field becomes its own output frame, or
	// timestamps are fully resampled). No expansion needed.
	KeepOddTrim Action = iota
	// KeepPair: the cut is already pair-aligned; nothing to do.
	KeepPair
	// ExpandToPair: pair-based output would weave fields from two
	// different source frames into one output frame at the cut, which is
	// exactly the one-field flicker seen across the join. The cut must be
	// expanded outward to the enclosing field pair.
	ExpandToPair
)

func (a Action) String() string {
	switch a {
	case KeepOddTrim:
		return "keep-odd-trim"
	case KeepPair:
		return "keep-pair"
	default:
		return "expand-to-pair"
	}
}

// Decision is the normalized cut plus an auditable explanation.
type Decision struct {
	Action Action
	Cut    Cut // normalized cut actually applied
	Reason string
}

// NormalizeCut applies the joint model: given the source field cadence and
// the output frame mapping, decide whether an odd-field trim is legal or
// must be expanded to a field pair.
func NormalizeCut(c Cut, src Source, out OutputSpec) Decision {
	fpf := out.FieldsPerFrame(src)
	pairBased := fpf.Equal(R(2, 1))

	if c.OnPairBoundary() {
		return Decision{KeepPair, c, "cut already on field-pair boundary"}
	}
	if !pairBased {
		return Decision{KeepOddTrim, c,
			"output consumes " + fpf.String() + " field(s) per frame; " +
				"odd-field trim at " + itoa(c.In) + "/" + itoa(c.Out) + " is legal"}
	}

	// Pair-based output with a non-pair cut: expanding outward keeps every
	// output frame woven from one source frame. Trimming inward would drop
	// the mate of an already-included field and still break a pair.
	fixed := Cut{In: c.In - c.In%2, Out: c.Out + c.Out%2}
	return Decision{ExpandToPair, fixed,
		"pair-based output (" + out.Rate.String() + " fps) cannot split frame " +
			itoa(c.In/2) + "/" + itoa(c.Out/2) + "; expanded [" + itoa(c.In) + "," +
			itoa(c.Out) + ") -> [" + itoa(fixed.In) + "," + itoa(fixed.Out) + ")"}
}

// NormalizeTimeline normalizes a shot list and merges segments that became
// adjacent after expansion, so no duplicated source field survives.
func NormalizeTimeline(cuts []Cut, src Source, out OutputSpec) []Decision {
	decisions := make([]Decision, 0, len(cuts))
	for _, c := range cuts {
		d := NormalizeCut(c, src, out)
		if n := len(decisions); n > 0 && d.Action == ExpandToPair {
			prev := &decisions[n-1]
			if d.Cut.In < prev.Cut.Out {
				// Expansion overlapped the previous segment: re-cut both at
				// the shared pair boundary so no field is emitted twice.
				boundary := prev.Cut.Out
				prev.Cut.Out = boundary
				d.Cut.In = boundary
				prev.Reason += "; tail re-cut at pair boundary " + itoa(boundary)
				d.Reason += "; head re-cut at pair boundary " + itoa(boundary)
			}
		}
		decisions = append(decisions, d)
	}
	return decisions
}
