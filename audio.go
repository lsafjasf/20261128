package st2110

import "fmt"

// Encoding is the ST 2110-30 payload coding (linear PCM, big endian).
type Encoding int

const (
	EncodingL16 Encoding = iota // 16-bit PCM
	EncodingL24                 // 24-bit PCM
)

func (e Encoding) BytesPerSample() int {
	if e == EncodingL24 {
		return 3
	}
	return 2
}

// Format describes one ST 2110-30 stream.
type Format struct {
	Encoding   Encoding
	Channels   int
	SampleRate int // 48000 or 96000
}

func (f Format) Validate() error {
	if f.Encoding != EncodingL16 && f.Encoding != EncodingL24 {
		return fmt.Errorf("st2110: unsupported encoding %d", f.Encoding)
	}
	if f.Channels < 1 || f.Channels > 64 {
		return fmt.Errorf("st2110: invalid channel count %d", f.Channels)
	}
	if f.SampleRate != 48000 && f.SampleRate != 96000 {
		return fmt.Errorf("st2110: invalid sample rate %d", f.SampleRate)
	}
	return nil
}

// FrameSize is bytes per sample frame (all channels, one sampling instant).
func (f Format) FrameSize() int {
	return f.Encoding.BytesPerSample() * f.Channels
}

// SamplesInPayload returns the number of sample frames in a payload.
// ST 2110-30 requires the payload to hold a whole number of sample frames;
// a remainder means a truncated or mis-declared packet.
func (f Format) SamplesInPayload(payloadBytes int) (int, error) {
	if payloadBytes == 0 {
		return 0, fmt.Errorf("st2110: empty audio payload")
	}
	frame := f.FrameSize()
	if payloadBytes%frame != 0 {
		return 0, fmt.Errorf("st2110: payload %d bytes not a multiple of frame size %d", payloadBytes, frame)
	}
	return payloadBytes / frame, nil
}

// SamplesToDuration converts a sample-frame count at this format's rate
// to nanoseconds, safe for uint32-wrapping RTP timestamp deltas.
func (f Format) SamplesToDuration(samples uint32) int64 {
	return int64(samples) * 1_000_000_000 / int64(f.SampleRate)
}
