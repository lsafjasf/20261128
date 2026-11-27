package opusmap

import "fmt"

type missingStreamError struct {
	stream int
	count  int
	final  bool
}

func (e *missingStreamError) Error() string {
	if e.final {
		return fmt.Sprintf("missing final stream %d of %d", e.stream, e.count)
	}
	return fmt.Sprintf("missing stream %d of %d", e.stream, e.count)
}

type durationMismatchError struct {
	stream   int
	duration int
	expected int
}

func (e *durationMismatchError) Error() string {
	return fmt.Sprintf(
		"stream %d duration is %d samples, stream 0 is %d",
		e.stream, e.duration, e.expected,
	)
}

// OpusPacket describes TOC framing. It does not contain decoded PCM.
type OpusPacket struct {
	TOC           byte  `json:"toc"`
	Config        uint8 `json:"config"`
	Stereo        bool  `json:"stereo"`
	Code          uint8 `json:"code"`
	FrameCount    int   `json:"frame_count"`
	FrameLengths  []int `json:"frame_lengths"`
	Duration48k   int   `json:"duration_48k"`
	SelfDelimited bool  `json:"self_delimited"`
	Size          int   `json:"size"`
}

// ParseMultistreamPacket splits one Ogg audio packet into the N Opus packets
// described by OpusHead. The first N-1 packets use RFC 6716 Appendix B
// self-delimiting framing; the final packet uses regular framing and consumes
// the remaining bytes, including a short final frame.
func ParseMultistreamPacket(payload []byte, streamCount int) ([]OpusPacket, error) {
	if streamCount <= 0 {
		return nil, fmt.Errorf("stream count must be positive, got %d", streamCount)
	}
	packets := make([]OpusPacket, 0, streamCount)
	offset := 0
	for stream := 0; stream < streamCount-1; stream++ {
		if offset >= len(payload) {
			return packets, &missingStreamError{
				stream: stream, count: streamCount,
			}
		}
		packet, err := parseOpusPacket(payload[offset:], true)
		if err != nil {
			return packets, fmt.Errorf("interpreting stream %d of %d: %w", stream, streamCount, err)
		}
		packets = append(packets, packet)
		offset += packet.Size
	}
	if offset >= len(payload) {
		return packets, &missingStreamError{
			stream: streamCount - 1, count: streamCount, final: true,
		}
	}
	packet, err := parseOpusPacket(payload[offset:], false)
	if err != nil {
		return packets, fmt.Errorf("interpreting final stream %d of %d: %w", streamCount-1, streamCount, err)
	}
	packets = append(packets, packet)
	for i := 1; i < len(packets); i++ {
		if packets[i].Duration48k != packets[0].Duration48k {
			return packets, &durationMismatchError{
				stream: i, duration: packets[i].Duration48k,
				expected: packets[0].Duration48k,
			}
		}
	}
	return packets, nil
}

func parseOpusPacket(data []byte, selfDelimited bool) (OpusPacket, error) {
	var packet OpusPacket
	if len(data) == 0 {
		return packet, fmt.Errorf("empty Opus packet")
	}
	packet.TOC = data[0]
	packet.Config = data[0] >> 3
	packet.Stereo = data[0]&0x04 != 0
	packet.Code = data[0] & 0x03
	packet.SelfDelimited = selfDelimited

	offset := 1
	var lengths []int
	padding := 0
	switch packet.Code {
	case 0:
		length, err := frameLength(data, &offset, selfDelimited, len(data)-offset)
		if err != nil {
			return packet, err
		}
		lengths = []int{length}
	case 1:
		if selfDelimited {
			length, err := readFrameLength(data, &offset)
			if err != nil {
				return packet, err
			}
			lengths = []int{length, length}
			break
		}
		remaining := len(data) - offset
		if remaining%2 != 0 {
			return packet, fmt.Errorf("code 1 payload is not divisible into two frames")
		}
		lengths = []int{remaining / 2, remaining / 2}
	case 2:
		first, err := readFrameLength(data, &offset)
		if err != nil {
			return packet, err
		}
		second, err := frameLength(data, &offset, selfDelimited, len(data)-offset-first)
		if err != nil {
			return packet, err
		}
		lengths = []int{first, second}
	case 3:
		var err error
		lengths, padding, err = code3Lengths(data, &offset, selfDelimited)
		if err != nil {
			return packet, err
		}
	default:
		return packet, fmt.Errorf("invalid frame count code %d", packet.Code)
	}

	frameBytes := 0
	for _, length := range lengths {
		if length < 0 || length > 1275 {
			return packet, fmt.Errorf("frame length %d outside 0...1275", length)
		}
		frameBytes += length
	}
	consumed := offset + frameBytes + padding
	if selfDelimited {
		if consumed > len(data) {
			return packet, fmt.Errorf(
				"self-delimited packet needs %d bytes, only %d available",
				consumed, len(data),
			)
		}
	} else if consumed != len(data) {
		return packet, fmt.Errorf(
			"regular packet consumes %d bytes, got %d", consumed, len(data),
		)
	}

	packet.FrameCount = len(lengths)
	packet.FrameLengths = lengths
	packet.Duration48k = packet.FrameCount * frameDuration48k(packet.Config)
	if packet.Duration48k > 5760 {
		return packet, fmt.Errorf("packet duration exceeds 120 ms")
	}
	packet.Size = consumed
	return packet, nil
}

