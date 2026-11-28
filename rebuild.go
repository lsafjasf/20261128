package mxfindex

import (
	"fmt"
	"sort"
)

func rebuild(result *Result) {
	result.RebuiltIndex = RebuiltIndex{
		Complete: true,
		Tables:   make([]RebuiltTable, 0),
	}
	if len(result.IndexSegments) == 0 {
		result.addIssue(SeverityError, "index.missing",
			"no Index Table Segments were found", 0)
		result.RebuiltIndex.Complete = false
		result.AddressableRanges = append(result.AddressableRanges, essenceScanRanges(result)...)
		return
	}
	groups := groupSegments(result.IndexSegments)
	if len(groups) == 0 {
		result.RebuiltIndex.Complete = false
		return
	}
	for _, group := range groups {
		table := buildTable(result, group)
		result.RebuiltIndex.Tables = append(result.RebuiltIndex.Tables, table)
		if !table.Complete {
			result.RebuiltIndex.Complete = false
		}
		result.AddressableRanges = append(result.AddressableRanges, rangesForTable(table)...)
	}
	if !validateBodyCoverage(result) {
		result.RebuiltIndex.Complete = false
	}
	if len(result.AddressableRanges) == 0 {
		result.AddressableRanges = append(result.AddressableRanges, essenceScanRanges(result)...)
	}
	sort.SliceStable(result.AddressableRanges, func(i, j int) bool {
		a, b := result.AddressableRanges[i], result.AddressableRanges[j]
		if a.BodySID != b.BodySID {
			return a.BodySID < b.BodySID
		}
		if a.IndexSID != b.IndexSID {
			return a.IndexSID < b.IndexSID
		}
		if a.StartEditUnit != b.StartEditUnit {
			return a.StartEditUnit < b.StartEditUnit
		}
		return a.StartOffset < b.StartOffset
	})
	if !result.RebuiltIndex.Complete && len(result.IndexSegments) > 0 {
		result.addIssue(SeverityError, "index.incomplete",
			"rebuilt index does not cover the complete edit-unit timeline", 0)
	}
}

func validateBodyCoverage(result *Result) bool {
	indexed := make(map[uint32]bool)
	for _, table := range result.RebuiltIndex.Tables {
		indexed[table.BodySID] = true
	}
	ok := true
	for i := range result.Partitions {
		part := &result.Partitions[i]
		if part.BodySID == 0 || part.ContainerStart <= 0 {
			continue
		}
		if !indexed[part.BodySID] {
			result.addIssue(SeverityError, "index.body_unreferenced",
				fmt.Sprintf("BodySID %d has essence but no index table", part.BodySID), part.Offset)
			ok = false
		}
	}
	return ok
}

func essenceScanRanges(result *Result) []AddressableRange {
	type key struct {
		bodySID uint32
		track   uint32
	}
	groups := make(map[key][]EssenceElement)
	order := make([]key, 0)
	for _, el := range result.Essence {
		if el.Kind != "essence" || el.BodySID == 0 {
			continue
		}
		k := key{bodySID: el.BodySID, track: el.Track}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], el)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].bodySID != order[j].bodySID {
			return order[i].bodySID < order[j].bodySID
		}
		return order[i].track < order[j].track
	})
	ranges := make([]AddressableRange, 0, len(order))
	for _, k := range order {
		els := groups[k]
		sort.SliceStable(els, func(i, j int) bool {
			return els[i].KeyOffset < els[j].KeyOffset
		})
		first, last := els[0], els[len(els)-1]
		rng := AddressableRange{
			BodySID:       k.bodySID,
			StartEditUnit: 0,
			EndEditUnit:   uint64(len(els)),
			StartOffset:   first.KeyOffset,
			EndOffset:     last.ValueOffset + last.Length,
			EntryCount:    len(els),
			Source:        "essence_scan",
			Complete:      false,
		}
		if tc, ok := chooseTimecode(result, Rational{}); ok {
			rng.StartTimecode = FrameToTimecode(tc.StartFrame, tc.Rate, tc.DropFrame)
			rng.EndTimecode = FrameToTimecode(tc.StartFrame+int64(len(els))-1, tc.Rate, tc.DropFrame)
		}
		ranges = append(ranges, rng)
	}
	return ranges
}

