package lfcheck

import (
	"fmt"
	"math"
)

const schemaVersion = 1

// Options controls the low-frequency audit. Zero values select cinema LFE
// defaults: 20-120 Hz and a 50 ms lag search.
type Options struct {
	LowHz    float64
	HighHz   float64
	MaxLagMS float64
	Layout   string
}

func (o Options) defaults() (Options, error) {
	if o.LowHz == 0 {
		o.LowHz = 20
	}
	if o.HighHz == 0 {
		o.HighHz = 120
	}
	if o.MaxLagMS == 0 {
		o.MaxLagMS = 50
	}
	if o.LowHz < 0 || o.HighHz <= o.LowHz || o.MaxLagMS <= 0 {
		return o, fmt.Errorf("invalid analysis band or lag search")
	}
	return o, nil
}

// ChannelResult reports level and activity for one audited channel.
type ChannelResult struct {
	Role       string  `json:"role"`
	Index      int     `json:"index"`
	Label      string  `json:"label"`
	RMSDBFS    float64 `json:"rms_dbfs"`
	LowRMSDBFS float64 `json:"low_rms_dbfs"`
	// LowRelativeToMainDB is relative to the loudest main channel. For a main
	// channel this is its own low-band level relative to that maximum.
	LowRelativeToMainDB float64 `json:"low_relative_to_main_db"`
	Active              bool    `json:"active"`
}

// Finding is a stable machine-readable diagnosis plus its supporting values.
type Finding struct {
	Code        string   `json:"code"`
	Severity    string   `json:"severity"`
	Channels    []string `json:"channels,omitempty"`
	Reference   []string `json:"reference,omitempty"`
	Pair        string   `json:"pair,omitempty"`
	Correlation float64  `json:"correlation,omitempty"`
	LagSamples  int      `json:"lag_samples,omitempty"`
	LagMS       float64  `json:"lag_ms,omitempty"`
	GainDB      float64  `json:"gain_db,omitempty"`
	Message     string   `json:"message"`
}

// Analysis is the complete result for one WAV program.
type Analysis struct {
	SchemaVersion int             `json:"schema_version"`
	Source        string          `json:"source"`
	SampleRate    int             `json:"sample_rate"`
	Channels      int             `json:"channels"`
	Bits          int             `json:"bits_per_sample"`
	Codec         string          `json:"codec"`
	Duration      float64         `json:"duration_seconds"`
	Layout        []string        `json:"channel_layout"`
	Roles         Roles           `json:"roles"`
	LowHz         float64         `json:"low_hz"`
	HighHz        float64         `json:"high_hz"`
	ChannelInfo   []ChannelResult `json:"channel_levels"`
	Pairs         []PairResult    `json:"pairs"`
	Findings      []Finding       `json:"findings"`
	Assessment    string          `json:"assessment"`
	Warnings      []string        `json:"warnings,omitempty"`
}

// AnalyzeFile reads and audits one WAV file.
func AnalyzeFile(path string, opts Options) (Analysis, error) {
	audio, err := ReadWAV(path)
	if err != nil {
		return Analysis{}, err
	}
	return AnalyzeAudio(audio, opts)
}

