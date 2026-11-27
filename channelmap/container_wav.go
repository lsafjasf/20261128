package channelmap

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"os"
)

// LoadContainer reads channel order and codec metadata from a WAV/RF64 file or
// a JSON container descriptor.
func LoadContainer(path string) (Layout, error) {
	file, err := os.Open(path)
	if err != nil {
		return Layout{}, fmt.Errorf("read container %s: %w", path, err)
	}
	defer file.Close()

	var header [12]byte
	n, err := io.ReadFull(file, header[:])
	if err != nil && err != io.ErrUnexpectedEOF {
		return Layout{}, fmt.Errorf("read container %s: %w", path, err)
	}
	if n == len(header) && (string(header[0:4]) == "RIFF" || string(header[0:4]) == "RF64") &&
		string(header[8:12]) == "WAVE" {
		layout, err := parseWAVFile(file, path)
		if err != nil {
			return Layout{}, fmt.Errorf("%s: %w", path, err)
		}
		return layout, nil
	}

	prefix := append([]byte(nil), header[:n]...)
	extra := make([]byte, 4092)
	extraN, _ := io.ReadFull(file, extra)
	prefix = append(prefix, extra[:extraN]...)
	trimmed := bytes.TrimSpace(prefix)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return Layout{}, fmt.Errorf("read container %s: %w", path, err)
		}
		data, err := io.ReadAll(file)
		if err != nil {
			return Layout{}, fmt.Errorf("read container %s: %w", path, err)
		}
		layout, err := parseContainerJSON(bytes.TrimSpace(data), path)
		if err != nil {
			return Layout{}, fmt.Errorf("%s: %w", path, err)
		}
		return layout, nil
	}
	return Layout{}, fmt.Errorf("%s: unsupported container; expected WAV/RF64 or a JSON descriptor", path)
}

func parseWAVFile(file *os.File, source string) (Layout, error) {
	var format *wavFormat
	for {
		var header [8]byte
		_, err := io.ReadFull(file, header[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return Layout{}, fmt.Errorf("truncated WAV chunk header")
		}
		id := string(header[0:4])
		size := uint64(binary.LittleEndian.Uint32(header[4:8]))
		if id == "fmt " {
			if size < 16 || size > 1<<20 {
				return Layout{}, fmt.Errorf("invalid WAV fmt chunk size %d", size)
			}
			payload := make([]byte, int(size))
			if _, err := io.ReadFull(file, payload); err != nil {
				return Layout{}, fmt.Errorf("truncated WAV fmt chunk")
			}
			parsed, err := parseWAVFormat(payload)
			if err != nil {
				return Layout{}, err
			}
			format = &parsed
			break
		}
		skip := int64(size)
		if size%2 == 1 {
			skip++
		}
		if _, err := file.Seek(skip, io.SeekCurrent); err != nil {
			return Layout{}, fmt.Errorf("invalid WAV chunk %q size %d", id, size)
		}
	}
	if format == nil {
		return Layout{}, fmt.Errorf("WAV has no fmt chunk")
	}
	return layoutFromWAVFormat(*format, source)
}

func layoutFromWAVFormat(format wavFormat, source string) (Layout, error) {
	if format.channels == 0 {
		return Layout{}, fmt.Errorf("WAV declares zero channels")
	}
	var labels []Label
	var mask *uint32
	if format.hasMask && format.channelMask != 0 {
		var err error
		labels, err = labelsFromWAVMask(format.channelMask, int(format.channels))
		if err != nil {
			return Layout{}, err
		}
		maskValue := format.channelMask
		mask = &maskValue
	} else {
		labels = genericLabels(int(format.channels))
	}

	return Layout{
		Container:   "WAV",
		Codec:       format.codecName,
		SampleRate:  int(format.sampleRate),
		ChannelMask: mask,
		Channels:    makeChannels(labels),
		Source:      source,
	}, nil
}

// ParseContainer dispatches on the file signature rather than its extension.
func ParseContainer(data []byte, source string) (Layout, error) {
	trimmed := bytes.TrimSpace(data)
	if len(data) >= 12 && (string(data[0:4]) == "RIFF" || string(data[0:4]) == "RF64") &&
		string(data[8:12]) == "WAVE" {
		return parseWAV(data, source)
	}
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		return parseContainerJSON(trimmed, source)
	}
	return Layout{}, fmt.Errorf("unsupported container; expected WAV/RF64 or a JSON descriptor")
}

type wavFormat struct {
	codec       uint16
	codecName   string
	channels    uint16
	sampleRate  uint32
	blockAlign  uint16
	channelMask uint32
	hasMask     bool
}

func parseWAV(data []byte, source string) (Layout, error) {
	if len(data) < 12 {
		return Layout{}, fmt.Errorf("truncated WAV header")
	}
	pos := 12
	var format *wavFormat
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := uint64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		payloadStart := pos + 8

		if id == "fmt " {
			if size < 16 || size > uint64(len(data)-payloadStart) {
				return Layout{}, fmt.Errorf("invalid WAV fmt chunk size %d", size)
			}
			parsed, err := parseWAVFormat(data[payloadStart : payloadStart+int(size)])
			if err != nil {
				return Layout{}, err
			}
			format = &parsed
			break
		}
		if size > uint64(len(data)-payloadStart) {
			return Layout{}, fmt.Errorf("WAV chunk %q extends past end of file", id)
		}
		next := payloadStart + int(size)
		if size%2 == 1 {
			next++
		}
		if next <= pos || next > len(data) {
			break
		}
		pos = next
	}
	if format == nil {
		return Layout{}, fmt.Errorf("WAV has no fmt chunk")
	}
	return layoutFromWAVFormat(*format, source)
}

