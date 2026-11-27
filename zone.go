package sf2

// ResolvedZone is a playback-ready view: global generators merged under
// the zone's own generators, the linked sample resolved, and loop points
// and envelopes computed in absolute smpl-data coordinates.
type ResolvedZone struct {
	Preset     *Preset
	Instrument *Instrument
	Zone       *Zone // instrument zone (never the global zone)
	Generators []Generator
	Sample     *Sample

	KeyLo, KeyHi uint8
	VelLo, VelHi uint8

	// Absolute loop boundaries in sample frames (smpl coordinates),
	// with all coarse/fine offsets applied.
	LoopStart uint32
	LoopEnd   uint32
	LoopMode  uint16 // sampleModes value; 0 = no loop

	VolEnv Envelope
	ModEnv Envelope
}

// Envelope holds the six SF2 envelope parameters. Times are raw timecents
// (log2 seconds * 1200); Sustain is in centibels of attenuation.
type Envelope struct {
	Delay   int16
	Attack  int16
	Hold    int16
	Decay   int16
	Sustain int16
	Release int16
}

// mergedGenerators overlays local generators on top of global ones.
// Unknown opcodes from both levels are carried through untouched.
func mergedGenerators(global, local *Zone) []Generator {
	idx := map[uint16]int{}
	var out []Generator
	appendGen := func(g Generator) {
		if i, ok := idx[g.Op]; ok {
			out[i] = g
			return
		}
		idx[g.Op] = len(out)
		out = append(out, g)
	}
	if global != nil {
		for _, g := range global.Generators {
			appendGen(g)
		}
	}
	if local != nil {
		for _, g := range local.Generators {
			appendGen(g)
		}
	}
	return out
}

type genSet struct{ m map[uint16]Generator }

func newGenSet(gens []Generator) genSet {
	s := genSet{m: make(map[uint16]Generator, len(gens))}
	for _, g := range gens {
		s.m[g.Op] = g
	}
	return s
}

func (s genSet) amount(op uint16, def int16) int16 {
	if g, ok := s.m[op]; ok {
		return g.Amount()
	}
	return def
}

func (s genSet) word(op uint16, def uint16) uint16 {
	if g, ok := s.m[op]; ok {
		return g.Value
	}
	return def
}

func (s genSet) keyRange() (uint8, uint8) {
	if g, ok := s.m[GenKeyRange]; ok {
		return g.Range()
	}
	return 0, 127
}

func (s genSet) velRange() (uint8, uint8) {
	if g, ok := s.m[GenVelRange]; ok {
		return g.Range()
	}
	return 0, 127
}

func (s genSet) envelope(delay, attack, hold, decay, sustain, release uint16) Envelope {
	return Envelope{
		Delay:   s.amount(delay, -12000),
		Attack:  s.amount(attack, -12000),
		Hold:    s.amount(hold, -12000),
		Decay:   s.amount(decay, -12000),
		Sustain: s.amount(sustain, 0),
		Release: s.amount(release, -12000),
	}
}

// ResolvePreset expands a preset into per-instrument resolved zones,
// following instrument links and resolving sample links. Multi-layer
// key ranges are preserved as separate zones.
func (sf *SoundFont) ResolvePreset(p *Preset) []*ResolvedZone {
	var pGlobal *Zone
	for _, z := range p.Zones {
		if z.Global {
			pGlobal = z
			break
		}
	}
	var out []*ResolvedZone
	for _, pz := range p.Zones {
		if pz.Global {
			continue
		}
		ig, ok := pz.Get(GenInstrument)
		if !ok || int(ig.Value) >= len(sf.Instruments) {
			continue
		}
		inst := sf.Instruments[ig.Value]
		pGens := newGenSet(mergedGenerators(pGlobal, pz))
		var iGlobal *Zone
		for _, z := range inst.Zones {
			if z.Global {
				iGlobal = z
				break
			}
		}
		for _, iz := range inst.Zones {
			if iz.Global {
				continue
			}
			sg, ok := iz.Get(GenSampleID)
			if !ok || int(sg.Value) >= len(sf.Samples) {
				continue
			}
			iGens := newGenSet(mergedGenerators(iGlobal, iz))
			rz := &ResolvedZone{
				Preset:     p,
				Instrument: inst,
				Zone:       iz,
				Generators: mergedGenerators(iGlobal, iz),
				Sample:     sf.Samples[sg.Value],
			}
			// Preset-level key/vel ranges narrow the instrument ranges.
			kLo, kHi := iGens.keyRange()
			if lo, hi := pGens.keyRange(); lo != 0 || hi != 127 {
				if lo > kLo {
					kLo = lo
				}
				if hi < kHi {
					kHi = hi
				}
			}
			rz.KeyLo, rz.KeyHi = kLo, kHi
			vLo, vHi := iGens.velRange()
			if lo, hi := pGens.velRange(); lo != 0 || hi != 127 {
				if lo > vLo {
					vLo = lo
				}
				if hi < vHi {
					vHi = hi
				}
			}
			rz.VelLo, rz.VelHi = vLo, vHi
			rz.LoopMode = iGens.word(GenSampleModes, 0)
			rz.LoopStart, rz.LoopEnd = computeLoop(rz.Sample, iGens, pGens)
			rz.VolEnv = iGens.envelope(GenDelayVolEnv, GenAttackVolEnv, GenHoldVolEnv,
				GenDecayVolEnv, GenSustainVolEnv, GenReleaseVolEnv)
			rz.ModEnv = iGens.envelope(GenDelayModEnv, GenAttackModEnv, GenHoldModEnv,
				GenDecayModEnv, GenSustainModEnv, GenReleaseModEnv)
			out = append(out, rz)
		}
	}
	return out
}

// computeLoop applies fine and coarse loop offsets from both instrument
// and preset level to the sample header's loop points.
func computeLoop(s *Sample, iGens, pGens genSet) (start, end uint32) {
	off := func(fine, coarse uint16) int64 {
		return int64(iGens.amount(fine, 0)) + int64(pGens.amount(fine, 0)) +
			32768*int64(iGens.amount(coarse, 0)) + 32768*int64(pGens.amount(coarse, 0))
	}
	so := off(GenStartloopAddrsOffset, GenStartloopAddrsCoarse)
	eo := off(GenEndloopAddrsOffset, GenEndloopAddrsCoarse)
	start = clampU32(int64(s.StartLoop) + so)
	end = clampU32(int64(s.EndLoop) + eo)
	return start, end
}

func clampU32(v int64) uint32 {
	if v < 0 {
		return 0
	}
	if v > 0xFFFFFFFF {
		return 0xFFFFFFFF
	}
	return uint32(v)
}
