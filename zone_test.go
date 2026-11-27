package sf2

import (
	"strings"
	"testing"
)

func keyRangeGen(lo, hi uint8) Generator {
	return Generator{Op: GenKeyRange, Value: uint16(lo) | uint16(hi)<<8}
}

// multiLayerFont builds a font with two layered zones over the same key
// range, a global instrument zone, loop offsets, and envelope generators.
func multiLayerFont() *testFontBuilder {
	return &testFontBuilder{
		smplLen: 10000,
		samples: []testSample{
			{name: "pianoL", start: 0, end: 4000, startLoop: 1000, endLoop: 3900, rate: 44100, pitch: 60, typ: 1},
			{name: "pianoR", start: 4000, end: 8000, startLoop: 5000, endLoop: 7900, rate: 44100, pitch: 60, typ: 1},
		},
		insts: []testInst{
			{
				name: "Piano",
				zones: [][]Generator{
					{ // global zone
						{Op: GenAttackVolEnv, Value: uint16(0xFFFF - 8000 + 1)},
						{Op: GenSustainVolEnv, Value: 250},
					},
					{ // layer 1
						keyRangeGen(0, 127),
						{Op: GenSampleModes, Value: ModeLoopContinuous},
						{Op: GenReleaseVolEnv, Value: uint16(0xFFFF - 3000 + 1)},
						{Op: GenSampleID, Value: 0},
					},
					{ // layer 2 with loop offsets
						keyRangeGen(0, 127),
						{Op: GenSampleModes, Value: ModeLoopContinuous},
						{Op: GenStartloopAddrsOffset, Value: uint16(int16(50))},
						{Op: GenEndloopAddrsOffset, Value: uint16(0xFFFF - 50 + 1)},
						{Op: GenSampleID, Value: 1},
					},
				},
			},
		},
		presets: []testPreset{
			{
				name: "Piano", bank: 0, num: 0,
				zones: [][]Generator{
					{{Op: GenInstrument, Value: 0}},
				},
			},
		},
	}
}

func TestParseMultiLayerZones(t *testing.T) {
	sf := parseTestFont(t, multiLayerFont().build())
	if sf.BankName != "test bank" || sf.Version != [2]uint16{1, 2} {
		t.Fatalf("INFO: %+v %v", sf.BankName, sf.Version)
	}
	if len(sf.Presets) != 1 || len(sf.Instruments) != 1 || len(sf.Samples) != 2 {
		t.Fatalf("counts: %d presets %d insts %d samples",
			len(sf.Presets), len(sf.Instruments), len(sf.Samples))
	}
	zones := sf.ResolvePreset(sf.Presets[0])
	if len(zones) != 2 {
		t.Fatalf("expected 2 resolved layers, got %d", len(zones))
	}
	for _, rz := range zones {
		if rz.KeyLo != 0 || rz.KeyHi != 127 {
			t.Errorf("key range %d-%d", rz.KeyLo, rz.KeyHi)
		}
		if rz.LoopMode&ModeLoopContinuous == 0 {
			t.Errorf("loop mode lost")
		}
	}
	// Layer 1: raw header loop points.
	if zones[0].LoopStart != 1000 || zones[0].LoopEnd != 3900 {
		t.Errorf("layer1 loop [%d,%d]", zones[0].LoopStart, zones[0].LoopEnd)
	}
	// Layer 2: offsets applied.
	if zones[1].LoopStart != 5050 || zones[1].LoopEnd != 7850 {
		t.Errorf("layer2 loop [%d,%d]", zones[1].LoopStart, zones[1].LoopEnd)
	}
	// Global envelope values merged into both layers.
	for _, rz := range zones {
		if rz.VolEnv.Attack != -8000 || rz.VolEnv.Sustain != 250 {
			t.Errorf("global env not merged: %+v", rz.VolEnv)
		}
	}
	// Local release overrides the default.
	if zones[0].VolEnv.Release != -3000 {
		t.Errorf("release %d", zones[0].VolEnv.Release)
	}
	if zones[1].VolEnv.Release != -12000 {
		t.Errorf("default release %d", zones[1].VolEnv.Release)
	}
	// Sample link resolved.
	if zones[0].Sample.Name != "pianoL" || zones[1].Sample.Name != "pianoR" {
		t.Errorf("sample links: %q %q", zones[0].Sample.Name, zones[1].Sample.Name)
	}
}

func TestUnknownGeneratorsPreserved(t *testing.T) {
	tf := multiLayerFont()
	// Opcode 200 is not defined by the SF2 spec.
	tf.insts[0].zones[1] = append(tf.insts[0].zones[1], Generator{Op: 200, Value: 0xBEEF})
	sf := parseTestFont(t, tf.build())
	z := sf.Instruments[0].Zones[1]
	g, ok := z.Get(200)
	if !ok {
		t.Fatal("unknown generator dropped from raw zone")
	}
	if g.Value != 0xBEEF || g.Known() {
		t.Fatalf("unknown generator mangled: %+v", g)
	}
	// It must also survive zone merging.
	rz := sf.ResolvePreset(sf.Presets[0])[0]
	found := false
	for _, mg := range rz.Generators {
		if mg.Op == 200 && mg.Value == 0xBEEF {
			found = true
		}
	}
	if !found {
		t.Fatal("unknown generator lost during merge")
	}
}

func TestLoopBoundaryValidation(t *testing.T) {
	tf := multiLayerFont()
	// Push layer 2's loop end past the sample end via a coarse offset.
	tf.insts[0].zones[2] = append(tf.insts[0].zones[2],
		Generator{Op: GenEndloopAddrsCoarse, Value: 1})
	sf := parseTestFont(t, tf.build())
	issues := sf.Validate()
	var found bool
	for _, i := range issues {
		if i.Severity == Error && strings.Contains(i.Message, "past sample end") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected loop-end error, got %v", issues)
	}
}

func TestKeyCoverageGap(t *testing.T) {
	tf := multiLayerFont()
	// Narrow both layers to keys 21-108, leaving 0-20 and 109-127 bare.
	tf.insts[0].zones[1][0] = keyRangeGen(21, 108)
	tf.insts[0].zones[2][0] = keyRangeGen(21, 108)
	sf := parseTestFont(t, tf.build())
	issues := sf.Validate()
	gaps := 0
	for _, i := range issues {
		if strings.Contains(i.Message, "not covered") {
			gaps++
		}
	}
	if gaps != 2 {
		t.Fatalf("expected 2 coverage gaps, got %d: %v", gaps, issues)
	}
}

func TestSampleHeaderLoopOutOfBounds(t *testing.T) {
	tf := multiLayerFont()
	tf.samples[0].endLoop = 9999 // past sample end 4000
	sf := parseTestFont(t, tf.build())
	issues := sf.Validate()
	var found bool
	for _, i := range issues {
		if i.Severity == Error && strings.Contains(i.Message, "truncation risk") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected header loop error, got %v", issues)
	}
}