func groupSegments(segments []IndexSegment) [][]int {
	bySID := make(map[uint32][]int)
	order := make([]uint32, 0)
	for i := range segments {
		sid := segments[i].IndexSID
		if _, ok := bySID[sid]; !ok {
			order = append(order, sid)
		}
		bySID[sid] = append(bySID[sid], i)
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i] < order[j] })
	groups := make([][]int, 0, len(order))
	for _, sid := range order {
		group := bySID[sid]
		sort.SliceStable(group, func(i, j int) bool {
			a, b := segments[group[i]], segments[group[j]]
			if a.StartPosition != b.StartPosition {
				return a.StartPosition < b.StartPosition
			}
			return a.Offset < b.Offset
		})
		groups = append(groups, group)
	}
	return groups
}

func buildTable(result *Result, group []int) RebuiltTable {
	segments := result.IndexSegments
	first := segments[group[0]]
	table := RebuiltTable{
		BodySID:       first.BodySID,
		IndexSID:      first.IndexSID,
		EditRate:      first.EditRate,
		StartPosition: first.StartPosition,
		Complete:      true,
		Issues:        make([]Issue, 0),
	}
	if table.IndexSID == 0 {
		tableIssue(result, &table, SeverityError, "index.sid", "index segment has IndexSID 0", first.Offset)
	}
	if table.BodySID == 0 {
		tableIssue(result, &table, SeverityError, "index.body_sid", "index segment has BodySID 0", first.Offset)
	}
	if !table.EditRate.Valid() {
		tableIssue(result, &table, SeverityError, "index.edit_rate", "index segment has invalid edit rate", first.Offset)
	}
	for _, idx := range group[1:] {
		seg := segments[idx]
		if seg.BodySID != table.BodySID {
			tableIssue(result, &table, SeverityError, "index.body_sid_conflict",
				fmt.Sprintf("IndexSID %d mixes BodySID %d and %d",
					table.IndexSID, table.BodySID, seg.BodySID), seg.Offset)
		}
		if seg.EditRate.Valid() && table.EditRate.Valid() && seg.EditRate != table.EditRate {
			tableIssue(result, &table, SeverityError, "index.edit_rate_conflict",
				fmt.Sprintf("IndexSID %d mixes edit rates %s and %s",
					table.IndexSID, table.EditRate, seg.EditRate), seg.Offset)
		}
	}
	hasEssence := bodyHasContainer(result, table.BodySID)
	if !hasEssence {
		tableIssue(result, &table, SeverityError, "index.body_missing",
			fmt.Sprintf("BodySID %d has no scanned essence container", table.BodySID), first.Offset)
	}
	if table.StartPosition != 0 {
		tableIssue(result, &table, SeverityError, "index.start_gap",
			fmt.Sprintf("IndexSID %d starts at edit unit %d", table.IndexSID, table.StartPosition), first.Offset)
	}

	cumulative := uint64(0)
	var expectedStart uint64 = 0
	for _, idx := range group {
		seg := segments[idx]
		duration := effectiveDuration(result, &table, seg)
		if duration == 0 {
			tableIssue(result, &table, SeverityError, "index.duration",
				fmt.Sprintf("index segment at %d has no usable duration", seg.Offset), seg.Offset)
			table.Complete = false
			continue
		}
		if seg.StartPosition != expectedStart {
			code := "index.overlap"
			if seg.StartPosition > expectedStart {
				code = "index.gap"
			}
			tableIssue(result, &table, SeverityError, code,
				fmt.Sprintf("IndexSID %d expected edit unit %d, segment starts at %d",
					table.IndexSID, expectedStart, seg.StartPosition), seg.Offset)
		}
		expectedStart = seg.StartPosition + duration
		if expectedStart > table.EndPosition {
			table.EndPosition = expectedStart
		}
		completeEntries := mapSegmentEntries(result, &table, seg, duration, cumulative)
		if !completeEntries {
			table.Complete = false
		}
		if seg.EditUnitByteCount > 0 && duration > 0 {
			if uint64(seg.EditUnitByteCount) > ^uint64(0)/duration {
				tableIssue(result, &table, SeverityError, "index.byte_count_overflow",
					"EditUnitByteCount overflows the cumulative container offset", seg.Offset)
			} else {
				cumulative += uint64(seg.EditUnitByteCount) * duration
			}
		}
	}
	if table.EndPosition == 0 {
		table.Complete = false
	} else if table.StartPosition != 0 || table.EndPosition == 0 {
		table.Complete = false
	}
	if !table.EntriesCover(result) {
		table.Complete = false
	}
	if !validateBodyTail(result, &table) {
		table.Complete = false
	}
	table.Complete = table.Complete && table.StartPosition == 0 && table.EndPosition > 0
	if !table.Complete {
		result.RebuiltIndex.Complete = false
	}
	return table
}

