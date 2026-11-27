package opusmap

import "fmt"

type SourceKind string

const (
	SourceSilence     SourceKind = "silence"
	SourceMono        SourceKind = "mono"
	SourceStereoLeft  SourceKind = "stereo_left"
	SourceStereoRight SourceKind = "stereo_right"
	SourceInvalid     SourceKind = "invalid"
)

// Source identifies the decoded channel feeding an output channel.
type Source struct {
	Kind         SourceKind `json:"kind"`
	Stream       int        `json:"stream"`
	DecodedIndex int        `json:"decoded_index"`
	Coupled      bool       `json:"coupled"`
}

// PhysicalChannel is one output channel and its family-specific meaning.
type PhysicalChannel struct {
	Channel      int    `json:"channel"`
	MappingIndex uint8  `json:"mapping_index"`
	Name         string `json:"name"`
	ACN          int    `json:"acn"`
	Source       Source `json:"source"`
}

// StreamMapping describes whether a coded stream was declared coupled by M.
type StreamMapping struct {
	Stream          int   `json:"stream"`
	Coupled         bool  `json:"coupled"`
	DecodedChannels int   `json:"decoded_channels"`
	UsedBy          []int `json:"used_by"`
}

// Mapping is the decoded view of OpusHead's channel mapping table.
type Mapping struct {
	Family            uint8             `json:"family"`
	StreamCount       int               `json:"stream_count"`
	CoupledCount      int               `json:"coupled_count"`
	DecodedChannels   int               `json:"decoded_channels"`
	AmbisonicOrder    int               `json:"ambisonic_order"`
	NonDiegeticStereo bool              `json:"non_diegetic_stereo"`
	Streams           []StreamMapping   `json:"streams"`
	Channels          []PhysicalChannel `json:"channels"`
}

// ReconstructMapping expands the channel mapping table into decoded-stream
// sources and assigns the family 2 ACN / non-diegetic names.
func ReconstructMapping(h *OpusHead) (Mapping, []Issue) {
	result := Mapping{}
	if h == nil {
		return result, []Issue{newIssue(
			CodeIllegalMapping, 0, 0, -1, -1, "nil OpusHead",
		)}
	}
	if h.ChannelCount < 0 || h.StreamCount < 0 || h.CoupledCount < 0 {
		return result, []Issue{newIssue(
			CodeIllegalMapping, 0, 0, -1, -1,
			"negative channel or stream count",
		)}
	}
	result.Family = h.MappingFamily
	result.StreamCount = h.StreamCount
	result.CoupledCount = h.CoupledCount
	result.DecodedChannels = h.StreamCount + h.CoupledCount
	issues := h.Validate()

	result.Streams = make([]StreamMapping, h.StreamCount)
	for i := range result.Streams {
		coupled := i < h.CoupledCount
		channels := 1
		if coupled {
			channels = 2
		}
		result.Streams[i] = StreamMapping{
			Stream: i, Coupled: coupled, DecodedChannels: channels,
		}
	}

	if h.MappingFamily == 2 {
		result.AmbisonicOrder, result.NonDiegeticStereo = ambisonicLayout(h.ChannelCount)
	}
	result.Channels = make([]PhysicalChannel, 0, h.ChannelCount)
	for i := 0; i < h.ChannelCount; i++ {
		channel := PhysicalChannel{
			Channel: i,
			Name:    channelName(h, i),
			ACN:     -1,
		}
		if h.MappingFamily == 2 {
			channel.ACN = ambisonicACN(h.ChannelCount, i)
		}
		if i >= len(h.Mapping) {
			channel.MappingIndex = 255
			channel.Source = Source{Kind: SourceSilence, Stream: -1, DecodedIndex: -1}
			result.Channels = append(result.Channels, channel)
			continue
		}

		index := h.Mapping[i]
		channel.MappingIndex = index
		source, ok := decodeMappingIndex(index, h.StreamCount, h.CoupledCount)
		channel.Source = source
		if ok && source.Kind != SourceSilence {
			result.Streams[source.Stream].UsedBy = append(
				result.Streams[source.Stream].UsedBy, i,
			)
		}
		result.Channels = append(result.Channels, channel)
	}
	return result, issues
}

