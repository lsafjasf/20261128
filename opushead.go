package opusmap

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

const opusHeadMinSize = 19

// OpusHead is the RFC 7845 identification header. For family 0, StreamCount
// and CoupledCount are filled from the implicit mono/stereo mapping.
type OpusHead struct {
	Version         uint8        `json:"version"`
	ChannelCount    int          `json:"channel_count"`
	PreSkip         uint16       `json:"pre_skip"`
	InputSampleRate uint32       `json:"input_sample_rate"`
	OutputGain      int16        `json:"output_gain"`
	MappingFamily   uint8        `json:"mapping_family"`
	StreamCount     int          `json:"stream_count"`
	CoupledCount    int          `json:"coupled_count"`
	Mapping         MappingTable `json:"mapping"`
}

// MappingTable is kept as bytes internally but serializes as a JSON array.
type MappingTable []uint8

func (m MappingTable) MarshalJSON() ([]byte, error) {
	values := make([]int, len(m))
	for i, value := range m {
		values[i] = int(value)
	}
	return json.Marshal(values)
}

// ParsedOpusHead contains the syntax-level header result. Syntax errors are
// reported in Issues with packet 0. Semantic mapping errors are also attached
// there so callers can print all header defects at once.
type ParsedOpusHead struct {
	Head   *OpusHead
	Issues []Issue
}

// DecodeOpusHead parses the identification header and validates its channel
// mapping. It never uses a decoder, so only framing and mapping semantics are
// checked.
func DecodeOpusHead(packet []byte) ParsedOpusHead {
	result := ParsedOpusHead{}
	if len(packet) < opusHeadMinSize {
		result.Issues = append(result.Issues, newIssue(
			CodeBadOpusHead, 0, 0, -1, -1,
			"OpusHead is %d bytes, need at least %d", len(packet), opusHeadMinSize,
		))
		return result
	}
	if string(packet[:8]) != "OpusHead" {
		result.Issues = append(result.Issues, newIssue(
			CodeBadOpusHead, 0, 0, -1, -1, "missing OpusHead magic",
		))
		return result
	}

	h := &OpusHead{
		Version:         packet[8],
		ChannelCount:    int(packet[9]),
		PreSkip:         binary.LittleEndian.Uint16(packet[10:12]),
		InputSampleRate: binary.LittleEndian.Uint32(packet[12:16]),
		OutputGain:      int16(binary.LittleEndian.Uint16(packet[16:18])),
		MappingFamily:   packet[18],
	}
	if h.Version >= 16 {
		result.Issues = append(result.Issues, newIssue(
			CodeBadOpusHead, 0, 0, -1, -1, "unsupported OpusHead version %d", h.Version,
		))
	}
	if h.ChannelCount == 0 {
		result.Issues = append(result.Issues, newIssue(
			CodeBadOpusHead, 0, 0, -1, -1, "channel count is zero",
		))
	}

	switch h.MappingFamily {
	case 0:
		h.StreamCount = 1
		h.CoupledCount = h.ChannelCount - 1
		h.Mapping = make(MappingTable, h.ChannelCount)
		for i := range h.Mapping {
			h.Mapping[i] = uint8(i)
		}
	case 1, 2:
		if len(packet) < opusHeadMinSize+2+h.ChannelCount {
			result.Issues = append(result.Issues, newIssue(
				CodeBadOpusHead, 0, 0, -1, -1,
				"mapping family %d needs %d bytes, header has %d",
				h.MappingFamily, opusHeadMinSize+2+h.ChannelCount, len(packet),
			))
			result.Head = h
			return result
		}
		h.StreamCount = int(packet[19])
		h.CoupledCount = int(packet[20])
		h.Mapping = append(MappingTable(nil), packet[21:21+h.ChannelCount]...)
	default:
		result.Issues = append(result.Issues, newIssue(
			CodeBadOpusHead, 0, 0, -1, -1,
			"unsupported mapping family %d (supported: 0, 1, 2)", h.MappingFamily,
		))
	}

	result.Head = h
	result.Issues = append(result.Issues, h.Validate()...)
	return result
}

// Validate checks stream counts and mapping entries. All returned issues use
// packet 0 because the mapping table belongs to OpusHead.
func (h *OpusHead) Validate() []Issue {
	if h == nil {
		return []Issue{newIssue(CodeBadOpusHead, 0, 0, -1, -1, "nil OpusHead")}
	}
	var issues []Issue
	switch h.MappingFamily {
	case 0:
		if h.ChannelCount != 1 && h.ChannelCount != 2 {
			issues = append(issues, newIssue(
				CodeIllegalMapping, 0, 0, -1, -1,
				"family 0 allows 1 or 2 channels, got %d", h.ChannelCount,
			))
		}
		return issues
	case 1:
		if h.ChannelCount < 1 || h.ChannelCount > 8 {
			issues = append(issues, newIssue(
				CodeIllegalMapping, 0, 0, -1, -1,
				"family 1 allows 1...8 channels, got %d", h.ChannelCount,
			))
		}
	case 2:
		if !validAmbisonicChannelCount(h.ChannelCount) {
			issues = append(issues, newIssue(
				CodeIllegalMapping, 0, 0, -1, -1,
				"family 2 channel count %d is not (1+order)^2 or (1+order)^2+2",
				h.ChannelCount,
			))
		}
	default:
		return issues
	}

	if h.StreamCount == 0 {
		issues = append(issues, newIssue(
			CodeIllegalMapping, 0, 0, -1, -1, "stream count is zero",
		))
	} else if h.CoupledCount < 0 || h.CoupledCount > h.StreamCount {
		issues = append(issues, newIssue(
			CodeIllegalMapping, 0, 0, -1, -1,
			"coupled stream count %d exceeds stream count %d",
			h.CoupledCount, h.StreamCount,
		))
	}
	decoded := h.StreamCount + h.CoupledCount
	if decoded > 255 {
		issues = append(issues, newIssue(
			CodeIllegalMapping, 0, 0, -1, -1,
			"decoded channel count N+M is %d, maximum is 255", decoded,
		))
	}
	for channel, index := range h.Mapping {
		if index != 255 && int(index) >= decoded {
			issues = append(issues, newIssue(
				CodeIllegalMapping, 0, 0, -1, channel,
				"mapping index %d is outside decoded channels 0...%d (N=%d, M=%d)",
				index, decoded-1, h.StreamCount, h.CoupledCount,
			))
		}
	}
	return issues
}

func validAmbisonicChannelCount(channels int) bool {
	for order := 0; order <= 14; order++ {
		full := (order + 1) * (order + 1)
		if channels == full || channels == full+2 {
			return true
		}
	}
	return false
}

// ParseOpusHead is a convenience wrapper that rejects any header issue.
func ParseOpusHead(packet []byte) (*OpusHead, error) {
	parsed := DecodeOpusHead(packet)
	if len(parsed.Issues) != 0 {
		return parsed.Head, errors.Join(issuesToErrors(parsed.Issues)...)
	}
	return parsed.Head, nil
}

func issuesToErrors(issues []Issue) []error {
	out := make([]error, len(issues))
	for i := range issues {
		out[i] = issues[i]
	}
	return out
}

func (h OpusHead) String() string {
	return fmt.Sprintf(
		"OpusHead v%d channels=%d pre-skip=%d rate=%d gain=%d family=%d N=%d M=%d mapping=%v",
		h.Version, h.ChannelCount, h.PreSkip, h.InputSampleRate, h.OutputGain,
		h.MappingFamily, h.StreamCount, h.CoupledCount, h.Mapping,
	)
}
