package dsd

import (
	"errors"
	"fmt"
	"io"
)

const dsfHeaderSize = 28

func parseDSF(r io.ReaderAt, fileSize int64) (*Manifest, error) {
	if fileSize < 92 {
		return nil, errors.New("DSF file is shorter than its fixed header")
	}
	headerSize, err := readU64LE(r, 4)
	if err != nil {
		return nil, fmt.Errorf("read DSF header size: %w", err)
	}
	if headerSize != 28 {
		return nil, fmt.Errorf("unexpected DSF header size %d (want 28)", headerSize)
	}
	declaredFileSize, err := readU64LE(r, 12)
	if err != nil {
		return nil, fmt.Errorf("read DSF file size: %w", err)
	}
	metadataOffset, err := readU64LE(r, 20)
	if err != nil {
		return nil, fmt.Errorf("read DSF metadata offset: %w", err)
	}

	fmtID, err := readFourCC(r, dsfHeaderSize)
	if err != nil {
		return nil, fmt.Errorf("read fmt chunk ID: %w", err)
	}
	if fmtID != "fmt " {
		return nil, fmt.Errorf("expected fmt chunk at %d, found %q", dsfHeaderSize, fmtID)
	}
	fmtSize, err := readU64LE(r, dsfHeaderSize+4)
	if err != nil {
		return nil, fmt.Errorf("read fmt chunk size: %w", err)
	}
	if fmtSize < 52 {
		return nil, fmt.Errorf("DSF fmt chunk is %d bytes, want at least 52", fmtSize)
	}
	fmtEnd, err := addInt64(dsfHeaderSize, int64(fmtSize), "fmt chunk")
	if err != nil {
		return nil, err
	}
	if fmtEnd > fileSize {
		return nil, fmt.Errorf("DSF fmt chunk ends at %d, file has %d", fmtEnd, fileSize)
	}
	if fmtEnd < 80 {
		return nil, errors.New("DSF fmt chunk is too short for fixed fields")
	}

	version, err := readU32LE(r, 40)
	if err != nil {
		return nil, fmt.Errorf("read fmt version: %w", err)
	}
	if version != 1 {
		return nil, fmt.Errorf("unsupported DSF format version %d", version)
	}
	formatID, err := readU32LE(r, 44)
	if err != nil {
		return nil, fmt.Errorf("read format ID: %w", err)
	}
	if formatID != 0 {
		return nil, fmt.Errorf("unsupported DSF format ID %d (want raw DSD)", formatID)
	}
	channelType, err := readU32LE(r, 48)
	if err != nil {
		return nil, fmt.Errorf("read channel type: %w", err)
	}
	channels, err := readU32LE(r, 52)
	if err != nil {
		return nil, fmt.Errorf("read channel count: %w", err)
	}
	if channels == 0 || channels > 256 {
		return nil, fmt.Errorf("invalid DSF channel count %d", channels)
	}
	sampleRate, err := readU32LE(r, 56)
	if err != nil {
		return nil, fmt.Errorf("read sample rate: %w", err)
	}
	if sampleRate == 0 {
		return nil, errors.New("DSF sample rate is zero")
	}
	bitsPerSample, err := readU32LE(r, 60)
	if err != nil {
		return nil, fmt.Errorf("read bits per sample: %w", err)
	}
	if bitsPerSample != 1 && bitsPerSample != 8 {
		return nil, fmt.Errorf("unsupported DSF bits-per-sample %d", bitsPerSample)
	}
	sampleCount, err := readU64LE(r, 64)
	if err != nil {
		return nil, fmt.Errorf("read sample count: %w", err)
	}
	samplesPerChannel, err := u64ToI64(sampleCount, "DSF sample count")
	if err != nil {
		return nil, err
	}
	blockSize, err := readU32LE(r, 72)
	if err != nil {
		return nil, fmt.Errorf("read block size: %w", err)
	}
	if blockSize == 0 {
		return nil, errors.New("DSF block size is zero")
	}

	dataOff := fmtEnd
	if dataOff+12 > fileSize {
		return nil, errors.New("DSF data chunk header is truncated")
	}
	dataID, err := readFourCC(r, dataOff)
	if err != nil {
		return nil, fmt.Errorf("read data chunk ID: %w", err)
	}
	if dataID != "data" {
		return nil, fmt.Errorf("expected data chunk at %d, found %q", dataOff, dataID)
	}
	dataField, err := readU64LE(r, dataOff+4)
	if err != nil {
		return nil, fmt.Errorf("read data chunk size: %w", err)
	}
	dataStart := dataOff + 12
	available := fileSize - dataStart
	var dataLength int64
	var declaredPayload int64
	switch {
	case dataField >= 12:
		dataLength = int64(dataField - 12)
		declaredPayload = dataLength
	case dataField <= uint64(available):
		dataLength = int64(dataField)
		declaredPayload = dataLength
	default:
		return nil, fmt.Errorf("invalid DSF data chunk size %d", dataField)
	}

	manifest := &Manifest{
		Format:          FormatDSF,
		FormType:        "DSD ",
		FileSize:        fileSize,
		SampleRate:      int(sampleRate),
		Channels:        int(channels),
		ChannelIDs:      dsfChannelIDs(channelType, int(channels)),
		BitsPerSample:   int(bitsPerSample),
		BitOrder:        BitOrderMSBFirst,
		Packing:         PackingBlockInterleaved,
		BlockSize:       int64(blockSize),
		DeclaredSamples: samplesPerChannel,
		Padding:         []Padding{},
	}
	if bitsPerSample == 1 {
		manifest.BitOrder = BitOrderLSBFirst
	}
	if declaredFileSize != 0 && declaredFileSize != uint64(fileSize) {
		manifest.Warnings = append(manifest.Warnings,
			fmt.Sprintf("DSF header declares file size %d, actual size is %d", declaredFileSize, fileSize))
	}
	if metadataOffset != 0 && metadataOffset < uint64(dataStart) {
		manifest.Warnings = append(manifest.Warnings,
			fmt.Sprintf("DSF metadata offset %d points before audio data", metadataOffset))
	}
	if dataLength > available {
		manifest.Warnings = append(manifest.Warnings,
			fmt.Sprintf("DSF data chunk declares %d bytes, only %d are present", dataLength, available))
		dataLength = available
	}
	manifest.AudioBytes = dataLength
	manifest.AudioChunks = []AudioChunk{{
		Index:    0,
		ID:       "data",
		Offset:   dataStart,
		Length:   dataLength,
		Declared: declaredPayload,
	}}
	if err := buildDSFTables(manifest, dataStart, dataLength, samplesPerChannel); err != nil {
		return nil, err
	}
	return manifest, nil
}

