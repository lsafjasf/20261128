package mxfindex

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// ScanFile scans an MXF file, rebuilds its edit-unit index and returns a
// complete or explicitly incomplete report. Read errors are returned as
// errors; recoverable structural problems are reported in Result.Issues.
func ScanFile(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("mxfindex: open %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("mxfindex: stat %s: %w", path, err)
	}
	if info.Size() < 0 {
		return nil, fmt.Errorf("mxfindex: invalid file size for %s", path)
	}
	return Scan(f, info.Size())
}

// RebuildIndexFile is an explicit alias for ScanFile. The scan always rebuilds
// the edit-unit index; it never reports a partial rebuild as successful.
func RebuildIndexFile(path string) (*Result, error) {
	return ScanFile(path)
}

// RebuildIndex is an explicit alias for Scan for callers that already have an
// io.ReaderAt and the exact file size.
func RebuildIndex(r io.ReaderAt, size int64) (*Result, error) {
	return Scan(r, size)
}

// Scan examines the KLV stream and returns an index rebuild report.
func Scan(r io.ReaderAt, size int64) (*Result, error) {
	if size < 0 {
		return nil, fmt.Errorf("mxfindex: negative file size")
	}
	runIn, err := findRunIn(r, size)
	if err != nil {
		return nil, err
	}
	result := &Result{
		FileSize:          size,
		RunIn:             runIn,
		Partitions:        make([]Partition, 0, 16),
		IndexSegments:     make([]IndexSegment, 0, 16),
		Essence:           make([]EssenceElement, 0, 256),
		Timecodes:         make([]TimecodeComponent, 0, 4),
		AddressableRanges: make([]AddressableRange, 0),
		Issues:            make([]Issue, 0),
	}
	currentPartition := -1
	headerSeen := false
	ripped := false

	for pos := runIn; pos < size; {
		packet, err := readKLVAt(r, size, pos)
		if err != nil {
			result.addError("klv.invalid", err.Error(), pos)
			break
		}
		if !bytes.Equal(packet.Key[:4], []byte{0x06, 0x0e, 0x2b, 0x34}) {
			result.addError("klv.key", "KLV key does not start with 06.0e.2b.34", pos)
			break
		}
		switch {
		case isPartition(packet.Key):
			if currentPartition >= 0 {
				result.Partitions[currentPartition].ContainerEnd = packet.Offset
			}
			part, err := parsePartition(r, packet, len(result.Partitions), runIn)
			result.Partitions = append(result.Partitions, part)
			currentPartition = len(result.Partitions) - 1
			if err != nil {
				result.addError("partition.invalid", err.Error(), packet.Offset)
			}
			if part.Type == "header" {
				headerSeen = true
			}
		case equalUID(packet.Key, indexSegmentKey):
			seg, err := parseIndexSegment(r, packet, currentPartition, len(result.IndexSegments))
			result.IndexSegments = append(result.IndexSegments, seg)
			if err != nil {
				result.addError("index.malformed", err.Error(), packet.Offset)
			}
		case equalUID(packet.Key, timecodeKey):
			tc, err := parseTimecodeComponent(r, packet)
			if err != nil {
				result.addError("timecode.malformed", err.Error(), packet.Offset)
			} else {
				result.Timecodes = append(result.Timecodes, tc)
			}
		case equalUID(packet.Key, randomIndexPackKey):
			rip, err := parseRandomIndexPack(r, packet)
			if err != nil {
				result.addError("rip.invalid", err.Error(), packet.Offset)
			}
			result.RandomIndexPack = &rip
			ripped = true
		case elementKindMatch(packet.Key):
			kind, track := classifyElement(packet.Key)
			var bodySID uint32
			if currentPartition >= 0 {
				bodySID = result.Partitions[currentPartition].BodySID
				if result.Partitions[currentPartition].ContainerStart == 0 {
					result.Partitions[currentPartition].ContainerStart = packet.Offset
				}
			}
			if bodySID == 0 {
				result.addError("essence.no_body_sid", "element found outside a partition with BodySID", packet.Offset)
			}
			result.Essence = append(result.Essence, EssenceElement{
				KeyOffset:      packet.Offset,
				ValueOffset:    packet.ValueOffset,
				Length:         packet.Length,
				Track:          track,
				BodySID:        bodySID,
				PartitionIndex: currentPartition,
				Kind:           kind,
			})
		}
		if ripped {
			if packet.End < size {
				result.addWarning("rip.trailing", "data follows the Random Index Pack", packet.End)
			}
			break
		}
		if packet.End <= pos {
			result.addError("klv.progress", "KLV does not advance file position", pos)
			break
		}
		pos = packet.End
	}

	if !headerSeen {
		result.addError("partition.header_missing", "header partition pack not found", runIn)
	}
	if currentPartition >= 0 {
		result.Partitions[currentPartition].ContainerEnd = size
	}
	validatePartitions(result)
	validateTimecodes(result)
	rebuild(result)
	result.finalize()
	return result, nil
}

