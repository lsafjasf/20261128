package sf2

import "fmt"

// Severity classifies a validation issue.
type Severity int

const (
	Warning Severity = iota
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "ERROR"
	}
	return "WARN"
}

// Issue is a single validation finding.
type Issue struct {
	Severity Severity
	Context  string // e.g. `preset 0/0 "Piano" zone 2`
	Message  string
}

func (i Issue) String() string {
	return fmt.Sprintf("%s %s: %s", i.Severity, i.Context, i.Message)
}

// Validate runs all structural and playback-relevant checks on the font.
func (sf *SoundFont) Validate() []Issue {
	var out []Issue
	for _, s := range sf.Samples {
		out = append(out, sf.validateSample(s)...)
	}
	for _, p := range sf.Presets {
		out = append(out, sf.validatePreset(p)...)
	}
	for _, inst := range sf.Instruments {
		out = append(out, sf.validateInstrument(inst)...)
	}
	return out
}

func (sf *SoundFont) validateSample(s *Sample) []Issue {
	var out []Issue
	ctx := fmt.Sprintf("sample %q", s.Name)
	dataLen := uint32(len(sf.SampleData))
	if s.End < s.Start {
		out = append(out, Issue{Error, ctx, fmt.Sprintf("end %d before start %d", s.End, s.Start)})
	}
	if s.End > dataLen {
		out = append(out, Issue{Error, ctx, fmt.Sprintf("end %d past smpl data length %d", s.End, dataLen)})
	}
	if s.StartLoop < s.Start {
		out = append(out, Issue{Error, ctx, fmt.Sprintf("loop start %d before sample start %d", s.StartLoop, s.Start)})
	}
	if s.EndLoop > s.End {
		out = append(out, Issue{Error, ctx, fmt.Sprintf("loop end %d past sample end %d (truncation risk)", s.EndLoop, s.End)})
	}
	if s.EndLoop <= s.StartLoop {
		out = append(out, Issue{Warning, ctx, fmt.Sprintf("empty loop [%d, %d)", s.StartLoop, s.EndLoop)})
	} else if s.EndLoop-s.StartLoop < 32 {
		out = append(out, Issue{Warning, ctx, fmt.Sprintf("loop length %d below 32-frame minimum", s.EndLoop-s.StartLoop)})
	}
	// Linked stereo pairs must reference each other consistently.
	switch s.Type {
	case 1, 4, 2, 8: // mono, right, left, linked
		if s.Type != 1 {
			if int(s.SampleLink) >= len(sf.Samples) {
				out = append(out, Issue{Error, ctx, fmt.Sprintf("sample link index %d out of range", s.SampleLink)})
			} else if link := sf.Samples[s.SampleLink]; link.SampleLink != uint16(indexOf(sf.Samples, s)) {
				out = append(out, Issue{Warning, ctx, fmt.Sprintf("link to %q is not reciprocal", link.Name)})
			}
		}
	default:
		out = append(out, Issue{Warning, ctx, fmt.Sprintf("unknown sample type %d", s.Type)})
	}
	return out
}

func indexOf(samples []*Sample, s *Sample) int {
	for i, x := range samples {
		if x == s {
			return i
		}
	}
	return -1
}

