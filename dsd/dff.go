package dsd

import (
	"errors"
	"fmt"
	"io"
)

const dffHeaderSize = 12

type dffChunk struct {
	id        string
	size      int64
	headerOff int64
	bodyOff   int64
	hasPad    bool
}

func parseDFF(r io.ReaderAt, fileSize int64) (*Manifest, error) {
	if fileSize < 16 {
		return nil, errors.New("DFF file is shorter than FRM8 header")
	}
	declared, err := readU64BE(r, 4)
	if err != nil {
		return nil, fmt.Errorf("read FRM8 size: %w", err)
	}
	declaredSize, err := u64ToI64(declared, "FRM8 size")
	if err != nil {
		return nil, err
	}
	formType, err := readFourCC(r, 12)
	if err != nil {
		return nil, fmt.Errorf("read FRM8 form type: %w", err)
	}
	if formType == "DST " {
		return nil, errors.New("DST-compressed DFF is not supported")
	}
	if formType != "DSD " {
		return nil, fmt.Errorf("unsupported FRM8 form type %q", formType)
	}

	bodyEnd, err := addInt64(dffHeaderSize, declaredSize, "FRM8 body")
	if err != nil {
		return nil, err
	}
	if bodyEnd > fileSize {
		return nil, fmt.Errorf("FRM8 declares %d bytes, file has %d", bodyEnd, fileSize)
	}
	if bodyEnd < 16 {
		return nil, errors.New("FRM8 has no child chunks")
	}

	manifest := &Manifest{
		Format:        FormatDFF,
		FormType:      formType,
		FileSize:      fileSize,
		BitsPerSample: 1,
		BitOrder:      BitOrderMSBFirst,
		Packing:       PackingByteInterleaved,
		Padding:       []Padding{},
	}
	if declaredSize%2 == 1 && bodyEnd < fileSize {
		manifest.Padding = append(manifest.Padding, Padding{
			Kind:       PaddingChunkAlignment,
			Chunk:      -1,
			Channel:    -1,
			FileOffset: bodyEnd,
			Length:     1,
			Reason:     "FRM8 container has odd size",
		})
	}
	var chunks []dffChunk
	off := int64(16)
	for off < bodyEnd {
		if bodyEnd-off < dffHeaderSize {
			manifest.Warnings = append(manifest.Warnings,
				fmt.Sprintf("ignored %d unparsed bytes at FRM8 end", bodyEnd-off))
			break
		}
		chunk, err := readDFFChunk(r, off, bodyEnd, fileSize)
		if err != nil {
			return nil, err
		}
		switch chunk.id {
		case "FVER":
			if chunk.size < 4 {
				return nil, fmt.Errorf("FVER at %d is too short", chunk.headerOff)
			}
		case "PROP":
			if err := parseDFFProperties(r, chunk, manifest); err != nil {
				return nil, err
			}
		case "DSD ":
			chunks = append(chunks, chunk)
		case "DST ":
			return nil, errors.New("DST-compressed DFF is not supported")
		}
		if chunk.size%2 == 1 && chunk.hasPad {
			manifest.Padding = append(manifest.Padding, Padding{
				Kind:       PaddingChunkAlignment,
				Chunk:      -1,
				Channel:    -1,
				FileOffset: chunk.bodyOff + chunk.size,
				Length:     1,
				Reason:     fmt.Sprintf("DFF chunk %q has odd size", chunk.id),
			})
		}
		off = chunk.bodyOff + chunk.size
		if chunk.size%2 == 1 && chunk.hasPad {
			off++
		} else if chunk.size%2 == 1 {
			manifest.Warnings = append(manifest.Warnings,
				fmt.Sprintf("DFF chunk %q at %d has no alignment pad byte", chunk.id, chunk.headerOff))
		}
	}

	if manifest.SampleRate == 0 {
		return nil, errors.New("DFF is missing PROP/SND/FS sample rate")
	}
	if manifest.Channels == 0 {
		return nil, errors.New("DFF is missing PROP/SND/CHNL channel information")
	}
	if len(chunks) == 0 {
		return nil, errors.New("DFF contains no DSD audio chunk")
	}
	if err := buildDFFTables(manifest, chunks); err != nil {
		return nil, err
	}
	if len(manifest.ChannelIDs) < manifest.Channels {
		manifest.ChannelIDs = append(manifest.ChannelIDs, make([]string, manifest.Channels-len(manifest.ChannelIDs))...)
	}
	for i := 0; i < manifest.Channels; i++ {
		if manifest.ChannelIDs[i] == "" {
			manifest.ChannelIDs[i] = fmt.Sprintf("CH%d", i+1)
		}
	}
	return manifest, nil
}