func validateBodyTail(result *Result, table *RebuiltTable) bool {
	counts := make(map[uint32]int)
	var firstElement int64 = -1
	for _, el := range result.Essence {
		if el.BodySID != table.BodySID || el.Kind != "essence" {
			continue
		}
		if counts[el.Track] == 0 {
			firstElement = el.KeyOffset
		}
		counts[el.Track]++
	}
	if len(counts) == 0 || table.EndPosition == 0 {
		return true
	}
	minCount := int(^uint(0) >> 1)
	for _, count := range counts {
		if count < minCount {
			minCount = count
		}
	}
	if uint64(minCount) > table.EndPosition {
		tableIssue(result, table, SeverityError, "index.essence_unindexed",
			fmt.Sprintf("BodySID %d contains at least %d elements per track but index ends at %d",
				table.BodySID, minCount, table.EndPosition), firstElement)
		return false
	}
	return true
}

func effectiveDuration(result *Result, table *RebuiltTable, seg IndexSegment) uint64 {
	if seg.Duration > 0 {
		return seg.Duration
	}
	if seg.HasEntryArray && len(seg.Entries) > 0 {
		count := uint64(len(seg.Entries))
		if count%2 == 1 && count >= 3 {
			// Avid writes two entries per edit unit plus a sentinel. This
			// heuristic is only used when IndexDuration was omitted.
			return (count - 1) / 2
		}
		return count
	}
	if seg.EditUnitByteCount == 0 {
		return 0
	}
	span := bodyContainerSpan(result, table.BodySID)
	if span <= 0 {
		return 0
	}
	return uint64(span) / uint64(seg.EditUnitByteCount)
}

func bodyContainerSpan(result *Result, bodySID uint32) int64 {
	var span int64
	for i := range result.Partitions {
		part := &result.Partitions[i]
		if part.BodySID != bodySID || part.ContainerStart <= 0 || part.ContainerEnd <= part.ContainerStart {
			continue
		}
		// BodyOffset is a position in the logical body container. If it is
		// non-zero, the partition's ContainerStart is not logical offset zero.
		if part.BodyOffset == 0 || span == 0 {
			if length := part.ContainerEnd - part.ContainerStart; length > span {
				span = length
			}
		}
	}
	for _, el := range result.Essence {
		if el.BodySID == bodySID && el.Kind == "essence" && el.Length > span {
			span = el.Length
		}
	}
	return span
}

func bodyHasContainer(result *Result, bodySID uint32) bool {
	for i := range result.Partitions {
		part := &result.Partitions[i]
		if part.BodySID == bodySID && part.ContainerStart > 0 {
			return true
		}
	}
	return false
}