func decodeMappingIndex(index uint8, streams, coupled int) (Source, bool) {
	if index == 255 {
		return Source{Kind: SourceSilence, Stream: -1, DecodedIndex: -1}, true
	}
	decoded := streams + coupled
	if int(index) >= decoded || streams <= 0 {
		return Source{Kind: SourceInvalid, Stream: -1, DecodedIndex: int(index)}, false
	}
	if int(index) < 2*coupled {
		stream := int(index) / 2
		if stream < 0 || stream >= streams {
			return Source{Kind: SourceInvalid, Stream: -1, DecodedIndex: int(index)}, false
		}
		kind := SourceStereoLeft
		if index&1 != 0 {
			kind = SourceStereoRight
		}
		return Source{
			Kind: kind, Stream: stream, DecodedIndex: int(index), Coupled: true,
		}, true
	}
	stream := int(index) - coupled
	if stream < 0 || stream >= streams {
		return Source{Kind: SourceInvalid, Stream: -1, DecodedIndex: int(index)}, false
	}
	return Source{
		Kind:         SourceMono,
		Stream:       stream,
		DecodedIndex: int(index),
		Coupled:      false,
	}, true
}

func ambisonicLayout(channels int) (order int, nonDiegetic bool) {
	if channels < 1 {
		return -1, false
	}
	for n := 0; n <= 14; n++ {
		full := (n + 1) * (n + 1)
		if channels == full {
			return n, false
		}
		if channels == full+2 {
			return n, true
		}
	}
	return -1, false
}

func ambisonicACN(channels, channel int) int {
	if channel < 0 || channels < 1 {
		return -1
	}
	_, nonDiegetic := ambisonicLayout(channels)
	if nonDiegetic && channel >= channels-2 {
		return -1
	}
	return channel
}

func channelName(h *OpusHead, channel int) string {
	switch h.MappingFamily {
	case 0:
		if h.ChannelCount == 1 {
			return "mono"
		}
		if channel == 0 {
			return "left"
		}
		if channel == 1 {
			return "right"
		}
	case 1:
		names := speakerNames(h.ChannelCount)
		if channel < len(names) {
			return names[channel]
		}
	case 2:
		_, nonDiegetic := ambisonicLayout(h.ChannelCount)
		if nonDiegetic && channel >= h.ChannelCount-2 {
			if channel == h.ChannelCount-2 {
				return "non-diegetic left"
			}
			return "non-diegetic right"
		}
		if ambisonicLayoutValid(h.ChannelCount) {
			order := ambisonicOrderOf(channel)
			degree := channel - order*(order+1)
			return fmt.Sprintf("ACN%d (order %d, degree %d)", channel, order, degree)
		}
	}
	return fmt.Sprintf("channel %d", channel)
}

func ambisonicLayoutValid(channels int) bool {
	order, _ := ambisonicLayout(channels)
	return order >= 0
}

func ambisonicOrderOf(acn int) int {
	n := 0
	for (n+1)*(n+1) <= acn {
		n++
	}
	return n
}

func speakerNames(channels int) []string {
	byChannel := map[int][]string{
		1: {"mono"},
		2: {"left", "right"},
		3: {"front left", "front center", "front right"},
		4: {"front left", "front right", "rear left", "rear right"},
		5: {"front left", "front center", "front right", "rear left", "rear right"},
		6: {"front left", "front center", "front right", "rear left", "rear right", "LFE"},
		7: {"front left", "front center", "front right", "side left", "side right", "rear center", "LFE"},
		8: {"front left", "front center", "front right", "side left", "side right", "rear left", "rear right", "LFE"},
	}
	return byChannel[channels]
}
