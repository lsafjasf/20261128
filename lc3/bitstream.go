package lc3

import (
	"encoding/binary"
	"fmt"
)

// BitstreamHeader mirrors the 18-byte header of the Bluetooth LC3 reference
// codec bitstream format (see the LC3 Test Suite, "Reference LC3 Codec
// Bitstream Header"). All fields are little-endian.
type BitstreamHeader struct {
	// FileID is the constant 0xCC1C.
	FileID uint16
	// HeaderSize is the total configuration header length in bytes (18).
	HeaderSize uint16
	// SampleRate is stored as sampling frequency / 100.
	SampleRateHundreds uint16
	// Bitrate is stored as bitrate / 100.
	BitrateHundreds uint16
	Channels        uint16
	// FrameMS is stored as frame duration in ms * 100 (1000 or 750).
	FrameMSHundredths uint16
	Reserved          uint16
	// SignalLength is the original input length in PCM samples.
	SignalLength uint32
}

// Rate decodes the sample rate in Hz.
func (h BitstreamHeader) Rate() uint32 { return uint32(h.SampleRateHundreds) * 100 }

// Duration decodes the configured LC3 frame duration.
func (h BitstreamHeader) Duration() Duration {
	switch h.FrameMSHundredths {
	case 1000:
		return Dur10ms
	case 750:
		return Dur7p5ms
	default:
		return Duration(0)
	}
}

const bitstreamHeaderSize = 18

// ParseBitstreamHeader reads and validates the 18-byte reference header.
func ParseBitstreamHeader(data []byte) (BitstreamHeader, error) {
	if len(data) < bitstreamHeaderSize {
		return BitstreamHeader{}, fmt.Errorf("%w: need %d header bytes, got %d", ErrShortBuffer, bitstreamHeaderSize, len(data))
	}
	h := BitstreamHeader{
		FileID:             binary.LittleEndian.Uint16(data[0:2]),
		HeaderSize:         binary.LittleEndian.Uint16(data[2:4]),
		SampleRateHundreds: binary.LittleEndian.Uint16(data[4:6]),
		BitrateHundreds:    binary.LittleEndian.Uint16(data[6:8]),
		Channels:           binary.LittleEndian.Uint16(data[8:10]),
		FrameMSHundredths:  binary.LittleEndian.Uint16(data[10:12]),
		Reserved:           binary.LittleEndian.Uint16(data[12:14]),
		SignalLength:       binary.LittleEndian.Uint32(data[14:18]),
	}
	if h.FileID != 0xCC1C {
		return BitstreamHeader{}, fmt.Errorf("lc3: bad file id 0x%04X (want 0xCC1C)", h.FileID)
	}
	if !h.Duration().Valid() {
		return BitstreamHeader{}, fmt.Errorf("lc3: header frame_ms %d unsupported", h.FrameMSHundredths)
	}
	if !SupportedRate(h.Rate()) {
		return BitstreamHeader{}, fmt.Errorf("lc3: header sample rate %d unsupported", h.Rate())
	}
	return h, nil
}

// BitstreamFrame is one length-prefixed encoded frame in a reference bitstream.
type BitstreamFrame struct {
	Octets    int
	Payload   []byte
	Truncated bool
}

// ParseBitstreamFrames walks every "uint16 little-endian size + payload"
// record after the header. A final record whose payload runs past the end of
// the buffer is returned with Truncated set instead of being silently dropped,
// which models a short tail frame produced by an interrupted capture.
func ParseBitstreamFrames(data []byte) ([]BitstreamFrame, error) {
	if _, err := ParseBitstreamHeader(data); err != nil {
		return nil, err
	}
	pos := bitstreamHeaderSize
	var out []BitstreamFrame
	for pos < len(data) {
		if pos+2 > len(data) {
			return out, fmt.Errorf("%w: %d byte(s) left, need 2 for size", ErrShortBuffer, len(data)-pos)
		}
		size := int(binary.LittleEndian.Uint16(data[pos : pos+2]))
		pos += 2
		if size == 0 {
			out = append(out, BitstreamFrame{Octets: 0})
			continue
		}
		avail := len(data) - pos
		if avail >= size {
			out = append(out, BitstreamFrame{Octets: size, Payload: data[pos : pos+size : pos+size]})
			pos += size
			continue
		}
		// Short tail frame: keep the partial payload and stop scanning.
		out = append(out, BitstreamFrame{
			Octets:    size,
			Payload:   data[pos:len(data):len(data)],
			Truncated: true,
		})
		break
	}
	return out, nil
}
