package dsd

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ParseFile parses a DFF or DSF file without loading its audio payload.
func ParseFile(path string) (*Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	manifest, err := Parse(file, info.Size())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	manifest.Path = path
	return manifest, nil
}

// ParseBytes parses an in-memory DFF or DSF file.
func ParseBytes(data []byte) (*Manifest, error) {
	return Parse(bytesReaderAt(data), int64(len(data)))
}

// Parse detects the container from its magic bytes and parses its metadata
// and channel layout.
func Parse(r io.ReaderAt, size int64) (*Manifest, error) {
	if size < 4 {
		return nil, errors.New("file is too short to contain a DSD container")
	}
	var magic [4]byte
	if err := readFullAt(r, magic[:], 0); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	switch string(magic[:]) {
	case "FRM8":
		return parseDFF(r, size)
	case "DSD ":
		return parseDSF(r, size)
	default:
		return nil, fmt.Errorf("unknown container magic %q (want FRM8 or DSD )", magic)
	}
}

// ExtractChannel returns exactly the valid packed DSD bytes for one channel.
// DSF block padding and DFF chunk alignment bytes are never included.
func (m *Manifest) ExtractChannel(r io.ReaderAt, channel int) ([]byte, error) {
	if channel < 0 || channel >= len(m.ChannelTables) {
		return nil, fmt.Errorf("channel %d out of range [0,%d)", channel, len(m.ChannelTables))
	}
	table := m.ChannelTables[channel]
	if table.ValidBytes < 0 || table.ValidBytes > int64(maxInt()) {
		return nil, fmt.Errorf("channel %d sample size is out of range", channel)
	}
	out := make([]byte, 0, int(table.ValidBytes))
	for _, segment := range table.Segments {
		var err error
		out, err = appendSegment(out, r, segment)
		if err != nil {
			return nil, fmt.Errorf("channel %d chunk %d: %w", channel, segment.Chunk, err)
		}
	}
	if int64(len(out)) != table.ValidBytes {
		return nil, fmt.Errorf("channel %d extracted %d bytes, table declares %d", channel, len(out), table.ValidBytes)
	}
	return out, nil
}

func appendSegment(dst []byte, r io.ReaderAt, segment Segment) ([]byte, error) {
	if segment.Units < 0 || segment.UnitBytes <= 0 || segment.Stride < segment.UnitBytes {
		return nil, errors.New("invalid segment geometry")
	}
	if segment.Units == 0 {
		if segment.TailValidBytes == 0 {
			return dst, nil
		}
		if segment.TailValidBytes > segment.TailStoredBytes {
			return nil, errors.New("tail declares more valid bytes than storage")
		}
		start := len(dst)
		dst = append(dst, make([]byte, segment.TailValidBytes)...)
		if err := readFullAt(r, dst[start:], segment.TailOffset); err != nil {
			return nil, fmt.Errorf("read tail at %d: %w", segment.TailOffset, err)
		}
		return dst, nil
	}
	const targetBatch = 1 << 20

	batchUnits := int64(targetBatch / segment.Stride)
	if batchUnits < 1 {
		batchUnits = 1
	}
	buf := make([]byte, batchUnits*segment.Stride)
	for base := int64(0); base < segment.Units; base += batchUnits {
		count := batchUnits
		if remain := segment.Units - base; remain < count {
			count = remain
		}
		span := (count-1)*segment.Stride + segment.UnitBytes
		if span > int64(len(buf)) {
			return nil, errors.New("internal segment buffer overflow")
		}
		if err := readFullAt(r, buf[:span], segment.Offset+base*segment.Stride); err != nil {
			return nil, fmt.Errorf("read unit at %d: %w", segment.Offset+base*segment.Stride, err)
		}
		for i := int64(0); i < count; i++ {
			start := i * segment.Stride
			dst = append(dst, buf[start:start+segment.UnitBytes]...)
		}
	}

	if segment.TailValidBytes > 0 {
		if segment.TailValidBytes > segment.TailStoredBytes {
			return nil, errors.New("tail declares more valid bytes than storage")
		}
		start := len(dst)
		dst = append(dst, make([]byte, segment.TailValidBytes)...)
		if err := readFullAt(r, dst[start:], segment.TailOffset); err != nil {
			return nil, fmt.Errorf("read tail at %d: %w", segment.TailOffset, err)
		}
	}
	return dst, nil
}

func readFullAt(r io.ReaderAt, p []byte, off int64) error {
	if off < 0 {
		return errors.New("negative file offset")
	}
	n, err := r.ReadAt(p, off)
	if n != len(p) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	return nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

type byteReaderAt []byte

func (b byteReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func bytesReaderAt(b []byte) io.ReaderAt {
	return byteReaderAt(b)
}
