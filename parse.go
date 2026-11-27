package mp4safe

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const maxBoxDepth = 64

// File is a parsed ISO BMFF file. ReaderAt is retained for sample access.
type File struct {
	Reader      io.ReaderAt
	Size        uint64
	Boxes       []*Box
	allBoxes    []*Box
	Tracks      []*Track
	Unsupported []string
}

// Parse validates the box layout and sample tables in r.
func Parse(r io.ReaderAt, size int64) (*File, error) {
	if size < 0 || uint64(size) > math.MaxInt64 {
		return nil, fmt.Errorf("invalid file size %d", size)
	}
	f := &File{Reader: r, Size: uint64(size)}
	for off := uint64(0); off < f.Size; {
		b, err := f.parseBox(off, f.Size, nil, 0)
		if err != nil {
			return nil, err
		}
		f.Boxes = append(f.Boxes, b)
		off = b.End
	}
	if err := f.collectTracks(); err != nil {
		return nil, err
	}
	f.collectUnsupported()
	return f, nil
}

func (f *File) parseBox(start, limit uint64, parent *Box, depth int) (*Box, error) {
	if depth > maxBoxDepth {
		return nil, fmt.Errorf("box nesting exceeds %d at offset %d", maxBoxDepth, start)
	}
	if limit-start < 8 {
		return nil, fmt.Errorf("truncated box header at offset %d", start)
	}
	var hdr [8]byte
	if err := f.readAt(hdr[:], start); err != nil {
		return nil, fmt.Errorf("read box header at %d: %w", start, err)
	}
	size32 := binary.BigEndian.Uint32(hdr[:4])
	var typ [4]byte
	copy(typ[:], hdr[4:8])
	headerSize := uint64(8)
	boxSize := uint64(size32)
	switch size32 {
	case 0:
		boxSize = limit - start
	case 1:
		if limit-start < 16 {
			return nil, fmt.Errorf("truncated 64-bit box size at offset %d", start)
		}
		var ext [8]byte
		if err := f.readAt(ext[:], start+8); err != nil {
			return nil, fmt.Errorf("read 64-bit box size at %d: %w", start, err)
		}
		boxSize = binary.BigEndian.Uint64(ext[:])
		headerSize = 16
	}
	if typ == [4]byte{'u', 'u', 'i', 'd'} {
		if limit-start < headerSize+16 {
			return nil, fmt.Errorf("truncated uuid box at offset %d", start)
		}
		headerSize += 16
	}
	if boxSize < headerSize {
		return nil, fmt.Errorf("box %s at %d has size %d smaller than header %d",
			TypeName(typ), start, boxSize, headerSize)
	}
	if boxSize > limit-start {
		return nil, fmt.Errorf("box %s at %d overruns parent by %d bytes",
			TypeName(typ), start, boxSize-(limit-start))
	}
	if start+boxSize < start {
		return nil, fmt.Errorf("box %s at %d has overflowing size", TypeName(typ), start)
	}

	b := &Box{
		Type:        typ,
		Start:       start,
		End:         start + boxSize,
		HeaderSize:  headerSize,
		PayloadSize: boxSize - headerSize,
		Parent:      parent,
		Depth:       depth,
	}
	if typ == [4]byte{'u', 'u', 'i', 'd'} {
		b.HasUserType = true
		if err := f.readAt(b.UserType[:], start+headerSize-16); err != nil {
			return nil, fmt.Errorf("read uuid user type at %d: %w", start, err)
		}
	}
	b.Class = classOf(b, parent)
	f.allBoxes = append(f.allBoxes, b)

	childrenStart := b.PayloadStart()
	if typ == [4]byte{'m', 'e', 't', 'a'} {
		if b.PayloadSize < 4 {
			return nil, fmt.Errorf("meta box at %d has no full-box header", start)
		}
		childrenStart += 4
	}
	if containerTypes[typ] || typ == [4]byte{'m', 'e', 't', 'a'} {
		if childrenStart > b.End {
			return nil, fmt.Errorf("container %s at %d has invalid payload", TypeName(typ), start)
		}
		for off := childrenStart; off < b.End; {
			child, err := f.parseBox(off, b.End, b, depth+1)
			if err != nil {
				return nil, err
			}
			b.Children = append(b.Children, child)
			off = child.End
		}
	}
	return b, nil
}

