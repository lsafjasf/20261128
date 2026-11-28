package mxfindex

import (
	"encoding/hex"
	"fmt"
	"strconv"
)

// Status is the final, externally visible result of a rebuild. A partial
// index must never be reported as complete.
type Status string

const (
	StatusComplete   Status = "complete"
	StatusIncomplete Status = "incomplete"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue describes a structural or coverage problem found while scanning.
// Errors prevent a complete result; warnings do not.
type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Offset   int64    `json:"offset,omitempty"`
}

// UID is a 16-byte SMPTE universal label.
type UID [16]byte

func (u UID) String() string {
	return hex.EncodeToString(u[:])
}

func (u UID) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *UID) UnmarshalText(text []byte) error {
	if len(text) != 32 {
		return fmt.Errorf("mxfindex: UID must contain 32 hex digits")
	}
	decoded, err := hex.DecodeString(string(text))
	if err != nil {
		return fmt.Errorf("mxfindex: invalid UID: %w", err)
	}
	copy(u[:], decoded)
	return nil
}

// Rational is an exact edit rate such as 25/1 or 30000/1001.
type Rational struct {
	Num int64 `json:"num"`
	Den int64 `json:"den"`
}

func (r Rational) Valid() bool {
	return r.Num > 0 && r.Den > 0
}

func (r Rational) String() string {
	if r.Den == 1 {
		return strconv.FormatInt(r.Num, 10)
	}
	return fmt.Sprintf("%d/%d", r.Num, r.Den)
}

// Partition is one parsed MXF partition pack. Offsets are absolute file
// offsets. Partition-relative values retain their on-disk representation.
type Partition struct {
	Index              int    `json:"index"`
	Offset             int64  `json:"offset"`
	Type               string `json:"type"`
	TypeCode           byte   `json:"type_code"`
	Closed             bool   `json:"closed"`
	Complete           bool   `json:"complete"`
	KAGSize            uint32 `json:"kag_size"`
	ThisPartition      uint64 `json:"this_partition"`
	PreviousPartition  uint64 `json:"previous_partition"`
	FooterPartition    uint64 `json:"footer_partition"`
	HeaderByteCount    uint64 `json:"header_byte_count"`
	IndexByteCount     uint64 `json:"index_byte_count"`
	IndexSID           uint32 `json:"index_sid"`
	BodyOffset         uint64 `json:"body_offset"`
	BodySID            uint32 `json:"body_sid"`
	OperationalPattern UID    `json:"operational_pattern"`
	EssenceContainers  []UID  `json:"essence_containers,omitempty"`

	// ContainerStart/ContainerEnd delimit the body container as observed in
	// the file. ContainerStart may point at a system item, not necessarily at
	// the first essence element.
	ContainerStart int64 `json:"container_start,omitempty"`
	ContainerEnd   int64 `json:"container_end,omitempty"`
}

type DeltaEntry struct {
	Position     int8   `json:"position"`
	Slice        int8   `json:"slice"`
	ElementDelta uint32 `json:"element_delta"`
}

type IndexEntry struct {
	TemporalOffset int8   `json:"temporal_offset"`
	KeyFrameOffset int8   `json:"key_frame_offset"`
	Flags          uint8  `json:"flags"`
	StreamOffset   uint64 `json:"stream_offset"`
	SliceOffset    uint32 `json:"slice_offset,omitempty"`
}

// IndexSegment mirrors an MXF Index Table Segment KLV.
type IndexSegment struct {
	Index             int          `json:"index"`
	Offset            int64        `json:"offset"`
	ValueLength       int64        `json:"value_length"`
	PartitionIndex    int          `json:"partition_index"`
	InstanceUID       UID          `json:"instance_uid"`
	EditRate          Rational     `json:"edit_rate"`
	StartPosition     uint64       `json:"start_position"`
	Duration          uint64       `json:"duration"`
	EditUnitByteCount uint32       `json:"edit_unit_byte_count"`
	IndexSID          uint32       `json:"index_sid"`
	BodySID           uint32       `json:"body_sid"`
	SliceCount        uint8        `json:"slice_count"`
	DeltaEntries      []DeltaEntry `json:"delta_entries,omitempty"`
	Entries           []IndexEntry `json:"entries,omitempty"`
	HasEntryArray     bool         `json:"has_entry_array"`
	Malformed         bool         `json:"malformed"`
}

// EssenceElement is a track or system item KLV observed in a body container.
type EssenceElement struct {
	KeyOffset      int64  `json:"key_offset"`
	ValueOffset    int64  `json:"value_offset"`
	Length         int64  `json:"length"`
	Track          uint32 `json:"track"`
	BodySID        uint32 `json:"body_sid"`
	PartitionIndex int    `json:"partition_index"`
	Kind           string `json:"kind"`
}