func readDFFChunk(r io.ReaderAt, off, limit, fileSize int64) (dffChunk, error) {
	if limit-off < dffHeaderSize {
		return dffChunk{}, fmt.Errorf("truncated chunk header at %d", off)
	}
	id, err := readFourCC(r, off)
	if err != nil {
		return dffChunk{}, fmt.Errorf("read chunk ID at %d: %w", off, err)
	}
	size, err := readU64BE(r, off+4)
	if err != nil {
		return dffChunk{}, fmt.Errorf("read %q size at %d: %w", id, off, err)
	}
	bodySize, err := u64ToI64(size, fmt.Sprintf("%q chunk size", id))
	if err != nil {
		return dffChunk{}, err
	}
	bodyOff, err := addInt64(off, dffHeaderSize, "chunk body")
	if err != nil {
		return dffChunk{}, err
	}
	bodyEnd, err := addInt64(bodyOff, bodySize, "chunk end")
	if err != nil {
		return dffChunk{}, err
	}
	if bodyEnd > limit {
		return dffChunk{}, fmt.Errorf("chunk %q at %d exceeds container end %d", id, off, limit)
	}
	return dffChunk{
		id:        id,
		size:      bodySize,
		headerOff: off,
		bodyOff:   bodyOff,
		hasPad:    bodySize%2 == 1 && bodyEnd < fileSize && bodyEnd < limit,
	}, nil
}

func parseDFFProperties(r io.ReaderAt, prop dffChunk, manifest *Manifest) error {
	if prop.size < 4 {
		return fmt.Errorf("PROP at %d has no property type", prop.headerOff)
	}
	propType, err := readFourCC(r, prop.bodyOff)
	if err != nil {
		return fmt.Errorf("read PROP type: %w", err)
	}
	if propType != "SND " {
		return nil
	}

	end := prop.bodyOff + prop.size
	for off := prop.bodyOff + 4; off < end; {
		if end-off < dffHeaderSize {
			manifest.Warnings = append(manifest.Warnings,
				fmt.Sprintf("ignored %d unparsed bytes in PROP/SND", end-off))
			break
		}
		chunk, err := readDFFChunk(r, off, end, manifest.FileSize)
		if err != nil {
			return err
		}
		switch chunk.id {
		case "FS  ":
			if chunk.size < 4 {
				return fmt.Errorf("FS chunk at %d is too short", chunk.headerOff)
			}
			rate, err := readU32BE(r, chunk.bodyOff)
			if err != nil {
				return fmt.Errorf("read sample rate: %w", err)
			}
			manifest.SampleRate = int(rate)
		case "CHNL":
			if chunk.size < 2 {
				return fmt.Errorf("CHNL chunk at %d is too short", chunk.headerOff)
			}
			count, err := readU16BE(r, chunk.bodyOff)
			if err != nil {
				return fmt.Errorf("read channel count: %w", err)
			}
			if count == 0 {
				return errors.New("DFF channel count is zero")
			}
			manifest.Channels = int(count)
			manifest.ChannelIDs = make([]string, int(count))
			idsAvailable := (chunk.size - 2) / 4
			if idsAvailable > int64(count) {
				idsAvailable = int64(count)
			}
			for i := int64(0); i < idsAvailable; i++ {
				id, err := readFourCC(r, chunk.bodyOff+2+i*4)
				if err != nil {
					return fmt.Errorf("read channel ID %d: %w", i, err)
				}
				manifest.ChannelIDs[i] = id
			}
		case "CMPR":
			if chunk.size < 4 {
				return fmt.Errorf("CMPR chunk at %d is too short", chunk.headerOff)
			}
			compression, err := readFourCC(r, chunk.bodyOff)
			if err != nil {
				return fmt.Errorf("read compression type: %w", err)
			}
			if compression == "DST " {
				return errors.New("DST-compressed DFF is not supported")
			}
			if compression != "DSD " {
				return fmt.Errorf("unsupported DFF compression %q", compression)
			}
		}
		if chunk.size%2 == 1 && chunk.hasPad {
			manifest.Padding = append(manifest.Padding, Padding{
				Kind:       PaddingChunkAlignment,
				Chunk:      -1,
				Channel:    -1,
				FileOffset: chunk.bodyOff + chunk.size,
				Length:     1,
				Reason:     fmt.Sprintf("DFF property chunk %q has odd size", chunk.id),
			})
		}
		off = chunk.bodyOff + chunk.size
		if chunk.size%2 == 1 && chunk.hasPad {
			off++
		} else if chunk.size%2 == 1 {
			manifest.Warnings = append(manifest.Warnings,
				fmt.Sprintf("DFF property chunk %q at %d has no alignment pad byte", chunk.id, chunk.headerOff))
		}
	}
	return nil
}

