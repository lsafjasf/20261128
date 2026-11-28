package wavinfo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrNotRIFF is returned when the input does not start with a RIFF header.
	ErrNotRIFF = errors.New("wavinfo: missing RIFF header")
	// ErrTruncated is returned when a chunk extends beyond the available data.
	ErrTruncated = errors.New("wavinfo: truncated RIFF data")
)

// Chunk is one RIFF sub-chunk: a four-character id followed by its payload.
type Chunk struct {
	ID   string
	Data []byte

	// raw holds the exact on-disk encoding (id, size, payload, padding byte)
	// of the chunk as it was parsed. It is cleared by MarkDirty so that edited
	// chunks are re-serialized, while every untouched chunk round-trips byte
	// for byte.
	raw []byte
}

// MarkDirty forces the chunk to be re-serialized on the next write instead of
// being copied from its original on-disk bytes.
func (c *Chunk) MarkDirty() {
	c.raw = nil
}

// RIFFFile is a parsed RIFF container (for WAV files, FormType == "WAVE").
type RIFFFile struct {
	FormType string
	chunks   []Chunk
	trailing []byte
}

// Chunks returns the container's sub-chunks in file order. The returned slice
// shares the chunk structs with the file; callers must not reorder or delete
// entries unless they also call Write themselves.
func (f *RIFFFile) Chunks() []Chunk {
	return f.chunks
}

const riffHeaderSize = 12 // "RIFF" + size(4) + form type(4)

// ParseRIFF reads and validates a complete RIFF stream.
func ParseRIFF(r io.Reader) (*RIFFFile, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("wavinfo: read riff: %w", err)
	}
	if len(buf) < riffHeaderSize {
		return nil, ErrNotRIFF
	}
	if string(buf[0:4]) != "RIFF" {
		return nil, ErrNotRIFF
	}
	declared := int(binary.LittleEndian.Uint32(buf[4:8]))
	// The size field counts the form type plus every sub-chunk, so the body
	// ends eight bytes (magic + size) after the declared length.
	bodyEnd := 8 + declared
	if bodyEnd > len(buf) {
		return nil, fmt.Errorf("%w: riff size %d exceeds %d data bytes",
			ErrTruncated, declared, len(buf)-8)
	}

	file := &RIFFFile{
		FormType: string(buf[8:12]),
		trailing: append([]byte(nil), buf[bodyEnd:]...),
	}

	pos := 12
	for pos < bodyEnd {
		if pos+8 > bodyEnd {
			return nil, fmt.Errorf("%w: chunk header at offset %d", ErrTruncated, pos)
		}
		id := string(buf[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(buf[pos+4 : pos+8]))
		dataStart := pos + 8
		dataEnd := dataStart + size
		if dataEnd > bodyEnd {
			return nil, fmt.Errorf("%w: chunk %q at offset %d", ErrTruncated, id, pos)
		}
		// RIFF aligns every sub-chunk to a word boundary relative to the file
		// start, so round the payload end up to the next even offset.
		paddedEnd := dataEnd + (dataEnd & 1)
		if paddedEnd > bodyEnd {
			return nil, fmt.Errorf("%w: pad byte of chunk %q at offset %d", ErrTruncated, id, pos)
		}
		file.chunks = append(file.chunks, Chunk{
			ID:   id,
			Data: append([]byte(nil), buf[dataStart:dataEnd]...),
			raw:  append([]byte(nil), buf[pos:paddedEnd]...),
		})
		pos = paddedEnd
	}
	return file, nil
}

// WriteTo serializes the container. Untouched chunks keep their exact original
// bytes; dirty chunks and any chunk whose raw cache is missing are encoded
// again from ID/Data with a zero padding byte when the payload is odd-sized.
func (f *RIFFFile) WriteTo(w io.Writer) (int64, error) {
	var body bytes.Buffer
	body.WriteString(f.FormType)
	for i := range f.chunks {
		chunk := &f.chunks[i]
		if len(chunk.raw) > 0 {
			body.Write(chunk.raw)
			continue
		}
		if err := writeChunk(&body, chunk.ID, chunk.Data); err != nil {
			return 0, err
		}
	}

	var out bytes.Buffer
	out.WriteString("RIFF")
	var sizeBytes [4]byte
	binary.LittleEndian.PutUint32(sizeBytes[:], uint32(body.Len()))
	out.Write(sizeBytes[:])
	out.Write(body.Bytes())
	out.Write(f.trailing)

	n, err := w.Write(out.Bytes())
	return int64(n), err
}

// writeChunk emits id, a little-endian size, the payload and a zero pad byte
// for odd payloads.
func writeChunk(buf *bytes.Buffer, id string, data []byte) error {
	if len(id) != 4 {
		return fmt.Errorf("wavinfo: invalid chunk id %q (must be 4 bytes)", id)
	}
	buf.WriteString(id)
	var sizeBytes [4]byte
	binary.LittleEndian.PutUint32(sizeBytes[:], uint32(len(data)))
	buf.Write(sizeBytes[:])
	buf.Write(data)
	if len(data)%2 == 1 {
		buf.WriteByte(0)
	}
	return nil
}

// listType returns the four-byte list type (e.g. "INFO") of a LIST chunk.
func listType(data []byte) (string, error) {
	if len(data) < 4 {
		return "", fmt.Errorf("wavinfo: LIST chunk too short for list type")
	}
	return string(data[0:4]), nil
}
