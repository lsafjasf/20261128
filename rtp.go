package st2110

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const rtpHeaderMin = 12

var ErrShortPacket = errors.New("st2110: rtp packet too short")

// Packet is a parsed RTP packet (RFC 3550). Payload aliases the input buffer.
type Packet struct {
	Marker         bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	Payload        []byte
}

// ParsePacket parses one RTP datagram, skipping CSRCs, header extensions
// and padding. It rejects non-v2 packets and truncated headers.
func ParsePacket(b []byte) (Packet, error) {
	if len(b) < rtpHeaderMin {
		return Packet{}, ErrShortPacket
	}
	if version := b[0] >> 6; version != 2 {
		return Packet{}, fmt.Errorf("st2110: unsupported rtp version %d", version)
	}
	padding := b[0]&0x20 != 0
	extension := b[0]&0x10 != 0
	off := rtpHeaderMin + 4*int(b[0]&0x0f)
	if len(b) < off {
		return Packet{}, ErrShortPacket
	}
	if extension {
		if len(b) < off+4 {
			return Packet{}, ErrShortPacket
		}
		off += 4 + 4*int(binary.BigEndian.Uint16(b[off+2:off+4]))
		if len(b) < off {
			return Packet{}, ErrShortPacket
		}
	}
	end := len(b)
	if padding {
		pad := int(b[len(b)-1])
		if pad == 0 || pad > end-off {
			return Packet{}, errors.New("st2110: invalid rtp padding")
		}
		end -= pad
	}
	return Packet{
		Marker:         b[1]&0x80 != 0,
		PayloadType:    b[1] & 0x7f,
		SequenceNumber: binary.BigEndian.Uint16(b[2:4]),
		Timestamp:      binary.BigEndian.Uint32(b[4:8]),
		SSRC:           binary.BigEndian.Uint32(b[8:12]),
		Payload:        b[off:end],
	}, nil
}
