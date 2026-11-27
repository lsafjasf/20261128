package wavpack

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

const (
	scanChunk  = 64 * 1024
	maxBlock   = 256 * 1024 * 1024
	headerWord = "wvpk"
)

// ScanResult is the set of validly located blocks in one .wv or .wvc file.
type ScanResult struct {
	Path   string
	Size   int64
	Blocks []Block
	Issues []string
}

// ScanFile locates and parses every WavPack block in path. Non-block bytes,
// including APEv2/ID3 tags, are skipped. Blocks with bad structure are retained
// with an error so callers can explain why a sample range is not recoverable.
func ScanFile(path string) (*ScanResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}

	result := &ScanResult{Path: path, Size: info.Size()}
	var pos int64
	for pos+HeaderSize <= info.Size() {
		found, err := findBlockHeader(f, info.Size(), pos)
		if err != nil {
			return nil, err
		}
		if found < 0 {
			break
		}

		block, err := readBlock(f, info.Size(), found)
		if err != nil {
			result.Issues = append(result.Issues, fmt.Sprintf("offset %d: %v", found, err))
			pos = found + 1
			continue
		}
		result.Blocks = append(result.Blocks, block)
		pos = found + int64(block.Length)
	}

	return result, nil
}

func findBlockHeader(f *os.File, size, start int64) (int64, error) {
	buf := make([]byte, scanChunk)
	for base := start; base+HeaderSize <= size; {
		n := int64(len(buf))
		if size-base < n {
			n = size - base
		}
		if _, err := f.ReadAt(buf[:n], base); err != nil && err != io.EOF {
			return -1, err
		}

		cursor := 0
		for cursor < int(n) {
			idx := bytes.Index(buf[cursor:int(n)], []byte(headerWord))
			if idx < 0 {
				break
			}
			candidate := base + int64(cursor+idx)
			if candidate+HeaderSize > size {
				return -1, nil
			}
			raw := make([]byte, HeaderSize)
			if _, err := f.ReadAt(raw, candidate); err != nil {
				return -1, err
			}
			h, err := parseHeader(raw)
			if err == nil && int64(h.Size)+8 <= size-candidate {
				return candidate, nil
			}
			cursor += idx + 1
		}

		if n < scanChunk-3 {
			break
		}
		base += n - 3
	}
	return -1, nil
}

func readBlock(f *os.File, fileSize, offset int64) (Block, error) {
	rawHeader := make([]byte, HeaderSize)
	if _, err := f.ReadAt(rawHeader, offset); err != nil {
		return Block{}, err
	}
	h, err := parseHeader(rawHeader)
	if err != nil {
		return Block{}, err
	}
	length := int64(h.Size) + 8
	if length > maxBlock {
		return Block{}, fmt.Errorf("block length %d exceeds safety limit", length)
	}
	if offset+length > fileSize {
		return Block{}, fmt.Errorf("truncated block at offset %d", offset)
	}

	raw := make([]byte, int(length))
	if _, err := f.ReadAt(raw, offset); err != nil {
		return Block{}, err
	}

	block := Block{
		Header:     h,
		FileOffset: offset,
		Length:     int(length),
		Checksum:   ChecksumStatus{Valid: true},
	}
	parseMetadata(raw, &block)
	return block, nil
}

func parseMetadata(block []byte, out *Block) {
	pos := HeaderSize
	for pos < len(block) {
		if len(block)-pos < 2 {
			out.MetadataError = "metadata header is truncated"
			return
		}
		rawID := block[pos]
		storedLen := int(block[pos+1]) << 1
		dataPos := pos + 2

		if rawID&metaLarge != 0 {
			if len(block)-dataPos < 2 {
				out.MetadataError = "large metadata length is truncated"
				return
			}
			storedLen += int(block[dataPos])<<9 + int(block[dataPos+1])<<17
			dataPos += 2
		}

		actualLen := storedLen
		if rawID&metaOddSize != 0 {
			if actualLen == 0 {
				out.MetadataError = "odd-size metadata has zero length"
				return
			}
			actualLen--
		}
		paddedLen := actualLen + actualLen&1
		if dataPos+actualLen > len(block) || dataPos+paddedLen > len(block) {
			out.MetadataError = fmt.Sprintf("metadata payload at %d exceeds block", dataPos)
			return
		}

		id := metaID(rawID)
		meta := Metadata{ID: id, Offset: pos, Length: actualLen}
		out.Metadata = append(out.Metadata, meta)
		switch id {
		case metaWVBitstream:
			out.HasWV = true
		case metaWVCBitstream:
			out.HasWVC = true
			out.WVCBytes = actualLen
		case metaWVXBitstream, metaWVXNew:
			out.HasWVX = true
			out.WVXBytes = actualLen
		case metaShapeWeights:
			out.HasShaping = true
		case metaSampleRate:
			if actualLen == 3 || actualLen == 4 {
				rate := uint32(block[dataPos]) | uint32(block[dataPos+1])<<8 | uint32(block[dataPos+2])<<16
				if actualLen == 4 {
					rate |= uint32(block[dataPos+3]&0x7f) << 24
				}
				out.SampleRateValue = rate
			}
		case metaBlockSum:
			if out.Checksum.Present {
				out.MetadataError = "duplicate BLOCK_CHECKSUM metadata"
				return
			}
			payload := block[dataPos : dataPos+actualLen]
			out.Checksum = checkBlockChecksum(block, pos, payload)
		}
		pos = dataPos + paddedLen
	}

	msg, cs := checkTrailingMetadata(block, pos, out.Flags&flagHasChecksum != 0, out.Checksum)
	out.Checksum = cs
	if msg != "" && out.MetadataError == "" {
		out.MetadataError = msg
	}
}

func sampleRateFor(blocks []Block) uint32 {
	for _, b := range blocks {
		if b.Audio() && b.SampleRateValue != 0 {
			return b.SampleRateValue
		}
	}
	for _, b := range blocks {
		if b.Audio() && b.SampleRate() != 0 {
			return b.SampleRate()
		}
	}
	return 0
}