func frameLength(data []byte, offset *int, delimited bool, remaining int) (int, error) {
	if delimited {
		return readFrameLength(data, offset)
	}
	if remaining < 0 {
		return 0, fmt.Errorf("declared frame length exceeds packet")
	}
	return remaining, nil
}

func readFrameLength(data []byte, offset *int) (int, error) {
	if *offset >= len(data) {
		return 0, fmt.Errorf("truncated frame length")
	}
	first := int(data[*offset])
	(*offset)++
	if first < 252 {
		return first, nil
	}
	if *offset >= len(data) {
		return 0, fmt.Errorf("truncated two-byte frame length")
	}
	second := int(data[*offset])
	(*offset)++
	return second*4 + first, nil
}

func code3Lengths(data []byte, offset *int, delimited bool) ([]int, int, error) {
	if *offset >= len(data) {
		return nil, 0, fmt.Errorf("truncated code 3 frame-count byte")
	}
	countByte := data[*offset]
	(*offset)++
	vbr := countByte&0x80 != 0
	hasPadding := countByte&0x40 != 0
	frames := int(countByte & 0x3f)
	if frames == 0 {
		return nil, 0, fmt.Errorf("code 3 declares zero frames")
	}

	paddingBytes := 0
	if hasPadding {
		for {
			if *offset >= len(data) {
				return nil, 0, fmt.Errorf("truncated padding length")
			}
			value := int(data[*offset])
			(*offset)++
			if value == 255 {
				paddingBytes += 254
			} else {
				paddingBytes += value
				break
			}
		}
	}

	lengths := make([]int, frames)
	if vbr {
		for i := 0; i < frames-1; i++ {
			length, err := readFrameLength(data, offset)
			if err != nil {
				return nil, 0, err
			}
			lengths[i] = length
		}
		if delimited {
			length, err := readFrameLength(data, offset)
			if err != nil {
				return nil, 0, err
			}
			lengths[frames-1] = length
		} else {
			used := 0
			for _, length := range lengths[:frames-1] {
				used += length
			}
			lengths[frames-1] = len(data) - *offset - paddingBytes - used
			if lengths[frames-1] < 0 {
				return nil, 0, fmt.Errorf("VBR frame lengths exceed packet")
			}
		}
		return lengths, paddingBytes, nil
	}

	if delimited {
		length, err := readFrameLength(data, offset)
		if err != nil {
			return nil, 0, err
		}
		for i := range lengths {
			lengths[i] = length
		}
		return lengths, paddingBytes, nil
	}
	remaining := len(data) - *offset - paddingBytes
	if remaining < 0 || remaining%frames != 0 {
		return nil, 0, fmt.Errorf("CBR payload is not divisible into %d frames", frames)
	}
	for i := range lengths {
		lengths[i] = remaining / frames
	}
	return lengths, paddingBytes, nil
}

func frameDuration48k(config uint8) int {
	if int(config) >= len(frameDurations48k) {
		return 0
	}
	return frameDurations48k[config]
}

var frameDurations48k = [32]int{
	480, 960, 1920, 2880,
	480, 960, 1920, 2880,
	480, 960, 1920, 2880,
	480, 960,
	480, 960,
	120, 240, 480, 960,
	120, 240, 480, 960,
	120, 240, 480, 960,
	120, 240, 480, 960,
}