func parseWAVFormat(payload []byte) (wavFormat, error) {
	if len(payload) < 16 {
		return wavFormat{}, fmt.Errorf("truncated WAV fmt chunk")
	}
	format := wavFormat{
		codec:      binary.LittleEndian.Uint16(payload[0:2]),
		channels:   binary.LittleEndian.Uint16(payload[2:4]),
		sampleRate: binary.LittleEndian.Uint32(payload[4:8]),
		blockAlign: binary.LittleEndian.Uint16(payload[12:14]),
	}
	format.codecName = codecName(format.codec)

	extension := len(payload) >= 20 && binary.LittleEndian.Uint16(payload[16:18]) >= 22
	if format.codec == 0xFFFE {
		if len(payload) < 40 {
			return wavFormat{}, fmt.Errorf("truncated WAVE_FORMAT_EXTENSIBLE fmt chunk")
		}
		extension = true
	}
	if extension && len(payload) >= 24 {
		format.channelMask = binary.LittleEndian.Uint32(payload[20:24])
		format.hasMask = true
	}
	if format.codec == 0xFFFE {
		subformat := binary.LittleEndian.Uint16(payload[24:26])
		format.codec = subformat
		format.codecName = codecName(subformat)
		if !isWaveSubformatGUID(payload[24:40]) {
			return wavFormat{}, fmt.Errorf("unsupported WAVE_FORMAT_EXTENSIBLE subformat GUID")
		}
	}
	if format.blockAlign == 0 {
		return wavFormat{}, fmt.Errorf("WAV has zero block alignment")
	}
	return format, nil
}

func isWaveSubformatGUID(guid []byte) bool {
	if len(guid) != 16 {
		return false
	}
	tail := []byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA,
		0x00, 0x38, 0x9B, 0x71}
	return bytes.Equal(guid[4:], tail)
}

func codecName(code uint16) string {
	switch code {
	case 0x0001:
		return "PCM"
	case 0x0003:
		return "IEEE_FLOAT"
	case 0x0006:
		return "A_LAW"
	case 0x0007:
		return "MU_LAW"
	case 0xFFFE:
		return "WAVE_FORMAT_EXTENSIBLE"
	default:
		return fmt.Sprintf("WAVE_FORMAT_0x%04X", code)
	}
}

type speakerDef struct {
	bit      uint32
	label    Label
	rearPair bool
}

var waveSpeakerDefs = []speakerDef{
	{0x00000001, LabelLeft, false},
	{0x00000002, LabelRight, false},
	{0x00000004, LabelCenter, false},
	{0x00000008, LabelLFE, false},
	{0x00000010, LabelLeftSurround, true},
	{0x00000020, LabelRightSurround, true},
	{0x00000040, LabelLeftOfCenter, false},
	{0x00000080, LabelRightOfCenter, false},
	{0x00000100, LabelCenterSurround, false},
	{0x00000200, LabelLeftSurround, false},
	{0x00000400, LabelRightSurround, false},
	{0x00000800, LabelTopCenter, false},
	{0x00001000, LabelTopFrontLeft, false},
	{0x00002000, LabelTopFrontCenter, false},
	{0x00004000, LabelTopFrontRight, false},
	{0x00008000, LabelTopBackLeft, false},
	{0x00010000, LabelTopBackCenter, false},
	{0x00020000, LabelTopBackRight, false},
}

func labelsFromWAVMask(mask uint32, channelCount int) ([]Label, error) {
	if bits.OnesCount32(mask) != channelCount {
		return nil, fmt.Errorf("WAV channel mask 0x%08X has %d bits but fmt declares %d channels",
			mask, bits.OnesCount32(mask), channelCount)
	}
	hasSideSurround := mask&0x00000200 != 0 || mask&0x00000400 != 0
	knownBits := uint32(0)
	labels := make([]Label, 0, channelCount)
	for _, speaker := range waveSpeakerDefs {
		knownBits |= speaker.bit
		if mask&speaker.bit == 0 {
			continue
		}
		label := speaker.label
		if speaker.rearPair && hasSideSurround {
			if speaker.bit == 0x00000010 {
				label = LabelLeftRearSurround
			} else {
				label = LabelRightRearSurround
			}
		}
		labels = append(labels, label)
	}
	if unknown := mask &^ knownBits; unknown != 0 {
		return nil, fmt.Errorf("unsupported WAV channel mask bit(s): 0x%08X", unknown)
	}
	return labels, nil
}

func genericLabels(count int) []Label {
	labels := make([]Label, count)
	for i := range labels {
		labels[i] = Label(fmt.Sprintf("CH%d", i+1))
	}
	return labels
}

func normalizeRawLabels(raw []string) ([]Label, error) {
	labels := make([]Label, len(raw))
	for i, text := range raw {
		label, err := NormalizeLabel(text)
		if err != nil {
			return nil, fmt.Errorf("channel %d: %w", i+1, err)
		}
		labels[i] = label
	}
	return labels, nil
}