func classOf(b *Box, parent *Box) Class {
	switch b.Type {
	case [4]byte{'m', 'd', 'a', 't'}:
		return ClassMedia
	case [4]byte{'s', 't', 'c', 'o'}, [4]byte{'c', 'o', '6', '4'}:
		return ClassIndex
	}
	if isFreeOrSkip(b.Type) {
		b.Removable = parent == nil
		return ClassPadding
	}
	if b.Type == [4]byte{'u', 'u', 'i', 'd'} || !isKnownType(b.Type) {
		return ClassUnknown
	}
	return ClassStructure
}

func (f *File) readAt(p []byte, off uint64) error {
	if off > f.Size || uint64(len(p)) > f.Size-off {
		return io.ErrUnexpectedEOF
	}
	n, err := f.Reader.ReadAt(p, int64(off))
	if err != nil && err != io.EOF {
		return err
	}
	if n != len(p) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (f *File) payload(b *Box) ([]byte, error) {
	if b.PayloadSize > uint64(math.MaxInt) {
		return nil, fmt.Errorf("box %s payload too large: %d", b.TypeName(), b.PayloadSize)
	}
	p := make([]byte, int(b.PayloadSize))
	if err := f.readAt(p, b.PayloadStart()); err != nil {
		return nil, fmt.Errorf("read %s payload: %w", b.TypeName(), err)
	}
	return p, nil
}

func findChild(parent *Box, typ [4]byte) *Box {
	for _, child := range parent.Children {
		if child.Type == typ {
			return child
		}
	}
	return nil
}

func hasBox(root *Box, typ [4]byte) bool {
	if root.Type == typ {
		return true
	}
	for _, child := range root.Children {
		if hasBox(child, typ) {
			return true
		}
	}
	return false
}

func (f *File) collectUnsupported() {
	seen := make(map[string]bool)
	add := func(reason string) {
		if !seen[reason] {
			seen[reason] = true
			f.Unsupported = append(f.Unsupported, reason)
		}
	}
	for _, b := range f.Boxes {
		switch b.Type {
		case [4]byte{'m', 'o', 'o', 'f'}, [4]byte{'m', 'f', 'r', 'a'},
			[4]byte{'s', 'i', 'd', 'x'}, [4]byte{'s', 's', 'i', 'x'},
			[4]byte{'s', 't', 'y', 'p'}:
			add("fragmented or segmented media (" + b.TypeName() + ")")
		}
	}
	if moov := f.topLevel([4]byte{'m', 'o', 'o', 'v'}); moov != nil {
		if hasBox(moov, [4]byte{'m', 'v', 'e', 'x'}) {
			add("fragmented media (mvex)")
		}
		if hasBox(moov, [4]byte{'s', 'a', 'i', 'o'}) {
			add("absolute auxiliary offsets (saio)")
		}
	}
	for _, b := range f.allBoxes {
		if b.Type == [4]byte{'i', 'l', 'o', 'c'} {
			add("absolute item offsets (iloc)")
		}
	}
}

func (f *File) topLevel(typ [4]byte) *Box {
	for _, b := range f.Boxes {
		if b.Type == typ {
			return b
		}
	}
	return nil
}

// Padding returns removable top-level free/skip boxes in file order.
func (f *File) Padding() []*Box {
	var boxes []*Box
	for _, b := range f.Boxes {
		if b.Removable {
			boxes = append(boxes, b)
		}
	}
	return boxes
}
