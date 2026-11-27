package ptpsim

import "time"

type MessageType string

const (
	Announce  MessageType = "announce"
	Sync      MessageType = "sync"
	DelayReq  MessageType = "delay_req"
	DelayResp MessageType = "delay_resp"
)

type Message struct {
	At            time.Duration `json:"at_ns"`
	Type          MessageType   `json:"type"`
	Source        string        `json:"source"`
	Target        string        `json:"target"`
	Sequence      uint32        `json:"sequence"`
	Grandmaster   string        `json:"grandmaster,omitempty"`
	Priority1     uint8         `json:"priority1,omitempty"`
	OriginTime    time.Duration `json:"origin_ns,omitempty"`
	Correction    time.Duration `json:"correction_ns,omitempty"`
	PathAsymmetry time.Duration `json:"path_asymmetry_ns,omitempty"`
	ClockState    string        `json:"clock_state"`
}

type TimelineSegment struct {
	Start  time.Duration `json:"start_ns"`
	End    time.Duration `json:"end_ns"`
	State  string        `json:"state"`
	Detail string        `json:"detail"`
}

type RecorderMetrics struct {
	ID                    string        `json:"id"`
	ClockPPM              float64       `json:"clock_ppm"`
	WrittenSamples        int           `json:"written_samples"`
	HoldoverSamples       float64       `json:"holdover_samples"`
	ResamplerFrozenSteps  int           `json:"resampler_frozen_steps"`
	TimestampOffsetError  time.Duration `json:"timestamp_offset_error_ns"`
	TimestampErrorSamples float64       `json:"timestamp_error_samples"`
}

type BufferMetrics struct {
	Packets           int `json:"packets"`
	ArrivalInversions int `json:"arrival_inversions"`
	Drops             int `json:"drops"`
	Duplicates        int `json:"duplicates"`
	SamplesBefore     int `json:"samples_before_reassembly"`
	SamplesAfter      int `json:"samples_after_reassembly"`
	NetSampleDelta    int `json:"net_sample_delta"`
}

type CountMetrics struct {
	A        int `json:"a"`
	B        int `json:"b"`
	Delta    int `json:"delta_b_minus_a"`
	AbsDelta int `json:"abs_delta"`
}

type TimestampEvidence struct {
	Skew        time.Duration `json:"skew_ns"`
	SkewSamples float64       `json:"skew_samples"`
	CountEffect int           `json:"count_effect_samples"`
	Explanation string        `json:"explanation"`
}

type ResamplingEvidence struct {
	Enabled          bool   `json:"enabled"`
	FrozenDuringLoss bool   `json:"frozen_during_loss"`
	CountEffect      int    `json:"count_effect_samples"`
	Explanation      string `json:"explanation"`
}

type ReorderEvidence struct {
	Inversions     int    `json:"inversions"`
	NetSampleDelta int    `json:"net_sample_delta"`
	CountEffect    int    `json:"count_effect_samples"`
	Explanation    string `json:"explanation"`
}

type Counterfactual struct {
	Name       string             `json:"name"`
	Counts     CountMetrics       `json:"counts"`
	Timestamp  TimestampEvidence  `json:"timestamp"`
	Resampling ResamplingEvidence `json:"resampling"`
	Buffer     BufferMetrics      `json:"buffer"`
}

type Diagnosis struct {
	Verdict        string   `json:"verdict"`
	DominantCause  string   `json:"dominant_cause"`
	Confidence     string   `json:"confidence"`
	Contributions  []string `json:"contributions"`
	SeamlessSplice bool     `json:"seamless_splice_allowed"`
	SpliceGuidance string   `json:"splice_guidance"`
}

type Report struct {
	Config          Config              `json:"config"`
	Timeline        []TimelineSegment   `json:"timeline"`
	LossWindow      time.Duration       `json:"loss_window_ns"`
	Messages        map[MessageType]int `json:"message_counts"`
	Events          []Message           `json:"events"`
	Recorders       []RecorderMetrics   `json:"recorders"`
	Counts          CountMetrics        `json:"counts"`
	Timestamp       TimestampEvidence   `json:"timestamp"`
	Resampling      ResamplingEvidence  `json:"resampling"`
	Reorder         ReorderEvidence     `json:"reorder"`
	Buffer          BufferMetrics       `json:"buffer"`
	Counterfactuals []Counterfactual    `json:"counterfactuals"`
	Diagnosis       Diagnosis           `json:"diagnosis"`
}
