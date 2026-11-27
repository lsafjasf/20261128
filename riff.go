package sf2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Chunk is a single RIFF chunk. For LIST/RIFF chunks, ListType holds the
// four-character form type and Children the nested chunks; for leaf chunks,
// Data holds the raw payload.
type Chunk struct {
	ID       string
	ListType string
	Data     []byte
	Children []*Chunk
}

var (
	errNotRIFF   = errors.New("sf2: not a RIFF file")
	errTruncated = errors.New("sf2: truncated chunk")
)

// ParseRIFF reads a RIFF container and returns its top-level chunks.
func ParseRIFF(r io.ReaderAt, size int64) (*Chunk, error) {
	root, _, err := readChunk(r, 0, size)
	if err != nil {
		return nil, err
	}
	if root.ID != "RIFF" && root.ID != "LIST" {
		return nil, errNotRIFF
	}
	return root, nil
}

func readChunk(r io.ReaderAt, off, limit int64) (*Chunk, int64, error) {
	var hdr [8]byte
	if _, err := r.ReadAt(hdr[:], off); err != nil {
		return nil, 0, errTruncated
	}
	id := string(hdr[:4])
	length := int64(binary.LittleEndian.Uint32(hdr[4:]))
	dataOff := off + 8
	if dataOff+length > limit {
		return nil, 0, fmt.Errorf("sf2: chunk %q overruns parent", id)
	}
	c := &Chunk{ID: id}
	next := dataOff + length
	if length%2 == 1 {
		next++ // RIFF chunks are word-aligned
	}
	if id == "RIFF" || id == "LIST" {
		var typ [4]byte
		if _, err := r.ReadAt(typ[:], dataOff); err != nil {
			return nil, 0, errTruncated
		}
		c.ListType = string(typ[:])
		childOff := dataOff + 4
		end := dataOff + length
		for childOff < end {
			child, n, err := readChunk(r, childOff, end)
			if err != nil {
				return nil, 0, err
			}
			c.Children = append(c.Children, child)
			childOff = n
		}
	} else {
		c.Data = make([]byte, length)
		if _, err := r.ReadAt(c.Data, dataOff); err != nil {
			return nil, 0, errTruncated
		}
	}
	return c, next, nil
}

// Find returns the first direct child with the given chunk ID, or nil.
func (c *Chunk) Find(id string) *Chunk {
	for _, ch := range c.Children {
		if ch.ID == id {
			return ch
		}
	}
	return nil
}

// FindList returns the first direct LIST child with the given form type.
func (c *Chunk) FindList(listType string) *Chunk {
	for _, ch := range c.Children {
		if ch.ID == "LIST" && ch.ListType == listType {
			return ch
		}
	}
	return nil
}
