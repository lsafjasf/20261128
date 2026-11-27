package amrwb

import (
	"bytes"
	"fmt"
	"io"
)

// StorageMagic is the RFC 4867 single-channel AMR-WB file magic.
const StorageMagic = "#!AMR-WB\n"

// ParseStorage reads an octet-aligned, single-channel AMR-WB storage stream.
// It returns a timeline containing exactly one 320-sample span per stored
// frame-block, including SPEECH_LOST and NO_DATA frames.
func ParseStorage(r io.Reader) (*Timeline, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil storage reader", ErrInvalidFrame)
	}

	magic := make([]byte, len(StorageMagic))
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("%w: incomplete AMR-WB storage header: %v", ErrInvalidFrame, err)
	}
	if string(magic) != StorageMagic {
		return nil, fmt.Errorf("%w: missing AMR-WB storage magic", ErrInvalidFrame)
	}

	timeline := NewTimeline()
	var index int64
	for {
		headerOctet := []byte{0}
		n, err := io.ReadFull(r, headerOctet)
		if err == io.EOF {
			return timeline, nil
		}
		if err != nil {
			if n == 0 {
				return nil, fmt.Errorf("%w: read frame %d header: %v", ErrInvalidFrame, index, err)
			}
			return nil, fmt.Errorf("%w: truncated frame %d header: %v", ErrInvalidFrame, index, err)
		}

		header := ParseHeader(headerOctet[0])
		if !header.Mode.Valid() {
			return nil, fmt.Errorf(
				"%w: frame %d has reserved mode %d",
				ErrInvalidFrame, index, uint8(header.Mode),
			)
		}

		size := header.FrameOctets()
		payload := make([]byte, size-1)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf(
				"%w: truncated %s frame %d: %v",
				ErrInvalidFrame, header.Mode, index, err,
			)
		}

		_, err = timeline.Add(Frame{
			Index:   index,
			Mode:    header.Mode,
			Size:    size,
			Quality: header.Quality,
		})
		if err != nil {
			return nil, fmt.Errorf("frame %d: %w", index, err)
		}
		index++
	}
}

// ParseStorageBytes is the []byte convenience form of ParseStorage.
func ParseStorageBytes(data []byte) (*Timeline, error) {
	return ParseStorage(bytes.NewReader(data))
}
