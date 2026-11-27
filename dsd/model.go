// Package dsd inspects raw DSDIFF (DFF) and DSF files and exposes a
// per-channel view of their packed DSD samples.
package dsd

import (
	"fmt"
	"io"
)

// Format identifies the container format.
type Format string

const (
	FormatDFF Format = "dff"
	FormatDSF Format = "dsf"
)

// Packing describes how channel data is arranged in the audio payload.
type Packing string

const (
	// PackingByteInterleaved is used by DFF: one byte for each channel in
	// every audio frame.
	PackingByteInterleaved Packing = "byte-interleaved"
	// PackingBlockInterleaved is used by DSF: one fixed-size block for each
	// channel, repeated for every block group.
	PackingBlockInterleaved Packing = "block-interleaved"
)

// BitOrder is the in-byte order of the eight one-bit DSD samples.
type BitOrder string

const (
	BitOrderMSBFirst BitOrder = "msb-first"
	BitOrderLSBFirst BitOrder = "lsb-first"
)

// PaddingKind distinguishes bytes that are not valid DSD samples.
type PaddingKind string

const (
	// PaddingChunkAlignment is an odd-sized DFF chunk's trailing pad byte.
	PaddingChunkAlignment PaddingKind = "chunk-alignment"
	// PaddingDSFBlock is the unused tail of a padded final DSF block.
	PaddingDSFBlock PaddingKind = "dsf-block-tail"
	// PaddingTailBits is the unused part of the final packed DSF byte.
	PaddingTailBits PaddingKind = "packed-byte-tail-bits"
	// PaddingMissingChannel is a synthesized zero byte needed to complete a
	// DFF frame whose payload ended before all channels were received.
	PaddingMissingChannel PaddingKind = "missing-channel-tail"
	// PaddingMissingData is a synthesized zero byte for a truncated DSF block.
	PaddingMissingData PaddingKind = "missing-data"
	// PaddingTrailingData is payload after the declared sample region.
	PaddingTrailingData PaddingKind = "trailing-data"
)

// Padding records a byte range that must not be treated as DSD samples.
// Synthetic ranges do not exist in the file; they describe zero-fill needed
// to align an incomplete trailing unit.
type Padding struct {
	Kind       PaddingKind `json:"kind"`
	Chunk      int         `json:"chunk"`
	Channel    int         `json:"channel"`
	FileOffset int64       `json:"file_offset"`
	Length     int64       `json:"length"`
	Bits       int64       `json:"bits,omitempty"`
	Synthetic  bool        `json:"synthetic"`
	Reason     string      `json:"reason"`
}

// Segment is one channel's regular strided run in an audio chunk. DFF uses
// one-byte units; DSF uses one configured block per unit.
type Segment struct {
	Chunk            int    `json:"chunk"`
	Offset           int64  `json:"offset"`
	Stride           int64  `json:"stride"`
	UnitBytes        int64  `json:"unit_bytes"`
	Units            int64  `json:"units"`
	ValidBytes       int64  `json:"valid_bytes"`
	PaddingBytes     int64  `json:"padding_bytes"`
	MissingBytes     int64  `json:"missing_bytes"`
	FirstSample      int64  `json:"first_sample"`
	TailOffset       int64  `json:"tail_offset,omitempty"`
	TailValidBytes   int64  `json:"tail_valid_bytes,omitempty"`
	TailStoredBytes  int64  `json:"tail_stored_bytes,omitempty"`
	TailPaddingBytes int64  `json:"tail_padding_bytes,omitempty"`
	TailPaddingBits  int64  `json:"tail_padding_bits,omitempty"`
	TailMissingBytes int64  `json:"tail_missing_bytes,omitempty"`
	TailLayout       string `json:"tail_layout,omitempty"`
}

// ChannelTable is the per-channel sample table. ValidBytes is the number of
// packed bytes that belong to the channel; ValidSamples is that count in
// one-bit DSD samples.
type ChannelTable struct {
	Index        int       `json:"index"`
	ID           string    `json:"id"`
	ValidBytes   int64     `json:"valid_bytes"`
	ValidSamples int64     `json:"valid_samples"`
	PaddingBytes int64     `json:"padding_bytes"`
	PaddingBits  int64     `json:"padding_bits,omitempty"`
	MissingBytes int64     `json:"missing_bytes"`
	Segments     []Segment `json:"segments"`
}

// AudioChunk records a raw audio payload found in the container.
type AudioChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Offset   int64  `json:"offset"`
	Length   int64  `json:"length"`
	Declared int64  `json:"declared_length"`
}

// Manifest is a JSON-serializable description of a parsed DSD file. It
// contains offsets only; use ExtractChannel to materialize samples.
type Manifest struct {
	Path            string         `json:"path,omitempty"`
	Format          Format         `json:"format"`
	FormType        string         `json:"form_type,omitempty"`
	FileSize        int64          `json:"file_size"`
	SampleRate      int            `json:"sample_rate_hz"`
	Channels        int            `json:"channels"`
	ChannelIDs      []string       `json:"channel_ids"`
	BitsPerSample   int            `json:"bits_per_sample"`
	BitOrder        BitOrder       `json:"bit_order"`
	Packing         Packing        `json:"packing"`
	BlockSize       int64          `json:"block_size_bytes,omitempty"`
	DeclaredSamples int64          `json:"declared_samples_per_channel,omitempty"`
	AudioBytes      int64          `json:"audio_bytes"`
	ChannelTables   []ChannelTable `json:"channel_tables"`
	AudioChunks     []AudioChunk   `json:"audio_chunks"`
	Padding         []Padding      `json:"padding"`
	Warnings        []string       `json:"warnings,omitempty"`
}

// TotalValidSamples returns the aggregate number of valid one-bit samples
// across all channels.
func (m *Manifest) TotalValidSamples() int64 {
	var total int64
	for _, channel := range m.ChannelTables {
		total += channel.ValidSamples
	}
	return total
}

// SampleTable materializes only valid packed DSD bytes for every channel.
// Padding is intentionally absent; inspect Manifest.Padding for its location.
func (m *Manifest) SampleTable(r io.ReaderAt) ([][]byte, error) {
	table := make([][]byte, len(m.ChannelTables))
	for i := range m.ChannelTables {
		data, err := m.ExtractChannel(r, i)
		if err != nil {
			return nil, err
		}
		table[i] = data
	}
	return table, nil
}

func channelID(ids []string, index int) string {
	if index >= 0 && index < len(ids) && ids[index] != "" {
		return ids[index]
	}
	return fmt.Sprintf("CH%d", index+1)
}
