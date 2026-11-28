package fieldcut

// OutField is one emitted output field (or frame, for progressive output),
// carrying its provenance for verification.
type OutField struct {
	Seq       int64    // output sequence index, 0-based
	SourceFld int64    // source field index this output came from
	Segment   int      // which cut/segment produced it
	PTS       Rational // exact presentation timestamp in seconds
	Dominance string   // "first" or "second" within its source frame
}

// Emit flattens normalized decisions into the output field sequence with
// exact rational timestamps anchored at t=0.
func Emit(decisions []Decision, src Source, out OutputSpec) []OutField {
	fdur := src.FieldDuration()
	var fields []OutField
	for seg, d := range decisions {
		for f := d.Cut.In; f < d.Cut.Out; f++ {
			fields = append(fields, OutField{
				Seq:       int64(len(fields)),
				SourceFld: f,
				Segment:   seg,
				PTS:       fdur.Scale(int64(len(fields))),
				Dominance: ParityOf(f),
			})
		}
	}
	return fields
}

// Issue is a single field-level verification failure.
type Issue struct {
	Seq    int64
	Kind   string
	Detail string
}

// VerifyFields checks an emitted sequence field by field:
//  1. PTS cadence: every step is exactly one field duration (no dup/gap).
//  2. Pair integrity: for pair-based output, fields 2k/2k+1 of every output
//     frame must come from the same source frame — a violation here is the
//     one-field flicker at a shot join.
//  3. Dominance alternation: first/second must alternate, and every output
//     frame must start on a "first" field.
func VerifyFields(fields []OutField, src Source, out OutputSpec) []Issue {
	var issues []Issue
	if len(fields) == 0 {
		return issues
	}
	fdur := src.FieldDuration()
	pairBased := out.FieldsPerFrame(src).Equal(R(2, 1))

	if pairBased && fields[0].Dominance != "first" {
		issues = append(issues, Issue{fields[0].Seq, "dominance",
			"output starts on a second field (source field " +
				itoa(fields[0].SourceFld) + "); head frame is a broken pair"})
	}
	for i, f := range fields {
		if i > 0 {
			step := R(f.PTS.Num*fields[i-1].PTS.Den-fields[i-1].PTS.Num*f.PTS.Den,
				f.PTS.Den*fields[i-1].PTS.Den)
			if !step.Equal(fdur) {
				issues = append(issues, Issue{f.Seq, "pts",
					"PTS step " + step.String() + "s != field duration " + fdur.String() + "s"})
			}
			if pairBased && f.Dominance == fields[i-1].Dominance {
				issues = append(issues, Issue{f.Seq, "dominance",
					"two consecutive " + f.Dominance + " fields (source " +
						itoa(fields[i-1].SourceFld) + " -> " + itoa(f.SourceFld) + ")"})
			}
		}
		if pairBased && f.Seq%2 == 1 {
			prev := fields[i-1]
			if prev.SourceFld/2 != f.SourceFld/2 {
				issues = append(issues, Issue{f.Seq, "pair-integrity",
					"output frame " + itoa(f.Seq/2) + " weaves source frames " +
						itoa(prev.SourceFld/2) + " and " + itoa(f.SourceFld/2) +
						" (segment " + itoa(int64(prev.Segment)) + " -> " +
						itoa(int64(f.Segment)) + "): one-field flicker"})
			}
		}
	}
	if pairBased && len(fields)%2 != 0 {
		issues = append(issues, Issue{fields[len(fields)-1].Seq, "pair-integrity",
			"odd total field count: last output frame is missing its mate"})
	}
	return issues
}