type RIPEntry struct {
	BodySID    uint32 `json:"body_sid"`
	ByteOffset uint64 `json:"byte_offset"`
}

type RandomIndexPack struct {
	Offset          int64      `json:"offset"`
	ValueLength     int64      `json:"value_length"`
	FooterPartition uint64     `json:"footer_partition"`
	TotalLength     uint32     `json:"total_length"`
	Entries         []RIPEntry `json:"entries,omitempty"`
	Valid           bool       `json:"valid"`
}

type TimecodeComponent struct {
	Offset      int64  `json:"offset"`
	InstanceUID UID    `json:"instance_uid"`
	StartFrame  int64  `json:"start_frame"`
	Rate        int    `json:"rate"`
	DropFrame   bool   `json:"drop_frame"`
	Timecode    string `json:"timecode"`
}

// RebuiltEntry maps one edit unit to a file offset. Offset is the position a
// player can seek to; Length is a conservative byte span for that edit unit.
type RebuiltEntry struct {
	EditUnit       uint64 `json:"edit_unit"`
	Offset         int64  `json:"offset"`
	Length         uint64 `json:"length"`
	Source         string `json:"source"`
	TemporalOffset int8   `json:"temporal_offset,omitempty"`
	KeyFrameOffset int8   `json:"key_frame_offset,omitempty"`
	Flags          uint8  `json:"flags,omitempty"`
	Timecode       string `json:"timecode,omitempty"`
}

type RebuiltTable struct {
	BodySID       uint32         `json:"body_sid"`
	IndexSID      uint32         `json:"index_sid"`
	EditRate      Rational       `json:"edit_rate"`
	StartPosition uint64         `json:"start_position"`
	EndPosition   uint64         `json:"end_position"`
	Complete      bool           `json:"complete"`
	Issues        []Issue        `json:"issues,omitempty"`
	Entries       []RebuiltEntry `json:"entries,omitempty"`
}

type RebuiltIndex struct {
	Complete bool           `json:"complete"`
	Tables   []RebuiltTable `json:"tables"`
}

// AddressableRange is a contiguous edit-unit interval whose entries all map
// to validated file offsets. Ranges are safe to expose to a random-access
// player; anything outside them must not be treated as seekable.
type AddressableRange struct {
	BodySID       uint32 `json:"body_sid"`
	IndexSID      uint32 `json:"index_sid"`
	StartEditUnit uint64 `json:"start_edit_unit"`
	EndEditUnit   uint64 `json:"end_edit_unit"`
	StartOffset   int64  `json:"start_offset"`
	EndOffset     int64  `json:"end_offset"`
	StartTimecode string `json:"start_timecode,omitempty"`
	EndTimecode   string `json:"end_timecode,omitempty"`
	EntryCount    int    `json:"entry_count"`
	Source        string `json:"source"`
	Complete      bool   `json:"complete"`
}

// Result is a complete scan and rebuild report.
type Result struct {
	Status            Status              `json:"status"`
	Success           bool                `json:"success"`
	FileSize          int64               `json:"file_size"`
	RunIn             int64               `json:"run_in"`
	Partitions        []Partition         `json:"partitions"`
	IndexSegments     []IndexSegment      `json:"index_segments"`
	Essence           []EssenceElement    `json:"essence_elements"`
	Timecodes         []TimecodeComponent `json:"timecodes"`
	RandomIndexPack   *RandomIndexPack    `json:"random_index_pack,omitempty"`
	AddressableRanges []AddressableRange  `json:"addressable_ranges"`
	RebuiltIndex      RebuiltIndex        `json:"rebuilt_index"`
	Issues            []Issue             `json:"issues"`
}

func (r *Result) addIssue(sev Severity, code, msg string, offset int64) {
	r.Issues = append(r.Issues, Issue{
		Code:     code,
		Severity: sev,
		Message:  msg,
		Offset:   offset,
	})
}

func (r *Result) addError(code, msg string, offset int64) {
	r.addIssue(SeverityError, code, msg, offset)
}

func (r *Result) addWarning(code, msg string, offset int64) {
	r.addIssue(SeverityWarning, code, msg, offset)
}

func (r *Result) finalize() {
	complete := true
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
			complete = false
			break
		}
	}
	if !r.RebuiltIndex.Complete {
		complete = false
	}
	if complete {
		r.Status = StatusComplete
		r.Success = true
		return
	}
	r.Status = StatusIncomplete
	r.Success = false
}
