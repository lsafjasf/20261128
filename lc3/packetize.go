package lc3

// Packetize packs a run of frames that share one duration into a wire packet.
// The sequence number is taken from the first frame; the rest are expected to
// follow consecutively.
func Packetize(seq uint16, dur Duration, payloadSizes []int, payload []byte) (Packet, error) {
	p := Packet{Seq: seq, Duration: dur, Frames: make([]Frame, 0, len(payloadSizes))}
	offset := 0
	for _, size := range payloadSizes {
		f := Frame{Seq: seq, Duration: dur, Octets: size}
		if size > 0 {
			end := offset + size
			if end > len(payload) {
				f.Payload = payload[offset:]
				f.Truncated = true
				end = len(payload)
			} else {
				f.Payload = payload[offset:end:end]
			}
			offset = end
		}
		p.Frames = append(p.Frames, f)
		seq++
	}
	return p, nil
}

// StreamFromPackets flattens already-parsed packets into the ordered frame
// slice expected by Build, assigning consecutive sequence semantics.
func StreamFromPackets(packets []Packet) []Frame {
	var out []Frame
	for _, p := range packets {
		out = append(out, p.Frames...)
	}
	return out
}

// MarshalPackets serializes a packet slice into one contiguous byte stream.
func MarshalPackets(packets []Packet) ([]byte, error) {
	var out []byte
	for _, p := range packets {
		raw, err := p.MarshalBinary()
		if err != nil {
			return nil, err
		}
		out = append(out, raw...)
	}
	return out, nil
}