// AnalyzeAudio audits the LFE, L, R and C low-frequency signals.
func AnalyzeAudio(audio Audio, opts Options) (Analysis, error) {
	opts, err := opts.defaults()
	if err != nil {
		return Analysis{}, err
	}
	layout, err := ChannelNames(audio)
	if err != nil {
		return Analysis{}, err
	}
	if opts.Layout != "" {
		layout, err = ParseLayout(opts.Layout)
		if err != nil {
			return Analysis{}, err
		}
		if len(layout) != audio.Channels {
			return Analysis{}, fmt.Errorf("layout has %d channels, file has %d",
				len(layout), audio.Channels)
		}
	}
	roles, err := RolesFromLayout(layout)
	if err != nil {
		return Analysis{}, err
	}

	result := Analysis{
		SchemaVersion: schemaVersion,
		Source:        audio.Source,
		SampleRate:    audio.SampleRate,
		Channels:      audio.Channels,
		Bits:          audio.Bits,
		Codec:         audio.Codec,
		Duration:      audio.Duration(),
		Layout:        layout,
		Roles:         roles,
		LowHz:         opts.LowHz,
		HighHz:        opts.HighHz,
		Findings:      []Finding{},
	}
	rolesList := []struct {
		name string
		idx  int
	}{
		{"L", roles.Left},
		{"R", roles.Right},
		{"C", roles.Center},
		{"LFE", roles.LFE},
	}
	bands := make(map[string][]float64, len(rolesList))
	levels := make(map[string]float64, len(rolesList))
	mainMax := -180.0
	for _, role := range rolesList {
		band := lowBand(audio.Samples[role.idx], audio.SampleRate, opts.LowHz, opts.HighHz)
		bands[role.name] = band
		level := dbfs(rms(band))
		levels[role.name] = level
		if role.name != "LFE" && level > mainMax {
			mainMax = level
		}
	}
	activeFloor := math.Max(-80, mainMax-50)
	for _, role := range rolesList {
		level := levels[role.name]
		relative := level - mainMax
		if math.Abs(relative) < 0.005 {
			relative = 0
		}
		result.ChannelInfo = append(result.ChannelInfo, ChannelResult{
			Role:                role.name,
			Index:               role.idx,
			Label:               layout[role.idx],
			RMSDBFS:             dbfs(rms(audio.Samples[role.idx])),
			LowRMSDBFS:          level,
			LowRelativeToMainDB: relative,
			Active:              level >= activeFloor,
		})
	}

	for i := 0; i < len(rolesList); i++ {
		for j := i + 1; j < len(rolesList); j++ {
			pair, err := comparePair(bands[rolesList[i].name], bands[rolesList[j].name],
				audio.SampleRate, opts.MaxLagMS/1000)
			if err != nil {
				return Analysis{}, fmt.Errorf("compare %s/%s: %w",
					rolesList[i].name, rolesList[j].name, err)
			}
			pair.A = rolesList[i].name
			pair.B = rolesList[j].name
			if math.Abs(pair.ZeroLag) < 0.0005 {
				pair.ZeroLag = 0
			}
			if math.Abs(pair.Best) < 0.0005 {
				pair.Best = 0
			}
			result.Pairs = append(result.Pairs, pair)
		}
	}

	classify(&result)
	if result.Assessment == "" {
		result.Assessment = defaultAssessment(result)
	}
	if audio.Source != "" {
		result.Source = audio.Source
	} else {
		result.Source = "memory"
	}
	result.Warnings = append(result.Warnings, periodicWarnings(result)...)
	return result, nil
}

func findPair(result *Analysis, a, b string) (PairResult, bool) {
	for _, pair := range result.Pairs {
		if (pair.A == a && pair.B == b) || (pair.A == b && pair.B == a) {
			return pair, true
		}
	}
	return PairResult{}, false
}

// pairFromA normalizes a pair so the requested channel is A. This makes lag
// and gain describe the direction needed by the diagnosis.
func pairFromA(pair PairResult, a string) PairResult {
	if pair.A == a {
		return pair
	}
	pair.A, pair.B = pair.B, pair.A
	pair.LagSamples = -pair.LagSamples
	pair.LagMS = -pair.LagMS
	if pair.LagSamples == 0 {
		pair.LagMS = 0
	}
	pair.GainDB, pair.ReverseGainDB = pair.ReverseGainDB, pair.GainDB
	return pair
}

func defaultAssessment(result Analysis) string {
	if len(result.Findings) == 0 {
		return "No coherent LFE routing or polarity fault was found in the selected low band."
	}
	return "Low-frequency routing or polarity evidence was found; review the findings."
}