func buildDFFTables(manifest *Manifest, chunks []dffChunk) error {
	channels := manifest.Channels
	manifest.ChannelTables = make([]ChannelTable, channels)
	for i := range manifest.ChannelTables {
		manifest.ChannelTables[i] = ChannelTable{Index: i, ID: channelID(manifest.ChannelIDs, i)}
	}
	firstSample := make([]int64, channels)

	for chunkIndex, chunk := range chunks {
		manifest.AudioChunks = append(manifest.AudioChunks, AudioChunk{
			Index:    chunkIndex,
			ID:       chunk.id,
			Offset:   chunk.bodyOff,
			Length:   chunk.size,
			Declared: chunk.size,
		})
		manifest.AudioBytes += chunk.size
		fullFrames := chunk.size / int64(channels)
		tailBytes := chunk.size % int64(channels)

		for channel := 0; channel < channels; channel++ {
			validBytes := fullFrames
			segment := Segment{
				Chunk:       chunkIndex,
				Offset:      chunk.bodyOff + int64(channel),
				Stride:      int64(channels),
				UnitBytes:   1,
				Units:       fullFrames,
				ValidBytes:  fullFrames,
				FirstSample: firstSample[channel],
			}
			if int64(channel) < tailBytes {
				validBytes++
				segment.TailOffset = chunk.bodyOff + fullFrames*int64(channels) + int64(channel)
				segment.TailValidBytes = 1
				segment.TailStoredBytes = 1
				segment.TailLayout = "partial-frame"
			} else if tailBytes > 0 {
				segment.MissingBytes = 1
				segment.TailMissingBytes = 1
				segment.TailLayout = "missing-channel"
				manifest.Padding = append(manifest.Padding, Padding{
					Kind:       PaddingMissingChannel,
					Chunk:      chunkIndex,
					Channel:    channel,
					FileOffset: chunk.bodyOff + chunk.size,
					Length:     1,
					Synthetic:  true,
					Reason:     "DFF audio payload ends mid-frame; zero-fill needed to align channels",
				})
			}
			segment.ValidBytes = validBytes
			table := &manifest.ChannelTables[channel]
			table.Segments = append(table.Segments, segment)
			table.ValidBytes += validBytes
			table.ValidSamples += validBytes * 8
			table.MissingBytes += segment.MissingBytes
			firstSample[channel] += validBytes * 8
		}
	}
	return nil
}
