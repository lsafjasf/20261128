package sf2

// Generator opcodes from the SoundFont 2.01 specification (section 8.1.2).
const (
	GenStartAddrsOffset       uint16 = 0
	GenEndAddrsOffset         uint16 = 1
	GenStartloopAddrsOffset   uint16 = 2
	GenEndloopAddrsOffset     uint16 = 3
	GenStartAddrsCoarseOffset uint16 = 4
	GenModLFOtoPitch          uint16 = 5
	GenVibLFOtoPitch          uint16 = 6
	GenModEnvtoPitch          uint16 = 7
	GenInitialFilterFc        uint16 = 8
	GenInitialFilterQ         uint16 = 9
	GenModLFOtoFilterFc       uint16 = 10
	GenModEnvtoFilterFc       uint16 = 11
	GenEndAddrsCoarseOffset   uint16 = 12
	GenModLFOtoVolume         uint16 = 13
	GenChorusEffectsSend      uint16 = 15
	GenReverbEffectsSend      uint16 = 16
	GenPan                    uint16 = 17
	GenDelayModLFO            uint16 = 21
	GenFreqModLFO             uint16 = 22
	GenDelayVibLFO            uint16 = 23
	GenFreqVibLFO             uint16 = 24
	GenDelayModEnv            uint16 = 25
	GenAttackModEnv           uint16 = 26
	GenHoldModEnv             uint16 = 27
	GenDecayModEnv            uint16 = 28
	GenSustainModEnv          uint16 = 29
	GenReleaseModEnv          uint16 = 30
	GenKeynumToModEnvHold     uint16 = 31
	GenKeynumToModEnvDecay    uint16 = 32
	GenDelayVolEnv            uint16 = 33
	GenAttackVolEnv           uint16 = 34
	GenHoldVolEnv             uint16 = 35
	GenDecayVolEnv            uint16 = 36
	GenSustainVolEnv          uint16 = 37
	GenReleaseVolEnv          uint16 = 38
	GenKeynumToVolEnvHold     uint16 = 39
	GenKeynumToVolEnvDecay    uint16 = 40
	GenInstrument             uint16 = 41
	GenKeyRange               uint16 = 43
	GenVelRange               uint16 = 44
	GenStartloopAddrsCoarse   uint16 = 45
	GenKeynum                 uint16 = 46
	GenVelocity               uint16 = 47
	GenInitialAttenuation     uint16 = 48
	GenEndloopAddrsCoarse     uint16 = 50
	GenCoarseTune             uint16 = 51
	GenFineTune               uint16 = 52
	GenSampleID               uint16 = 53
	GenSampleModes            uint16 = 54
	GenScaleTuning            uint16 = 56
	GenExclusiveClass         uint16 = 57
	GenOverridingRootKey      uint16 = 58
	GenEndOper                uint16 = 60
)

// generatorNames maps known opcodes to their spec names. Opcodes absent
// from this table are unknown generators and are preserved verbatim.
var generatorNames = map[uint16]string{
	GenStartAddrsOffset:       "startAddrsOffset",
	GenEndAddrsOffset:         "endAddrsOffset",
	GenStartloopAddrsOffset:   "startloopAddrsOffset",
	GenEndloopAddrsOffset:     "endloopAddrsOffset",
	GenStartAddrsCoarseOffset: "startAddrsCoarseOffset",
	GenModLFOtoPitch:          "modLfoToPitch",
	GenVibLFOtoPitch:          "vibLfoToPitch",
	GenModEnvtoPitch:          "modEnvToPitch",
	GenInitialFilterFc:        "initialFilterFc",
	GenInitialFilterQ:         "initialFilterQ",
	GenModLFOtoFilterFc:       "modLfoToFilterFc",
	GenModEnvtoFilterFc:       "modEnvToFilterFc",
	GenEndAddrsCoarseOffset:   "endAddrsCoarseOffset",
	GenModLFOtoVolume:         "modLfoToVolume",
	GenChorusEffectsSend:      "chorusEffectsSend",
	GenReverbEffectsSend:      "reverbEffectsSend",
	GenPan:                    "pan",
	GenDelayModLFO:            "delayModLFO",
	GenFreqModLFO:             "freqModLFO",
	GenDelayVibLFO:            "delayVibLFO",
	GenFreqVibLFO:             "freqVibLFO",
	GenDelayModEnv:            "delayModEnv",
	GenAttackModEnv:           "attackModEnv",
	GenHoldModEnv:             "holdModEnv",
	GenDecayModEnv:            "decayModEnv",
	GenSustainModEnv:          "sustainModEnv",
	GenReleaseModEnv:          "releaseModEnv",
	GenKeynumToModEnvHold:     "keynumToModEnvHold",
	GenKeynumToModEnvDecay:    "keynumToModEnvDecay",
	GenDelayVolEnv:            "delayVolEnv",
	GenAttackVolEnv:           "attackVolEnv",
	GenHoldVolEnv:             "holdVolEnv",
	GenDecayVolEnv:            "decayVolEnv",
	GenSustainVolEnv:          "sustainVolEnv",
	GenReleaseVolEnv:          "releaseVolEnv",
	GenKeynumToVolEnvHold:     "keynumToVolEnvHold",
	GenKeynumToVolEnvDecay:    "keynumToVolEnvDecay",
	GenInstrument:             "instrument",
	GenKeyRange:               "keyRange",
	GenVelRange:               "velRange",
	GenStartloopAddrsCoarse:   "startloopAddrsCoarseOffset",
	GenKeynum:                 "keynum",
	GenVelocity:               "velocity",
	GenInitialAttenuation:     "initialAttenuation",
	GenEndloopAddrsCoarse:     "endloopAddrsCoarseOffset",
	GenCoarseTune:             "coarseTune",
	GenFineTune:               "fineTune",
	GenSampleID:               "sampleID",
	GenSampleModes:            "sampleModes",
	GenScaleTuning:            "scaleTuning",
	GenExclusiveClass:         "exclusiveClass",
	GenOverridingRootKey:      "overridingRootKey",
	GenEndOper:                "endOper",
}

// Generator is a single raw generator entry. Unknown opcodes are kept
// byte-for-byte so files round-trip without data loss.
type Generator struct {
	Op    uint16
	Value uint16
}

// Known reports whether the opcode is defined by the SF2 spec.
func (g Generator) Known() bool {
	_, ok := generatorNames[g.Op]
	return ok
}

// Name returns the spec name, or "unknown(<op>)" for unrecognized opcodes.
func (g Generator) Name() string {
	if n, ok := generatorNames[g.Op]; ok {
		return n
	}
	return "unknown"
}

// Amount interprets the value as a signed 16-bit quantity.
func (g Generator) Amount() int16 { return int16(g.Value) }

// Range interprets the value as a lo/hi byte pair (keyRange, velRange).
func (g Generator) Range() (lo, hi uint8) {
	return uint8(g.Value & 0xFF), uint8(g.Value >> 8)
}
