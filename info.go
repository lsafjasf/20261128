package wavinfo

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Entry is one INFO sub-chunk (for example IART with the artist text).
type Entry struct {
	// ID is the four-character field identifier; unknown identifiers are
	// preserved exactly as found in the file.
	ID string
	// Text is the field value with the spec-mandated trailing NUL terminator
	// removed. Use Payload to access the raw stored bytes.
	Text string

	// payload is the exact sub-chunk data as parsed (NUL included when the
	// writer supplied one). Untouched entries are re-emitted from these bytes,
	// so non-standard encodings or terminators round-trip losslessly.
	payload []byte
}

// Known reports whether the entry uses one of the standard INFO identifiers.
func (e Entry) Known() bool {
	return IsKnown(e.ID)
}

// Payload returns the raw sub-chunk bytes stored for the entry.
func (e Entry) Payload() []byte {
	return append([]byte(nil), e.payload...)
}

// InfoList is the ordered content of a RIFF "LIST" "INFO" chunk.
type InfoList struct {
	entries []Entry
}

// Entries returns a copy of the entries in their current (parse) order.
func (l *InfoList) Entries() []Entry {
	if l == nil {
		return nil
	}
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Get returns the text of the first entry with the given id.
func (l *InfoList) Get(id string) (string, bool) {
	if l == nil {
		return "", false
	}
	for _, e := range l.entries {
		if e.ID == id {
			return e.Text, true
		}
	}
	return "", false
}

// Set stores text under id, replacing any previous entries with that id. The
// value is serialized with a single trailing NUL terminator as required by the
// INFO convention. id must be exactly four bytes.
func (l *InfoList) Set(id, text string) error {
	if len(id) != 4 {
		return fmt.Errorf("wavinfo: INFO id %q must be 4 bytes", id)
	}
	payload := make([]byte, 0, len(text)+1)
	payload = append(payload, text...)
	payload = append(payload, 0)
	l.replace(id, payload, text)
	return nil
}

// SetRaw stores an arbitrary payload under id without NUL processing.
func (l *InfoList) SetRaw(id string, payload []byte) error {
	if len(id) != 4 {
		return fmt.Errorf("wavinfo: INFO id %q must be 4 bytes", id)
	}
	text := string(bytes.TrimRight(payload, "\x00"))
	l.replace(id, append([]byte(nil), payload...), text)
	return nil
}

// Delete removes every entry carrying id and reports whether one existed.
func (l *InfoList) Delete(id string) bool {
	if l == nil {
		return false
	}
	kept := make([]Entry, 0, len(l.entries))
	removed := false
	for _, e := range l.entries {
		if e.ID == id {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	l.entries = kept
	return removed
}

func (l *InfoList) replace(id string, payload []byte, text string) {
	for i := range l.entries {
		if l.entries[i].ID == id {
			l.entries[i].payload = payload
			l.entries[i].Text = text
			// Rebuild without duplicates using a fresh backing array so the
			// remaining entries keep their original relative order.
			kept := make([]Entry, 0, len(l.entries))
			kept = append(kept, l.entries[:i+1]...)
			for _, e := range l.entries[i+1:] {
				if e.ID != id {
					kept = append(kept, e)
				}
			}
			l.entries = kept
			return
		}
	}
	l.entries = append(l.entries, Entry{ID: id, Text: text, payload: payload})
}

// parseInfoList decodes the payload of a LIST chunk whose list type is INFO.
// data must include the leading four-byte "INFO" list type.
func parseInfoList(data []byte) (*InfoList, error) {
	listType, err := listType(data)
	if err != nil {
		return nil, err
	}
	if listType != "INFO" {
		return nil, fmt.Errorf("wavinfo: not an INFO list (got %q)", listType)
	}
	list := &InfoList{}
	pos := 4
	for pos < len(data) {
		if pos+8 > len(data) {
			return nil, fmt.Errorf("%w: INFO entry header at offset %d", ErrTruncated, pos)
		}
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		end := start + size
		if end > len(data) {
			return nil, fmt.Errorf("%w: INFO entry %q at offset %d", ErrTruncated, id, pos)
		}
		payload := append([]byte(nil), data[start:end]...)
		list.entries = append(list.entries, Entry{
			ID:      id,
			Text:    string(trimOneNUL(payload)),
			payload: payload,
		})
		// RIFF sub-chunk headers are word-aligned: round the payload end up to
		// the next even offset to skip any pad byte.
		pos = end + end%2
	}
	return list, nil
}

func trimOneNUL(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == 0 {
		return b[:len(b)-1]
	}
	return b
}

// encode rebuilds the LIST chunk payload: the INFO type followed by every
// entry in authoritative order. Known fields follow canonicalOrder; unknown
// fields keep their original relative order and trail the known ones.
// Duplicate ids retain their relative first-seen ordering.
func (l *InfoList) encode() []byte {
	var buf bytes.Buffer
	buf.WriteString("INFO")

	emitted := make([]bool, len(l.entries))
	emit := func(id string) {
		for i, e := range l.entries {
			if emitted[i] || e.ID != id {
				continue
			}
			emitted[i] = true
			writeChunk(&buf, e.ID, e.payload)
		}
	}

	for _, id := range canonicalOrder {
		emit(id)
	}

	// Unknown fields keep their original relative order: iterating the parsed
	// entries in place and skipping already-emitted known ones is sufficient.
	for i, e := range l.entries {
		if !emitted[i] {
			writeChunk(&buf, e.ID, e.payload)
		}
	}
	return buf.Bytes()
}
