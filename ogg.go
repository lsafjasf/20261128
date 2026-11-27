package opusmap

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	oggContinued = 0x01
	oggBOS       = 0x02
	oggEOS       = 0x04
)

// OggPage is a validated Ogg page. Data contains only the concatenated packet
// payload; Segments keeps the packet boundary lacing values.
type OggPage struct {
	Flags    uint8
	Granule  int64
	Serial   uint32
	Sequence uint32
	CRC      uint32
	Segments []byte
	Data     []byte
}

func (p OggPage) Continued() bool { return p.Flags&oggContinued != 0 }
func (p OggPage) BOS() bool       { return p.Flags&oggBOS != 0 }
func (p OggPage) EOS() bool       { return p.Flags&oggEOS != 0 }

// ReadOggPage reads and checksums one Ogg page. A clean end of input returns
// io.EOF; a partial page returns io.ErrUnexpectedEOF.
func ReadOggPage(r io.Reader) (OggPage, error) {
	var page OggPage
	header := make([]byte, 27)
	if _, err := io.ReadFull(r, header); err != nil {
		return page, err
	}
	if string(header[:4]) != "OggS" {
		return page, fmt.Errorf("bad Ogg capture pattern %q", header[:4])
	}
	if header[4] != 0 {
		return page, fmt.Errorf("unsupported Ogg version %d", header[4])
	}
	page.Flags = header[5]
	page.Granule = int64(binary.LittleEndian.Uint64(header[6:14]))
	page.Serial = binary.LittleEndian.Uint32(header[14:18])
	page.Sequence = binary.LittleEndian.Uint32(header[18:22])
	storedCRC := binary.LittleEndian.Uint32(header[22:26])
	segmentCount := int(header[26])
	page.CRC = storedCRC

	segments := make([]byte, segmentCount)
	if _, err := io.ReadFull(r, segments); err != nil {
		return page, unexpected(err)
	}
	page.Segments = segments
	payloadSize := 0
	for _, lacing := range segments {
		payloadSize += int(lacing)
	}
	page.Data = make([]byte, payloadSize)
	if _, err := io.ReadFull(r, page.Data); err != nil {
		return page, unexpected(err)
	}

	crcHeader := append([]byte(nil), header...)
	for i := 22; i < 26; i++ {
		crcHeader[i] = 0
	}
	crc := oggCRC(crcHeader)
	crc = oggCRCUpdate(crc, segments)
	crc = oggCRCUpdate(crc, page.Data)
	if crc != storedCRC {
		return page, fmt.Errorf(
			"Ogg page %d checksum mismatch: stored %08x, calculated %08x",
			page.Sequence, storedCRC, crc,
		)
	}
	return page, nil
}

func unexpected(err error) error {
	if err == io.EOF {
		return io.ErrUnexpectedEOF
	}
	return err
}

func oggCRC(data []byte) uint32 {
	return oggCRCUpdate(0, data)
}

func oggCRCUpdate(crc uint32, data []byte) uint32 {
	for _, value := range data {
		crc = (crc << 8) ^ oggCRCTable[byte(crc>>24)^value]
	}
	return crc
}

var oggCRCTable = func() [256]uint32 {
	var table [256]uint32
	for i := range table {
		value := uint32(i) << 24
		for bit := 0; bit < 8; bit++ {
			if value&0x80000000 != 0 {
				value = (value << 1) ^ 0x04c11db7
			} else {
				value <<= 1
			}
		}
		table[i] = value
	}
	return table
}()
