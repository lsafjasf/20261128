package captionsession

// NewState returns an empty materialized view.
func NewState() *State {
	return &State{segments: map[string]Segment{}}
}

// Apply materializes a snapshot followed by an ordered op prefix. It is the
// single replay path shared by live viewers, reconnecting viewers and the
// server itself: identical version prefixes always yield identical state.
func Apply(snap *Snapshot, ops []Op) (*State, error) {
	st := NewState()
	if snap != nil {
		if err := st.apply(Op{Kind: OpSnapshot, Snapshot: snap}); err != nil {
			return nil, err
		}
	}
	for _, op := range ops {
		if err := st.apply(op); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (st *State) apply(op Op) error {
	switch op.Kind {
	case OpSnapshot:
		snap := op.Snapshot
		st.Version = snap.BaseVersion
		st.Watermark = snap.Watermark
		st.segments = make(map[string]Segment, len(snap.Segments))
		st.order = st.order[:0]
		for _, seg := range snap.Segments {
			st.segments[seg.ID] = seg
			st.order = append(st.order, seg.ID)
		}
	case OpUpsert:
		seg := *op.Segment
		if _, exists := st.segments[seg.ID]; !exists {
			st.order = append(st.order, seg.ID)
		}
		st.segments[seg.ID] = seg
	case OpReplace:
		seg, ok := st.segments[op.SegmentID]
		if !ok {
			return ErrSegmentNotFound
		}
		seg.Text = op.NewText
		st.segments[seg.ID] = seg
	case OpWatermark:
		if op.Watermark > st.Watermark {
			st.Watermark = op.Watermark
		}
	default:
		return ErrInvalidSegment
	}
	if op.Kind != OpSnapshot {
		// Snapshot versions come from BaseVersion; the wrapping Op itself
		// may carry version zero when applied standalone.
		st.Version = op.Version
	}
	return nil
}

// Get returns a segment and whether it exists.
func (st *State) Get(id string) (Segment, bool) {
	seg, ok := st.segments[id]
	return seg, ok
}

// Segments returns all segments in presentation order.
func (st *State) Segments() []Segment {
	out := make([]Segment, 0, len(st.order))
	for _, id := range st.order {
		out = append(out, st.segments[id])
	}
	return out
}

// Committed returns segments already presented to the audience. These are
// frozen and cannot be revised.
func (st *State) Committed() []Segment {
	out := make([]Segment, 0)
	for _, id := range st.order {
		seg := st.segments[id]
		if seg.EndPTS <= st.Watermark {
			out = append(out, seg)
		}
	}
	return out
}

// Provisional returns segments still mutable by ASR revisions or operator
// replacements.
func (st *State) Provisional() []Segment {
	out := make([]Segment, 0)
	for _, id := range st.order {
		seg := st.segments[id]
		if seg.EndPTS > st.Watermark {
			out = append(out, seg)
		}
	}
	return out
}

// IsFrozen reports whether the segment has already been displayed.
func (st *State) IsFrozen(id string) bool {
	seg, ok := st.segments[id]
	return ok && seg.EndPTS <= st.Watermark
}
