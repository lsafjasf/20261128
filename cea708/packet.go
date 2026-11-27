package cea708

// MaxPacketLen is the largest legal DTVCC caption channel packet, including
// its one-byte header.
const MaxPacketLen = 128

// DTVCCPacket is a reassembled caption channel packet. Header is the original
// packet header byte; Payload holds the service blocks that follow it.
type DTVCCPacket struct {
	Header  byte
	SeqNo   int
	Payload []byte
}

// headerLen decodes the two-bit sequence number and the payload length (in
// bytes excluding the header). A size field of zero encodes the maximum of 127
// payload bytes.
func headerLen(header byte) (seqNo, payloadLen int) {
	seqNo = int(header>>6) & 0x03
	size := int(header & 0x3F)
	if size == 0 {
		payloadLen = 127
	} else {
		payloadLen = size*2 - 1
	}
	return seqNo, payloadLen
}

// ServiceBlock is one service block inside a DTVCC packet. Number ranges from
// 1..63 (service 0 is the null padding service and yields an empty block).
// Data is the raw caption byte stream for the service.
type ServiceBlock struct {
	Number int
	Data   []byte
}

// ParsePacket splits a reassembled packet (header plus payload) into service
// blocks. It does not allocate copies of the payload bytes beyond the block
// headers it has to skip.
func ParsePacket(packet []byte) (DTVCCPacket, []ServiceBlock) {
	if len(packet) == 0 {
		return DTVCCPacket{}, nil
	}
	header := packet[0]
	seqNo, advertised := headerLen(header)
	payload := packet[1:]
	if len(payload) > advertised {
		payload = payload[:advertised]
	}

	var blocks []ServiceBlock
	for pos := 0; pos < len(payload); {
		serviceNo := int(payload[pos]>>5) & 0x07
		blockSize := int(payload[pos] & 0x1F)
		pos++

		if serviceNo == 7 {
			// Extended service header: one more byte.
			if blockSize == 0 || pos >= len(payload) {
				break
			}
			serviceNo = int(payload[pos] & 0x3F)
			pos++
		}

		if blockSize == 0 {
			// Null service block (or padding); nothing to consume.
			if serviceNo == 0 {
				continue
			}
			break
		}
		if pos+blockSize > len(payload) {
			break
		}

		if serviceNo != 0 {
			blocks = append(blocks, ServiceBlock{
				Number: serviceNo,
				Data:   payload[pos : pos+blockSize : pos+blockSize],
			})
		}
		pos += blockSize
	}

	return DTVCCPacket{Header: header, SeqNo: seqNo, Payload: payload}, blocks
}

// PacketAssembler reorders cc_data triples (as carried inside SMPTE 334
// cc_data, each triple being ccData byte plus two caption bytes) into complete
// DTVCC packets. Only DTVCC cc types 2 (continuation) and 3 (packet start) are
// consumed; callers route CEA-608 triples elsewhere.
type PacketAssembler struct {
	buf     []byte
	need    int
	haveHdr bool
}

// PushTriple feeds one cc_data triple. ccType is the two-bit cc_type field
// (2 or 3 for DTVCC) and valid is the cc_valid flag. When a complete packet is
// available it is returned together with ok=true.
func (a *PacketAssembler) PushTriple(ccType int, valid bool, b1, b2 byte) (packet []byte, ok bool) {
	if !valid || (ccType != 2 && ccType != 3) {
		return nil, false
	}
	if ccType == 3 {
		a.buf = a.buf[:0]
		a.haveHdr = true
		_, payloadLen := headerLen(b1)
		a.need = payloadLen + 1
	}
	if !a.haveHdr {
		return nil, false
	}

	if len(a.buf) < a.need {
		a.buf = append(a.buf, b1)
	}
	if len(a.buf) < a.need {
		a.buf = append(a.buf, b2)
	}

	if len(a.buf) >= a.need {
		out := make([]byte, a.need)
		copy(out, a.buf[:a.need])
		a.buf = a.buf[:0]
		a.haveHdr = false
		a.need = 0
		return out, true
	}
	return nil, false
}

// Reset discards any partially assembled packet.
func (a *PacketAssembler) Reset() {
	a.buf = a.buf[:0]
	a.need = 0
	a.haveHdr = false
}