func isPartition(key UID) bool {
	_, ok := isPartitionKey(key)
	return ok
}

func equalUID(a, b UID) bool {
	return bytes.Equal(a[:], b[:])
}

func elementKindMatch(key UID) bool {
	_, _, ok := elementKind(key)
	return ok
}

func classifyElement(key UID) (string, uint32) {
	kind, track, _ := elementKind(key)
	return kind, track
}

func validatePartitions(result *Result) {
	if len(result.Partitions) == 0 {
		result.addError("partition.none", "no partition packs found", result.RunIn)
		return
	}
	first := result.Partitions[0]
	if first.Type != "header" {
		result.addError("partition.first", "first partition is not a header", first.Offset)
	}
	footer := findFooter(result.Partitions)
	if footer < 0 {
		result.addError("partition.footer_missing", "footer partition pack not found", 0)
	}
	actualFooter := uint64(0)
	if footer >= 0 {
		actualFooter = uint64(result.Partitions[footer].Offset - result.RunIn)
	}
	for i := range result.Partitions {
		part := &result.Partitions[i]
		expectedThis := uint64(part.Offset - result.RunIn)
		if part.ThisPartition != expectedThis {
			result.addError("partition.this_mismatch",
				fmt.Sprintf("partition %d declares ThisPartition=%d, actual=%d", i, part.ThisPartition, expectedThis),
				part.Offset)
		}
		expectedPrevious := uint64(0)
		if i > 0 {
			expectedPrevious = uint64(result.Partitions[i-1].Offset - result.RunIn)
		}
		if part.PreviousPartition != expectedPrevious {
			result.addError("partition.previous_mismatch",
				fmt.Sprintf("partition %d declares PreviousPartition=%d, actual=%d", i, part.PreviousPartition, expectedPrevious),
				part.Offset)
		}
		if part.FooterPartition != 0 && actualFooter != 0 && part.FooterPartition != actualFooter {
			result.addError("partition.footer_mismatch",
				fmt.Sprintf("partition %d points at footer %d, actual=%d", i, part.FooterPartition, actualFooter),
				part.Offset)
		}
		if part.FooterPartition == 0 && i == 0 {
			result.addWarning("partition.footer_unset", "header does not declare a footer partition", part.Offset)
		}
	}
	if result.RandomIndexPack != nil {
		validateRIP(result)
	} else {
		result.addWarning("rip.missing", "no Random Index Pack found", 0)
	}
}

func findFooter(parts []Partition) int {
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i].Type == "footer" {
			return i
		}
	}
	return -1
}

func validateRIP(result *Result) {
	rip := result.RandomIndexPack
	if !rip.Valid {
		return
	}
	if rip.FooterPartition == 0 {
		result.addWarning("rip.footer", "RIP has no footer partition entry", rip.Offset)
	}
	if len(rip.Entries) == 0 {
		result.addWarning("rip.empty", "RIP has no partition entries", rip.Offset)
		return
	}
	for _, entry := range rip.Entries {
		found := false
		for _, part := range result.Partitions {
			if uint64(part.Offset-result.RunIn) == entry.ByteOffset {
				found = true
				break
			}
		}
		if !found {
			result.addError("rip.partition_missing",
				fmt.Sprintf("RIP byte offset %d does not identify a scanned partition", entry.ByteOffset),
				rip.Offset)
		}
	}
}

func validateTimecodes(result *Result) {
	for i, tc := range result.Timecodes {
		if tc.Rate <= 0 {
			result.addWarning("timecode.rate", fmt.Sprintf("timecode component %d has no valid rate", i), tc.Offset)
		}
		if tc.StartFrame < 0 {
			result.addWarning("timecode.start", fmt.Sprintf("timecode component %d starts before zero", i), tc.Offset)
		}
	}
}
