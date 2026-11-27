package wavpack

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// ChecksumStatus describes the optional WavPack 5 block checksum. This is a
// byte-level checksum of the block metadata, not the decoded-sample CRC stored
// in the header.
type ChecksumStatus struct {
	Present bool
	Valid   bool
	Stored  uint32
	Actual  uint32
	Error   string
}

func checkBlockChecksum(block []byte, metaOffset int, payload []byte) ChecksumStatus {
	status := ChecksumStatus{Present: true}
	start := metaOffset
	if start < 0 || start >= len(block) || start&1 != 0 {
		status.Error = "checksum metadata starts at an invalid offset"
		return status
	}
	if len(payload) != 2 && len(payload) != 4 {
		status.Error = fmt.Sprintf("checksum metadata has %d bytes, want 2 or 4", len(payload))
		return status
	}

	var sum uint32 = ^uint32(0)
	words := block[:start]
	for len(words) >= 2 {
		sum = sum*3 + uint32(binary.LittleEndian.Uint16(words[:2]))
		words = words[2:]
	}
	if len(words) != 0 {
		status.Error = "checksum input is not word aligned"
		return status
	}

	if len(payload) == 2 {
		sum ^= sum >> 16
		status.Stored = uint32(binary.LittleEndian.Uint16(payload))
		status.Actual = sum & 0xffff
	} else {
		status.Stored = binary.LittleEndian.Uint32(payload)
		status.Actual = sum
	}
	status.Valid = status.Stored == status.Actual
	if !status.Valid {
		status.Error = "block checksum mismatch"
	}
	return status
}

func checkTrailingMetadata(block []byte, end int, hasChecksumFlag bool, status ChecksumStatus) (string, ChecksumStatus) {
	if end != len(block) {
		return "garbage or truncated metadata at end of block", status
	}
	if hasChecksumFlag && !status.Present {
		return "HAS_CHECKSUM is set but no BLOCK_CHECKSUM metadata exists", status
	}
	return "", status
}

func findHeader(data []byte) int {
	return bytes.Index(data, []byte("wvpk"))
}
