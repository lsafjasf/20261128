package sf2

// SoundFont is the parsed representation of an SF2 file.
type SoundFont struct {
	Version     [2]uint16 // ifil: major, minor
	SoundEngine string    // isng
	BankName    string    // INAM
	Presets     []*Preset
	Instruments []*Instrument
	Samples     []*Sample
	SampleData  []int16 // raw smpl payload
}

// Preset is a phdr entry with its zones.
type Preset struct {
	Name       string
	Number     uint16
	Bank       uint16
	Zones      []*Zone
	Library    uint32
	Genre      uint32
	Morphology uint32
}

// Instrument is an inst entry with its zones.
type Instrument struct {
	Name  string
	Zones []*Zone
}

// Zone is one bag: the generators (and modulators, kept raw) declared for
// it. The first zone of a preset/instrument is global when it carries no
// sampleID/instrument generator respectively.
type Zone struct {
	Generators []Generator
	Modulators []Modulator
	Global     bool
}

// Modulator is a raw pmod/imod entry, preserved verbatim.
type Modulator struct {
	SrcOper       uint16
	DestOper      uint16
	Amount        int16
	AmtSrcOper    uint16
	TransformOper uint16
}

// Sample is an shdr entry. Offsets are relative to the start of smpl data.
type Sample struct {
	Name            string
	Start           uint32
	End             uint32
	StartLoop       uint32
	EndLoop         uint32
	SampleRate      uint32
	OriginalPitch   uint8
	PitchCorrection int8
	SampleLink      uint16
	Type            uint16
}

// SampleMode flags from the sampleModes generator.
const (
	ModeLoopContinuous uint16 = 1
	ModeLoopRelease    uint16 = 2 // loop until note-off, then release (value 3)
)

// Get returns the last generator with the given opcode in the zone
// (later entries override earlier ones per the spec).
func (z *Zone) Get(op uint16) (Generator, bool) {
	for i := len(z.Generators) - 1; i >= 0; i-- {
		if z.Generators[i].Op == op {
			return z.Generators[i], true
		}
	}
	return Generator{}, false
}

// KeyRange returns the zone's key range, defaulting to 0-127.
func (z *Zone) KeyRange() (lo, hi uint8) {
	if g, ok := z.Get(GenKeyRange); ok {
		return g.Range()
	}
	return 0, 127
}

// VelRange returns the zone's velocity range, defaulting to 0-127.
func (z *Zone) VelRange() (lo, hi uint8) {
	if g, ok := z.Get(GenVelRange); ok {
		return g.Range()
	}
	return 0, 127
}
