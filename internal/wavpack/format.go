// Package wavpack contains a read-only structural parser for WavPack blocks.
// It intentionally does not synthesize audio samples.
package wavpack

import (
	"encoding/binary"
	"fmt"
)

const (
	HeaderSize = 32

	flagBytesStored = 0x00000003
	flagMono        = 0x00000004
	flagHybrid      = 0x00000008
	flagFloat       = 0x00000080
	flagInt32       = 0x00000100
	flagInitial     = 0x00000800
	flagFinal       = 0x00001000
	flagMagMask     = 0x007c0000
	flagSampleMask  = 0x07800000
	flagHasChecksum = 0x10000000
	flagDSD         = 0x80000000

	metaUnique       = 0x3f
	metaOptional     = 0x20
	metaOddSize      = 0x40
	metaLarge        = 0x80
	metaWVBitstream  = 0x0a
	metaWVCBitstream = 0x0b
	metaWVXBitstream = 0x0c
	metaShapeWeights = 0x07
	metaSampleRate   = 0x27
	metaBlockSum     = 0x2f
	metaWVXNew       = 0x2c
)

var sampleRates = [...]uint32{
	6000, 8000, 9600, 11025, 12000, 16000, 22050,
	24000, 32000, 44100, 48000, 64000, 88200,
	96000, 192000,
}

// Header is the 32-byte WavPack 4/5 block header. BlockIndexLow and
// BlockIndexHigh form the 40-bit sample index used by large files.
type Header struct {
	Raw              []byte
	Size             uint32
	Version          uint16
	BlockIndexHigh   uint8
	TotalSamplesHigh uint8
	TotalSamplesLow  uint32
	BlockIndexLow    uint32
	BlockSamples     uint32
	Flags            uint32
	CRC              uint32
}

func parseHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, fmt.Errorf("short WavPack header: %d bytes", len(b))
	}
	size := binary.LittleEndian.Uint32(b[4:8])
	if size&1 != 0 {
		return Header{}, fmt.Errorf("odd ckSize %d", size)
	}
	if size+8 < HeaderSize {
		return Header{}, fmt.Errorf("ckSize %d is smaller than a header", size)
	}
	version := binary.LittleEndian.Uint16(b[8:10])
	if version < 0x402 || version > 0x410 {
		return Header{}, fmt.Errorf("unsupported stream version 0x%x", version)
	}
	return Header{
		Raw:              append([]byte(nil), b[:HeaderSize]...),
		Size:             size,
		Version:          version,
		BlockIndexHigh:   b[10],
		TotalSamplesHigh: b[11],
		TotalSamplesLow:  binary.LittleEndian.Uint32(b[12:16]),
		BlockIndexLow:    binary.LittleEndian.Uint32(b[16:20]),
		BlockSamples:     binary.LittleEndian.Uint32(b[20:24]),
		Flags:            binary.LittleEndian.Uint32(b[24:28]),
		CRC:              binary.LittleEndian.Uint32(b[28:32]),
	}, nil
}

// BlockIndex returns the unsigned 40-bit sample index.
func (h Header) BlockIndex() uint64 {
	return uint64(h.BlockIndexLow) + uint64(h.BlockIndexHigh)<<32
}

// TotalSamples returns the 40-bit sample count, or -1 when unknown.
func (h Header) TotalSamples() int64 {
	if h.TotalSamplesLow == ^uint32(0) {
		return -1
	}
	return int64(uint64(h.TotalSamplesLow) + uint64(h.TotalSamplesHigh)<<32 - uint64(h.TotalSamplesHigh))
}

func (h Header) Hybrid() bool  { return h.Flags&flagHybrid != 0 }
func (h Header) Initial() bool { return h.Flags&flagInitial != 0 }
func (h Header) Final() bool   { return h.Flags&flagFinal != 0 }
func (h Header) Mono() bool    { return h.Flags&flagMono != 0 }
func (h Header) BytesPerSample() int {
	return int(h.Flags&flagBytesStored) + 1
}
func (h Header) SampleRate() uint32 {
	idx := (h.Flags & flagSampleMask) >> 23
	if idx < uint32(len(sampleRates)) {
		return sampleRates[idx]
	}
	return 0
}

// Metadata describes one WavPack sub-block. Payload bytes are deliberately not
// retained so scanning a large file does not duplicate its bitstreams in RAM.
type Metadata struct {
	ID     byte
	Offset int
	Length int
}

// Block is a structurally parsed WavPack block.
type Block struct {
	Header
	FileOffset      int64
	Length          int
	Metadata        []Metadata
	MetadataError   string
	Checksum        ChecksumStatus
	HasWV           bool
	HasWVC          bool
	HasWVX          bool
	HasShaping      bool
	WVCBytes        int
	WVXBytes        int
	SampleRateValue uint32
}

func (b Block) Valid() bool {
	return b.MetadataError == "" && b.Checksum.Valid
}

func (b Block) Audio() bool {
	return b.BlockSamples != 0
}

func (b Block) CorrectionData() bool {
	return b.HasWVC && b.WVCBytes > 0 || b.HasWVX && b.WVXBytes > 4
}

func metaID(id byte) byte {
	return id & metaUnique
}