func mapSegmentEntries(result *Result, table *RebuiltTable, seg IndexSegment, duration, cumulative uint64) bool {
	complete := true
	entryStep := uint64(1)
	if seg.HasEntryArray && duration > 0 {
		switch {
		case uint64(len(seg.Entries)) == duration:
		case uint64(len(seg.Entries)) == 2*duration+1:
			entryStep = 2
		case uint64(len(seg.Entries)) < duration:
			tableIssue(result, table, SeverityError, "index.entries_short",
				fmt.Sprintf("segment at %d has %d entries for duration %d",
					seg.Offset, len(seg.Entries), duration), seg.Offset)
			complete = false
		default:
			tableIssue(result, table, SeverityWarning, "index.entries_extra",
				fmt.Sprintf("segment at %d has %d entries for duration %d",
					seg.Offset, len(seg.Entries), duration), seg.Offset)
		}
	} else if !seg.HasEntryArray && seg.EditUnitByteCount == 0 {
		tableIssue(result, table, SeverityError, "index.entries_missing",
			fmt.Sprintf("segment at %d has neither IndexEntryArray nor EditUnitByteCount", seg.Offset),
			seg.Offset)
		return false
	}

	baseTC, hasTC := chooseTimecode(result, table.EditRate)
	for unit := uint64(0); unit < duration; unit++ {
		var (
			rel      uint64
			temporal int8
			keyOff   int8
			flags    uint8
			ok       = true
		)
		if seg.HasEntryArray {
			index := int(unit)
			if entryStep == 2 {
				index = int(unit * 2)
			}
			if index < 0 || index >= len(seg.Entries) {
				ok = false
			} else {
				entry := seg.Entries[index]
				rel = entry.StreamOffset
				temporal = entry.TemporalOffset
				keyOff = entry.KeyFrameOffset
				flags = entry.Flags
			}
		} else {
			rel = cumulative + unit*uint64(seg.EditUnitByteCount)
		}
		if !ok {
			tableIssue(result, table, SeverityError, "index.entry_missing",
				fmt.Sprintf("no index entry for edit unit %d", seg.StartPosition+unit), seg.Offset)
			complete = false
			continue
		}
		abs, length, err := mapBodyOffset(result, table.BodySID, rel)
		if err != nil {
			tableIssue(result, table, SeverityError, "index.offset_unmapped",
				fmt.Sprintf("edit unit %d offset %d: %v", seg.StartPosition+unit, rel, err),
				seg.Offset)
			complete = false
			continue
		}
		source := "index"
		if !seg.HasEntryArray {
			source = "index"
		}
		entry := RebuiltEntry{
			EditUnit:       seg.StartPosition + unit,
			Offset:         abs,
			Length:         length,
			Source:         source,
			TemporalOffset: temporal,
			KeyFrameOffset: keyOff,
			Flags:          flags,
		}
		if hasTC {
			entry.Timecode = timecodeAt(baseTC, table.EditRate, entry.EditUnit)
		}
		table.Entries = append(table.Entries, entry)
	}
	return complete
}

func tableIssue(result *Result, table *RebuiltTable, sev Severity, code, msg string, offset int64) {
	issue := Issue{Code: code, Severity: sev, Message: msg, Offset: offset}
	table.Issues = append(table.Issues, issue)
	result.Issues = append(result.Issues, issue)
}

func chooseTimecode(result *Result, rate Rational) (TimecodeComponent, bool) {
	for _, tc := range result.Timecodes {
		if tc.Rate <= 0 {
			continue
		}
		if rate.Valid() {
			rounded := (rate.Num + rate.Den/2) / rate.Den
			if rounded == int64(tc.Rate) {
				return tc, true
			}
		}
	}
	for _, tc := range result.Timecodes {
		if tc.Rate > 0 {
			return tc, true
		}
	}
	return TimecodeComponent{}, false
}

func mapBodyOffset(result *Result, bodySID uint32, relative uint64) (int64, uint64, error) {
	var (
		best     *Partition
		bestSpan int64
	)
	for i := range result.Partitions {
		part := &result.Partitions[i]
		if part.BodySID != bodySID || part.ContainerStart <= 0 {
			continue
		}
		span := part.ContainerEnd - part.ContainerStart
		if span <= 0 || relative < part.BodyOffset {
			continue
		}
		delta := relative - part.BodyOffset
		if delta >= uint64(span) {
			continue
		}
		if best == nil || part.BodyOffset >= best.BodyOffset {
			best = part
			bestSpan = span
		}
	}
	if best == nil {
		return 0, 0, fmt.Errorf("BodySID %d has no containing partition for offset %d", bodySID, relative)
	}
	delta := relative - best.BodyOffset
	candidate := best.ContainerStart + int64(delta)
	if candidate < best.ContainerStart || candidate >= best.ContainerStart+bestSpan {
		return 0, 0, fmt.Errorf("offset %d maps outside BodySID %d container", relative, bodySID)
	}
	tolerance := int64(best.KAGSize) * 2
	if tolerance < 4096 {
		tolerance = 4096
	}
	var next *EssenceElement
	for i := range result.Essence {
		el := &result.Essence[i]
		if el.BodySID != bodySID {
			continue
		}
		elementEnd := el.ValueOffset + el.Length
		if candidate >= el.KeyOffset && candidate <= elementEnd {
			offset := candidate
			if offset < el.KeyOffset {
				offset = el.KeyOffset
			}
			if offset >= elementEnd {
				continue
			}
			return offset, uint64(elementEnd - offset), nil
		}
		if el.KeyOffset > candidate && (next == nil || el.KeyOffset < next.KeyOffset) {
			next = el
		}
	}
	if next != nil && next.KeyOffset-candidate <= tolerance {
		return next.KeyOffset, uint64(next.ValueOffset + next.Length - next.KeyOffset), nil
	}
	return 0, 0, fmt.Errorf("offset %d maps to %d, which is not inside a contained element", relative, candidate)
}

