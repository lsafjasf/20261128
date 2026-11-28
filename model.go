package fieldcut

// FieldOrder declares which field of a frame is temporally first.
type FieldOrder int

const (
	TopFirst FieldOrder = iota
	BottomFirst
)

func (o FieldOrder) String() string {
	if o == TopFirst {
		return "TFF"
	}
	return "BFF"
}

// ParityOf returns the dominance ("first" or "second") of a source field
// index. Field 2k and 2k+1 together form source frame k; 2k is always the
// temporally first field regardless of TFF/BFF.
func ParityOf(fieldIndex int64) string {
	if fieldIndex%2 == 0 {
		return "first"
	}
	return "second"
}

// Source describes interlaced input material.
type Source struct {
	FieldRate Rational   // fields per second, e.g. R(60000, 1001) for 59.94i
	Order     FieldOrder // dominance of the stored fields
}

// FrameRate is the pair rate: FieldRate / 2.
func (s Source) FrameRate() Rational { return s.FieldRate.Div(R(2, 1)) }

// FieldDuration is the exact duration of one field in seconds.
func (s Source) FieldDuration() Rational { return R(1, 1).Div(s.FieldRate) }

// OutputKind selects how source fields are mapped onto the output raster.
type OutputKind int

const (
	// InterlacedOut: one output frame consumes one field pair (2 fields).
	InterlacedOut OutputKind = iota
	// ProgressiveFrameRateOut: deinterlaced, one output frame per field pair.
	ProgressiveFrameRateOut
	// ProgressiveFieldRateOut: bob/double-rate, one output frame per field.
	ProgressiveFieldRateOut
	// ResampledOut: arbitrary output rate, timestamps are regenerated.
	ResampledOut
)

// OutputSpec is the output side of the joint model.
type OutputSpec struct {
	Kind  OutputKind
	Rate  Rational // output frames per second
	Order FieldOrder
}

// FieldsPerFrame reports how many source fields one output frame consumes.
// The result is exact: 2 for pair-based output, 1 for field-rate output,
// and a rational ratio for resampled output.
func (o OutputSpec) FieldsPerFrame(src Source) Rational {
	switch o.Kind {
	case InterlacedOut, ProgressiveFrameRateOut:
		return R(2, 1)
	case ProgressiveFieldRateOut:
		return R(1, 1)
	default:
		return src.FieldRate.Div(o.Rate)
	}
}

// Cut is an edit decision in source field units. In is inclusive,
// Out is exclusive: the segment covers fields [In, Out).
type Cut struct {
	In  int64
	Out int64
}

func (c Cut) Len() int64 { return c.Out - c.In }

// OnPairBoundary reports whether both edges fall on field-pair boundaries.
func (c Cut) OnPairBoundary() bool { return c.In%2 == 0 && c.Out%2 == 0 }
