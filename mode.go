package amrwb

import "fmt"

const (
	// SampleRate is the AMR-WB sampling rate in samples per second.
	SampleRate = 16000
	// FrameDurationMS is the duration of one AMR-WB frame.
	FrameDurationMS = 20
	// SamplesPerFrame is fixed for every AMR-WB mode: 16000 * 0.020.
	// The encoded frame size changes with the mode; the sample duration
	// does not.
	SamplesPerFrame = SampleRate * FrameDurationMS / 1000
)

// Mode is the four-bit frame type (FT) from an AMR-WB frame header.
type Mode uint8

const (
	Mode660  Mode = 0
	Mode885  Mode = 1
	Mode1265 Mode = 2
	Mode1425 Mode = 3
	Mode1585 Mode = 4
	Mode1825 Mode = 5
	Mode1985 Mode = 6
	Mode2305 Mode = 7
	Mode2385 Mode = 8
	ModeSID  Mode = 9
	// ModeSpeechLost and ModeNoData carry no encoded payload. In the
	// storage format they still delimit one 20 ms frame-block.
	ModeSpeechLost Mode = 14
	ModeNoData     Mode = 15
)

type frameSizeEntry struct {
	octets      int
	coreBits    int
	description string
}

var frameSizes = map[Mode]frameSizeEntry{
	Mode660:        {18, 132, "6.60 kbit/s"},
	Mode885:        {24, 177, "8.85 kbit/s"},
	Mode1265:       {33, 253, "12.65 kbit/s"},
	Mode1425:       {37, 285, "14.25 kbit/s"},
	Mode1585:       {41, 317, "15.85 kbit/s"},
	Mode1825:       {47, 365, "18.25 kbit/s"},
	Mode1985:       {51, 397, "19.85 kbit/s"},
	Mode2305:       {59, 461, "23.05 kbit/s"},
	Mode2385:       {61, 477, "23.85 kbit/s"},
	ModeSID:        {6, 40, "SID"},
	ModeSpeechLost: {1, 0, "SPEECH_LOST"},
	ModeNoData:     {1, 0, "NO_DATA"},
}

// Valid reports whether m is a defined AMR-WB frame type. Modes 10-13 are
// reserved by the AMR-WB frame structure.
func (m Mode) Valid() bool {
	_, ok := frameSizes[m]
	return ok
}

// IsSpeech reports whether m is an AMR-WB speech coding mode.
func (m Mode) IsSpeech() bool {
	return m >= Mode660 && m <= Mode2385
}

// FrameOctets returns the octet-aligned storage/RTP frame size, including
// the one-octet frame header. It returns 0 for an undefined mode.
func (m Mode) FrameOctets() int {
	return frameSizes[m].octets
}

// PayloadOctets returns the encoded payload size excluding the one-octet
// frame header. It returns 0 for undefined modes.
func (m Mode) PayloadOctets() int {
	if !m.Valid() {
		return 0
	}
	return m.FrameOctets() - 1
}

// CoreBits returns the number of encoded core bits. It returns 0 for
// non-payload frame types and undefined modes.
func (m Mode) CoreBits() int {
	return frameSizes[m].coreBits
}

// String returns the 3GPP name of the mode.
func (m Mode) String() string {
	entry, ok := frameSizes[m]
	if !ok {
		return fmt.Sprintf("mode(%d)", uint8(m))
	}
	return fmt.Sprintf("%s (FT=%d)", entry.description, uint8(m))
}

// FrameQuality is the inverse of the AMR-WB Q bit.
type FrameQuality uint8

const (
	QualityGood FrameQuality = iota
	QualityBad
)

// Header is the decoded first octet of an octet-aligned AMR-WB frame.
type Header struct {
	Mode    Mode
	Quality FrameQuality
}

// ParseHeader decodes the AMR-WB storage/RTP ToC header octet. The F bit in
// an RTP ToC is ignored; the stored header uses that bit as padding.
func ParseHeader(b byte) Header {
	quality := QualityBad
	if b&0x04 != 0 {
		quality = QualityGood
	}
	return Header{
		Mode:    Mode((b >> 3) & 0x0f),
		Quality: quality,
	}
}

// Bad reports whether the frame quality bit is zero.
func (h Header) Bad() bool {
	return h.Quality == QualityBad
}

// FrameOctets returns the expected storage/RTP frame size for the header's
// mode, including the header octet. An undefined mode returns 0.
func (h Header) FrameOctets() int {
	return h.Mode.FrameOctets()
}