func (table *RebuiltTable) EntriesCover(result *Result) bool {
	if len(table.Entries) == 0 {
		tableIssue(result, table, SeverityError, "index.empty",
			fmt.Sprintf("IndexSID %d has no mapped entries", table.IndexSID), 0)
		return false
	}
	sort.SliceStable(table.Entries, func(i, j int) bool {
		return table.Entries[i].EditUnit < table.Entries[j].EditUnit
	})
	expected := table.StartPosition
	lastOffset := int64(-1)
	for _, entry := range table.Entries {
		if entry.EditUnit < expected {
			tableIssue(result, table, SeverityError, "index.duplicate",
				fmt.Sprintf("edit unit %d is mapped more than once", entry.EditUnit), entry.Offset)
			return false
		}
		if entry.EditUnit > expected {
			tableIssue(result, table, SeverityError, "index.coverage_gap",
				fmt.Sprintf("IndexSID %d has no mapping for edit unit %d", table.IndexSID, expected),
				entry.Offset)
			return false
		}
		if entry.Offset == lastOffset {
			tableIssue(result, table, SeverityError, "index.offset_duplicate",
				fmt.Sprintf("edit units map to the same file offset %d", entry.Offset), entry.Offset)
			return false
		}
		if entry.Offset < lastOffset {
			tableIssue(result, table, SeverityError, "index.offset_order",
				fmt.Sprintf("edit unit %d maps backwards to offset %d", entry.EditUnit, entry.Offset),
				entry.Offset)
			return false
		}
		lastOffset = entry.Offset
		expected++
	}
	if expected != table.EndPosition {
		tableIssue(result, table, SeverityError, "index.coverage_short",
			fmt.Sprintf("IndexSID %d covers through %d, expected %d", table.IndexSID, expected, table.EndPosition),
			0)
		return false
	}
	fillEntryLengths(result, table)
	return true
}

func fillEntryLengths(result *Result, table *RebuiltTable) {
	bodyEnd := bodyContainerEnd(result, table.BodySID)
	for i := range table.Entries {
		entry := &table.Entries[i]
		var end int64
		if i+1 < len(table.Entries) {
			end = table.Entries[i+1].Offset
		} else {
			end = bodyEnd
		}
		if end > entry.Offset {
			entry.Length = uint64(end - entry.Offset)
		}
	}
}

func bodyContainerEnd(result *Result, bodySID uint32) int64 {
	for i := len(result.Partitions) - 1; i >= 0; i-- {
		part := &result.Partitions[i]
		if part.BodySID == bodySID && part.ContainerStart > 0 && part.ContainerEnd > part.ContainerStart {
			return part.ContainerEnd
		}
	}
	return 0
}

func rangesForTable(table RebuiltTable) []AddressableRange {
	if len(table.Entries) == 0 {
		return nil
	}
	entries := table.Entries
	ranges := make([]AddressableRange, 0)
	start := 0
	for i := 1; i <= len(entries); i++ {
		if i < len(entries) && entries[i].EditUnit == entries[i-1].EditUnit+1 {
			continue
		}
		run := entries[start:i]
		rng := AddressableRange{
			BodySID:       table.BodySID,
			IndexSID:      table.IndexSID,
			StartEditUnit: run[0].EditUnit,
			EndEditUnit:   run[len(run)-1].EditUnit + 1,
			StartOffset:   run[0].Offset,
			EndOffset:     run[len(run)-1].Offset + int64(run[len(run)-1].Length),
			StartTimecode: run[0].Timecode,
			EndTimecode:   run[len(run)-1].Timecode,
			EntryCount:    len(run),
			Source:        "index",
			Complete:      table.Complete,
		}
		ranges = append(ranges, rng)
		start = i
	}
	return ranges
}