func (sf *SoundFont) validatePreset(p *Preset) []Issue {
	var out []Issue
	ctx := fmt.Sprintf("preset %d/%d %q", p.Bank, p.Number, p.Name)
	zones := sf.ResolvePreset(p)
	if len(zones) == 0 {
		return append(out, Issue{Warning, ctx, "no playable zones"})
	}
	// Key coverage: every key 0-127 should be served by at least one zone.
	var covered [128]bool
	for _, rz := range zones {
		if rz.KeyLo > rz.KeyHi {
			out = append(out, Issue{Error, ctx, fmt.Sprintf("inverted key range %d-%d", rz.KeyLo, rz.KeyHi)})
			continue
		}
		for k := rz.KeyLo; k <= rz.KeyHi && k < 128; k++ {
			covered[k] = true
		}
	}
	lo := -1
	for k := 0; k <= 128; k++ {
		coveredK := k < 128 && covered[k]
		if !coveredK && lo < 0 {
			lo = k
		}
		if coveredK && lo >= 0 {
			out = append(out, Issue{Warning, ctx, fmt.Sprintf("keys %d-%d not covered by any zone", lo, k-1)})
			lo = -1
		}
	}
	if lo >= 0 && lo < 128 {
		out = append(out, Issue{Warning, ctx, fmt.Sprintf("keys %d-%d not covered by any zone", lo, 127)})
	}
	for i, rz := range zones {
		out = append(out, validateResolved(rz, fmt.Sprintf("%s zone %d", ctx, i))...)
	}
	return out
}

func (sf *SoundFont) validateInstrument(inst *Instrument) []Issue {
	var out []Issue
	ctx := fmt.Sprintf("instrument %q", inst.Name)
	for i, z := range inst.Zones {
		if z.Global {
			continue
		}
		g, ok := z.Get(GenSampleID)
		if !ok {
			out = append(out, Issue{Error, fmt.Sprintf("%s zone %d", ctx, i), "non-global zone missing sampleID"})
			continue
		}
		if int(g.Value) >= len(sf.Samples) {
			out = append(out, Issue{Error, fmt.Sprintf("%s zone %d", ctx, i),
				fmt.Sprintf("sampleID %d out of range (%d samples)", g.Value, len(sf.Samples))})
		}
	}
	return out
}

// validateResolved checks loop boundaries and envelope ranges on a
// fully-resolved zone (offsets applied, sample linked).
func validateResolved(rz *ResolvedZone, ctx string) []Issue {
	var out []Issue
	s := rz.Sample
	ctx = fmt.Sprintf("%s -> sample %q", ctx, s.Name)
	if rz.LoopMode&ModeLoopContinuous != 0 {
		if rz.LoopStart < s.Start {
			out = append(out, Issue{Error, ctx, fmt.Sprintf("loop start %d before sample start %d", rz.LoopStart, s.Start)})
		}
		if rz.LoopEnd > s.End {
			out = append(out, Issue{Error, ctx, fmt.Sprintf("loop end %d past sample end %d: playback truncates", rz.LoopEnd, s.End)})
		}
		if rz.LoopEnd <= rz.LoopStart {
			out = append(out, Issue{Error, ctx, fmt.Sprintf("loop end %d not after loop start %d", rz.LoopEnd, rz.LoopStart)})
		} else if rz.LoopEnd-rz.LoopStart < 32 {
			out = append(out, Issue{Warning, ctx, fmt.Sprintf("loop length %d below 32-frame minimum", rz.LoopEnd-rz.LoopStart)})
		}
	}
	out = append(out, checkEnvelope(rz.VolEnv, "volEnv", ctx)...)
	out = append(out, checkEnvelope(rz.ModEnv, "modEnv", ctx)...)
	return out
}

func checkEnvelope(e Envelope, name, ctx string) []Issue {
	var out []Issue
	check := func(param string, v int16, lo, hi int32) {
		if int32(v) < lo || int32(v) > hi {
			out = append(out, Issue{Warning, ctx, fmt.Sprintf("%s.%s %d outside [%d, %d] timecents", name, param, v, lo, hi)})
		}
	}
	check("delay", e.Delay, -12000, 5000)
	check("attack", e.Attack, -12000, 8000)
	check("hold", e.Hold, -12000, 5000)
	check("decay", e.Decay, -12000, 8000)
	check("release", e.Release, -12000, 8000)
	if e.Sustain < 0 || e.Sustain > 1440 {
		out = append(out, Issue{Warning, ctx, fmt.Sprintf("%s.sustain %d cB outside [0, 1440]", name, e.Sustain)})
	}
	return out
}
