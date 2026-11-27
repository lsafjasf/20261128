package cea608

import "fmt"

// EventType identifies the result of one decoded byte pair.
type EventType uint8

const (
	// EventText contains one or two newly decoded caption characters.
	EventText EventType = iota + 1
	// EventControl identifies a valid control or positioning code.
	EventControl
	// EventCaption is emitted when EOC exposes a completed caption.
	EventCaption
	// EventError reports bytes that were rejected instead of displayed.
	EventError
)

// Event is the decoded representation of a CEA-608 byte pair.
type Event struct {
	Type    EventType
	Text    string
	Control *Control
	Caption *Caption
	Error   *DecodeError
}

// Control describes a non-text pair such as RCL, EOC, or a PAC.
type Control struct {
	First   byte
	Second  byte
	Channel int
	Code    ControlCode
	Name    string
}

// ControlCode is a stable identifier for a decoded control pair.
type ControlCode uint16

// Caption is a snapshot of displayed caption memory.
type Caption struct {
	Text string
	Rows []string
}

// ErrorKind classifies a rejected pair or byte.
type ErrorKind uint8

const (
	// ErrorParity means at least one byte had even parity.
	ErrorParity ErrorKind = iota + 1
	// ErrorMalformedPair means parity was valid but the pair was not a legal
	// CEA-608 character, control, PAC, or padding sequence.
	ErrorMalformedPair
)

// DecodeError describes a byte pair that was not interpreted as caption text.
// ByteIndex is 0 for the first byte and 1 for the second. Offset is the
// zero-based position of the pair's first byte in the complete stream passed
// to the decoder.
type DecodeError struct {
	Kind      ErrorKind
	Pair      [2]byte
	ByteIndex int
	Offset    int64
}

func (e *DecodeError) Error() string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case ErrorParity:
		return fmt.Sprintf("cea608: parity error in pair %02X %02X at offset %d",
			e.Pair[0], e.Pair[1], e.Offset)
	case ErrorMalformedPair:
		return fmt.Sprintf("cea608: malformed pair %02X %02X at offset %d",
			e.Pair[0], e.Pair[1], e.Offset)
	default:
		return fmt.Sprintf("cea608: rejected pair %02X %02X at offset %d",
			e.Pair[0], e.Pair[1], e.Offset)
	}
}

// Stats is a cumulative decoding summary. ValidPairs counts accepted byte
// pairs, while DroppedPairs counts pairs that produced no text or control.
type Stats struct {
	Bytes        uint64
	Pairs        uint64
	ValidPairs   uint64
	ParityErrors uint64
	Malformed    uint64
	DroppedPairs uint64
	Captions     uint64
}
