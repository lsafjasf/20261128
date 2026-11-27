package opusmap

import "fmt"

const (
	CodeBadOgg           = "bad_ogg"
	CodeBadOpusHead      = "bad_opus_head"
	CodeMissingStream    = "missing_stream"
	CodeIllegalMapping   = "illegal_mapping"
	CodeBadPacket        = "bad_opus_packet"
	CodeDurationMismatch = "duration_mismatch"
	CodeTruncatedPacket  = "truncated_packet"
)

// Issue is a recoverable stream violation. PacketSeq is the zero-based
// ordinal of the Ogg packet in the Opus logical stream. Header packet 0 is
// OpusHead, packet 1 is OpusTags, and the first audio packet is packet 2.
type Issue struct {
	Code      string `json:"code"`
	PacketSeq uint64 `json:"packet_seq"`
	PageSeq   uint32 `json:"page_seq"`
	Stream    int    `json:"stream"`
	Channel   int    `json:"channel"`
	Message   string `json:"message"`
}

func (i Issue) Error() string {
	where := fmt.Sprintf("packet %d (page %d)", i.PacketSeq, i.PageSeq)
	if i.Stream >= 0 {
		where += fmt.Sprintf(" stream %d", i.Stream)
	}
	if i.Channel >= 0 {
		where += fmt.Sprintf(" channel %d", i.Channel)
	}
	return where + ": " + i.Message
}

func newIssue(code string, seq uint64, page uint32, stream, channel int, format string, args ...any) Issue {
	return Issue{
		Code:      code,
		PacketSeq: seq,
		PageSeq:   page,
		Stream:    stream,
		Channel:   channel,
		Message:   fmt.Sprintf(format, args...),
	}
}
