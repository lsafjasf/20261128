package lc3

import (
	"encoding/binary"
	"fmt"
)

// Packet is the re-packetized LC3 container used on the wire. It is a compact
// framing that keeps the two pieces of information a fixed-duration player
// loses: the 16-bit sequence number (for loss detection) and the per-frame
// duration (for exact timing).
//
// Wire layout (all multi-byte integers big-endian):
//
//	octets 0-1 : RTP-style sequence number
//	octet  2   : flags; bit 7 = frame duration (0 = 10 ms, 1 = 7.5 ms),
//	             bits 6-0 reserved
//	octets 3-4 : number of complete LC3 frames carried in this packet
//	then, per frame:
//	  octets 0-1 : encoded frame size in bytes ("octets")
//	  n octets   : encoded LC3 payload
type Packet struct {
	Seq      uint16
	Duration Duration
	Frames   []Frame
}

const packetFixedHeader = 5

// MarshalBinary serializes a packet. Every frame carries its declared Octets
// length prefix; Payload may be shorter to model a truncated tail frame.
func (p Packet) MarshalBinary() ([]byte, error) {
	if !p.Duration.Valid() {
		return nil, fmt.Errorf("lc3: invalid packet duration %d", p.Duration)
	}
	size := packetFixedHeader
	for _, f := range p.Frames {
		size += 2 + len(f.Payload)
	}
	buf := make([]byte, packetFixedHeader, size)
	binary.BigEndian.PutUint16(buf[0:2], p.Seq)
	if p.Duration == Dur7p5ms {
		buf[2] = 0x80
	}
	binary.BigEndian.PutUint16(buf[3:5], uint16(len(p.Frames)))
	for _, f := range p.Frames {
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(f.Octets))
		buf = append(buf, lenBuf...)
		buf = append(buf, f.Payload...)
	}
	return buf, nil
}

// ParsePacket decodes one wire packet. It returns ErrShortBuffer when the
// header or any frame's declared payload cannot be satisfied. A frame whose
// declared Octets exceed the remaining bytes is surfaced as a truncated frame
// rather than rejected, so callers can detect a short tail frame.
func ParsePacket(data []byte) (Packet, error) {
	if len(data) < packetFixedHeader {
		return Packet{}, fmt.Errorf("%w: need %d packet header bytes, got %d", ErrShortBuffer, packetFixedHeader, len(data))
	}
	p := Packet{Seq: binary.BigEndian.Uint16(data[0:2])}
	if data[2]&0x80 != 0 {
		p.Duration = Dur7p5ms
	} else {
		p.Duration = Dur10ms
	}
	count := int(binary.BigEndian.Uint16(data[3:5]))
	pos := packetFixedHeader
	for i := 0; i < count; i++ {
		if pos+2 > len(data) {
			return p, fmt.Errorf("%w: missing size prefix for frame %d", ErrShortBuffer, i)
		}
		octets := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
		// The packet carries one RTP sequence number; each contained frame
		// occupies the next frame slot, so derive its per-frame sequence.
		f := Frame{Seq: p.Seq + uint16(i), Duration: p.Duration, Octets: octets}
		if octets > 0 {
			if pos >= len(data) {
				f.Truncated = true
			} else if avail := len(data) - pos; avail >= octets {
				f.Payload = data[pos : pos+octets : pos+octets]
				pos += octets
			} else {
				f.Payload = data[pos:len(data):len(data)]
				f.Truncated = true
				pos = len(data)
			}
		}
		p.Frames = append(p.Frames, f)
	}
	return p, nil
}

// ParsePacketStream decodes a concatenated sequence of packets and returns the
// ordered list of all contained frames. Sequence numbers are preserved on each
// frame so Build can locate loss gaps.
func ParsePacketStream(data []byte) ([]Frame, error) {
	var frames []Frame
	pos := 0
	for pos < len(data) {
		p, err := ParsePacket(data[pos:])
		frames = append(frames, p.Frames...)
		if err != nil {
			return frames, fmt.Errorf("lc3: packet at offset %d: %w", pos, err)
		}
		consumed, _ := packetSize(data[pos:])
		pos += consumed
	}
	return frames, nil
}

// packetSize computes the full wire length of a packet starting at data[0].
func packetSize(data []byte) (int, error) {
	if len(data) < packetFixedHeader {
		return 0, fmt.Errorf("%w: short header", ErrShortBuffer)
	}
	count := int(binary.BigEndian.Uint16(data[3:5]))
	pos := packetFixedHeader
	for i := 0; i < count; i++ {
		if pos+2 > len(data) {
			return 0, fmt.Errorf("%w: missing frame size", ErrShortBuffer)
		}
		octets := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2 + octets
		if pos > len(data) {
			return len(data), nil
		}
	}
	return pos, nil
}