func dsfChannelIDs(channelType uint32, channels int) []string {
	layouts := map[uint32][]string{
		1: {"M"},
		2: {"L", "R"},
		3: {"L", "R", "C"},
		4: {"L", "R", "Ls", "Rs"},
		5: {"L", "R", "C", "BC"},
		6: {"L", "R", "C", "Ls", "Rs"},
		7: {"L", "R", "C", "LFE", "Ls", "Rs"},
	}
	ids := make([]string, channels)
	if layout, ok := layouts[channelType]; ok && len(layout) == channels {
		copy(ids, layout)
	}
	for i := range ids {
		if ids[i] == "" {
			ids[i] = fmt.Sprintf("CH%d", i+1)
		}
	}
	return ids
}

func buildDSFTables(manifest *Manifest, dataStart, dataLength, samplesPerChannel int64) error {
	channels := int64(manifest.Channels)
	blockSize := manifest.BlockSize
	validBytes := samplesPerChannel / 8
	tailBitCount := samplesPerChannel % 8
	if tailBitCount != 0 {
		validBytes++
	}
	fullBlocks := validBytes / blockSize
	tailBytes := validBytes % blockSize

	groupBytes, err := mulInt64(channels, blockSize, "DSF block group size")
	if err != nil {
		return err
	}
	base, err := mulInt64(fullBlocks, groupBytes, "DSF full-block payload size")
	if err != nil {
		return err
	}
	dataEnd, err := addInt64(dataStart, dataLength, "DSF data end")
	if err != nil {
		return err
	}
	paddedEnd := base
	tightEnd := base
	if tailBytes > 0 {
		paddedEnd, err = addInt64(base, groupBytes, "DSF padded payload end")
		if err != nil {
			return err
		}
		tightBytes, err := mulInt64(channels, tailBytes, "DSF tight tail size")
		if err != nil {
			return err
		}
		tightEnd, err = addInt64(base, tightBytes, "DSF tight payload end")
		if err != nil {
			return err
		}
	}

	tailMode := "none"
	if tailBytes > 0 {
		switch {
		case dataLength >= paddedEnd:
			tailMode = "block-padded"
		case dataLength == tightEnd:
			tailMode = "tail-packed"
		case dataLength <= tightEnd:
			tailMode = "tail-packed-short"
		default:
			tailMode = "block-padded-short"
		}
	}

	manifest.ChannelTables = make([]ChannelTable, manifest.Channels)
	for i := range manifest.ChannelTables {
		manifest.ChannelTables[i] = ChannelTable{Index: i, ID: manifest.ChannelIDs[i]}
	}
	firstSample := make([]int64, manifest.Channels)

	for channel := 0; channel < manifest.Channels; channel++ {
		channel64 := int64(channel)
		firstOffset, err := addInt64(dataStart, channel64*blockSize, "DSF channel offset")
		if err != nil {
			return err
		}
		completeGroups := dataLength / groupBytes
		if completeGroups > fullBlocks {
			completeGroups = fullBlocks
		}
		units := completeGroups
		if units < fullBlocks {
			nextOffset := dataStart + (units*channels+channel64)*blockSize
			if nextOffset+blockSize <= dataEnd {
				units++
			}
		}

		actual := units * blockSize
		segment := Segment{
			Chunk:       0,
			Offset:      firstOffset,
			Stride:      groupBytes,
			UnitBytes:   blockSize,
			Units:       units,
			ValidBytes:  actual,
			FirstSample: firstSample[channel],
		}

		if units < fullBlocks {
			partialOffset := dataStart + (units*channels+channel64)*blockSize
			stored := dataEnd - partialOffset
			if stored < 0 {
				stored = 0
			}
			if stored > blockSize {
				stored = blockSize
			}
			actual += stored
			segment.TailOffset = partialOffset
			segment.TailStoredBytes = stored
			segment.TailValidBytes = stored
			segment.TailLayout = "truncated-full-block"
			if stored > 0 {
				segment.ValidBytes = actual
			}
		} else if tailBytes > 0 {
			var tailOffset int64
			var slotSize int64
			switch tailMode {
			case "tail-packed", "tail-packed-short":
				tailOffset = dataStart + base + channel64*tailBytes
				slotSize = tailBytes
			default:
				tailOffset = dataStart + base + channel64*blockSize
				slotSize = blockSize
			}
			stored := dataEnd - tailOffset
			if stored < 0 {
				stored = 0
			}
			if stored > slotSize {
				stored = slotSize
			}
			valid := stored
			if valid > tailBytes {
				valid = tailBytes
			}
			padding := stored - valid
			missing := tailBytes - valid
			actual += valid
			segment.TailOffset = tailOffset
			segment.TailStoredBytes = stored
			segment.TailValidBytes = valid
			segment.TailPaddingBytes = padding
			segment.TailMissingBytes = missing
			segment.TailLayout = tailMode
			segment.PaddingBytes += padding
			segment.MissingBytes += missing
			if padding > 0 {
				manifest.Padding = append(manifest.Padding, Padding{
					Kind:       PaddingDSFBlock,
					Chunk:      0,
					Channel:    channel,
					FileOffset: tailOffset + valid,
					Length:     padding,
					Reason:     "unused bytes after the final valid sample in a DSF block",
				})
			}
			if missing > 0 {
				manifest.Padding = append(manifest.Padding, Padding{
					Kind:       PaddingMissingData,
					Chunk:      0,
					Channel:    channel,
					FileOffset: tailOffset + stored,
					Length:     missing,
					Synthetic:  true,
					Reason:     "DSF data ends before the declared final block",
				})
			}
		}

		if units < fullBlocks {
			missing := validBytes - actual
			if missing > 0 {
				segment.MissingBytes += missing
				manifest.Padding = append(manifest.Padding, Padding{
					Kind:       PaddingMissingData,
					Chunk:      0,
					Channel:    channel,
					FileOffset: dataEnd,
					Length:     missing,
					Synthetic:  true,
					Reason:     "DSF data ends before the declared sample region",
				})
			}
		}

		segment.ValidBytes = actual
		if actual == validBytes && tailBitCount > 0 {
			offset, err := finalPackedByteOffset(dataStart, dataEnd, channel64, channels, blockSize, fullBlocks, tailBytes, validBytes, segment)
			if err != nil {
				return err
			}
			bits := 8 - tailBitCount
			manifest.Padding = append(manifest.Padding, Padding{
				Kind:       PaddingTailBits,
				Chunk:      0,
				Channel:    channel,
				FileOffset: offset,
				Length:     0,
				Bits:       bits,
				Reason:     fmt.Sprintf("%d unused bits in the final packed byte", bits),
			})
			segment.TailPaddingBits = bits
			manifest.ChannelTables[channel].PaddingBits += bits
		}

		table := &manifest.ChannelTables[channel]
		table.Segments = append(table.Segments, segment)
		table.ValidBytes = actual
		if actual == validBytes {
			table.ValidSamples = samplesPerChannel
		} else {
			table.ValidSamples = actual * 8
		}
		table.PaddingBytes += segment.PaddingBytes
		table.MissingBytes += segment.MissingBytes
		firstSample[channel] += table.ValidSamples
	}

	physicalEnd := base
	if tailBytes > 0 && (tailMode == "block-padded" || tailMode == "block-padded-short") {
		physicalEnd = paddedEnd
	} else if tailBytes > 0 && (tailMode == "tail-packed" || tailMode == "tail-packed-short") {
		physicalEnd = tightEnd
	}
	if dataLength > physicalEnd {
		extra := dataLength - physicalEnd
		manifest.Padding = append(manifest.Padding, Padding{
			Kind:       PaddingTrailingData,
			Chunk:      0,
			Channel:    -1,
			FileOffset: dataStart + physicalEnd,
			Length:     extra,
			Reason:     "bytes after the declared channel blocks are not part of the sample table",
		})
	}
	return nil
}

func finalPackedByteOffset(
	dataStart, dataEnd, channel, channels, blockSize, fullBlocks, tailBytes, validBytes int64,
	segment Segment,
) (int64, error) {
	finalIndex := validBytes - 1
	blockIndex := finalIndex / blockSize
	within := finalIndex % blockSize
	if blockIndex < fullBlocks {
		offset := dataStart + (blockIndex*channels+channel)*blockSize + within
		if offset < dataStart || offset >= dataEnd {
			return 0, errors.New("final packed byte lies outside DSF data")
		}
		return offset, nil
	}
	if segment.TailValidBytes <= within {
		return 0, errors.New("final packed byte is missing from the DSF tail")
	}
	return segment.TailOffset + within, nil
}
