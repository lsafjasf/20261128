package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Box is a parsed MP4 box header. Payload is the raw box payload for leaf
// boxes; Children is populated for known container boxes.
type Box struct {
	Type     string
	Size     int64 // total size including header
	Header   int64 // header size in bytes (8 or 16)
	Payload  []byte
	Children []*Box
}

var containerTypes = map[string]bool{
	"moov": true, "trak": true, "mdia": true, "minf": true,
	"stbl": true, "edts": true, "dinf": true,
}

// ReadBoxes reads all top-level boxes from r until EOF, recursing into
// container boxes. Leaf payloads are kept in memory.
func ReadBoxes(r io.Reader) ([]*Box, error) {
	var boxes []*Box
	for {
		hdr := make([]byte, 8)
		if _, err := io.ReadFull(r, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return boxes, nil
			}
			return nil, err
		}
		size := int64(binary.BigEndian.Uint32(hdr[0:4]))
		typ := string(hdr[4:8])
		header := int64(8)
		if size == 1 {
			ext := make([]byte, 8)
			if _, err := io.ReadFull(r, ext); err != nil {
				return nil, fmt.Errorf("box %s: %w", typ, err)
			}
			size = int64(binary.BigEndian.Uint64(ext))
			header = 16
		} else if size == 0 {
			return nil, fmt.Errorf("box %s: size 0 (to-EOF) unsupported", typ)
		}
		if size < header {
			return nil, fmt.Errorf("box %s: invalid size %d", typ, size)
		}
		payload := make([]byte, size-header)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("box %s payload: %w", typ, err)
		}
		b := &Box{Type: typ, Size: size, Header: header}
		if containerTypes[typ] {
			children, err := ReadBoxes(bytesReader(payload))
			if err != nil {
				return nil, err
			}
			b.Children = children
		} else {
			b.Payload = payload
		}
		boxes = append(boxes, b)
	}
}

type byteReader struct {
	buf []byte
	off int
}

func bytesReader(b []byte) io.Reader { return &byteReader{buf: b} }

func (r *byteReader) Read(p []byte) (int, error) {
	if r.off >= len(r.buf) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

// Find returns the first direct child with the given type, or nil.
func (b *Box) Find(typ string) *Box {
	for _, c := range b.Children {
		if c.Type == typ {
			return c
		}
	}
	return nil
}

// Path walks nested containers, e.g. Path("mdia", "minf", "stbl").
func (b *Box) Path(types ...string) *Box {
	cur := b
	for _, t := range types {
		if cur == nil {
			return nil
		}
		cur = cur.Find(t)
	}
	return cur
}

// --- binary cursor for payload decoding ---

type cursor struct {
	buf []byte
	off int
}

func (c *cursor) u8() uint8 {
	v := c.buf[c.off]
	c.off++
	return v
}

func (c *cursor) u16() uint16 {
	v := binary.BigEndian.Uint16(c.buf[c.off:])
	c.off += 2
	return v
}

func (c *cursor) u24() uint32 {
	v := uint32(c.buf[c.off])<<16 | uint32(c.buf[c.off+1])<<8 | uint32(c.buf[c.off+2])
	c.off += 3
	return v
}

func (c *cursor) u32() uint32 {
	v := binary.BigEndian.Uint32(c.buf[c.off:])
	c.off += 4
	return v
}

func (c *cursor) i32() int32 { return int32(c.u32()) }

func (c *cursor) u64() uint64 {
	v := binary.BigEndian.Uint64(c.buf[c.off:])
	c.off += 8
	return v
}

func (c *cursor) i64() int64 { return int64(c.u64()) }

func (c *cursor) skip(n int) { c.off += n }

func (c *cursor) str(n int) string {
	s := string(c.buf[c.off : c.off+n])
	c.off += n
	return s
}

// --- binary writer ---

type writer struct{ buf []byte }

func (w *writer) u8(v uint8)   { w.buf = append(w.buf, v) }
func (w *writer) u16(v uint16) { w.buf = append(w.buf, byte(v>>8), byte(v)) }
func (w *writer) u24(v uint32) { w.buf = append(w.buf, byte(v>>16), byte(v>>8), byte(v)) }
func (w *writer) u32(v uint32) {
	w.buf = append(w.buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
func (w *writer) i32(v int32) { w.u32(uint32(v)) }
func (w *writer) u64(v uint64) {
	w.buf = append(w.buf,
		byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
func (w *writer) bytes(b []byte) { w.buf = append(w.buf, b...) }
func (w *writer) str(s string)   { w.buf = append(w.buf, s...) }

// boxBytes wraps payload in a box header of the given type.
func boxBytes(typ string, payload []byte) []byte {
	w := &writer{}
	w.u32(uint32(len(payload) + 8))
	w.str(typ)
	w.bytes(payload)
	return w.buf
}
