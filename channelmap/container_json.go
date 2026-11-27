package channelmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// parseContainerJSON supports a metadata-only container descriptor. This is
// useful for codecs whose container metadata has already been extracted, but
// channel order must still be supplied by the container side rather than the
// delivery map.
func parseContainerJSON(data []byte, source string) (Layout, error) {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		labels, err := parseJSONChannelList(data)
		if err != nil {
			return Layout{}, err
		}
		return Layout{
			Container: "JSON",
			Channels:  makeChannels(labels),
			Source:    source,
		}, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return Layout{}, fmt.Errorf("decode container descriptor: %w", err)
	}
	layout := Layout{Container: "JSON", Source: source}
	if value, ok := firstString(object, "container", "format"); ok {
		layout.Container = value
	}
	if value, ok := firstString(object, "codec", "codec_name", "audio_codec"); ok {
		layout.Codec = value
	}
	if value, ok := firstRaw(object, "sample_rate", "sampleRate"); ok {
		rate, err := parseNonNegativeInt(value)
		if err != nil {
			return Layout{}, fmt.Errorf("invalid sample rate: %w", err)
		}
		layout.SampleRate = rate
	}

	declaredCount := 0
	if value, ok := firstRaw(object, "channel_count", "channelCount", "channels_count"); ok {
		count, err := parsePositiveInt(value)
		if err != nil {
			return Layout{}, fmt.Errorf("invalid channel count: %w", err)
		}
		declaredCount = count
	} else if value, ok := firstRaw(object, "channels"); ok {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) > 0 && trimmed[0] != '[' && trimmed[0] != '{' {
			count, err := parsePositiveInt(value)
			if err != nil {
				return Layout{}, fmt.Errorf("invalid channel count: %w", err)
			}
			declaredCount = count
		}
	}

	var maskValue uint32
	hasMask := false
	if value, ok := firstRaw(object, "channel_mask", "channelMask", "dwChannelMask"); ok {
		mask, err := parseUint32(value)
		if err != nil {
			return Layout{}, fmt.Errorf("invalid channel mask: %w", err)
		}
		maskValue, hasMask = mask, true
	}

	var labels []Label
	rawChannels, hasChannels := firstChannelList(object, "channels", "channel_order",
		"channelOrder", "encoded_order", "embedded_channel_order", "order")
	if hasChannels {
		var err error
		labels, err = parseJSONChannelList(rawChannels)
		if err != nil {
			return Layout{}, err
		}
	} else if hasMask {
		count := declaredCount
		if count == 0 {
			count = channelBitCount(maskValue)
		}
		if count == 0 {
			return Layout{}, fmt.Errorf("container channel mask has no channels")
		}
		var err error
		labels, err = labelsFromWAVMask(maskValue, count)
		if err != nil {
			return Layout{}, err
		}
	} else {
		return Layout{}, fmt.Errorf("container descriptor needs channels/channel_order or channel_mask")
	}

	if declaredCount != 0 && declaredCount != len(labels) {
		return Layout{}, fmt.Errorf("container declares %d channels but lists %d", declaredCount, len(labels))
	}
	if hasMask && maskValue != 0 {
		fromMask, err := labelsFromWAVMask(maskValue, len(labels))
		if err != nil {
			return Layout{}, err
		}
		if !sameLabels(labels, fromMask) {
			return Layout{}, fmt.Errorf("container channel order conflicts with channel mask 0x%08X",
				maskValue)
		}
		maskCopy := maskValue
		layout.ChannelMask = &maskCopy
	}
	layout.Channels = makeChannels(labels)
	return layout, nil
}

func parseJSONChannelList(raw json.RawMessage) ([]Label, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("container channel list must be an array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, fmt.Errorf("decode container channels: %w", err)
	}
	labels := make([]Label, 0, len(items))
	for i, item := range items {
		item = bytes.TrimSpace(item)
		var text string
		if len(item) > 0 && item[0] == '"' {
			if err := json.Unmarshal(item, &text); err != nil {
				return nil, fmt.Errorf("channel %d: %w", i+1, err)
			}
		} else {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(item, &object); err != nil {
				return nil, fmt.Errorf("channel %d must be a string or object", i+1)
			}
			var ok bool
			text, ok = firstString(object, "label", "channel", "channel_label", "name")
			if !ok {
				return nil, fmt.Errorf("channel %d has no label", i+1)
			}
		}
		label, err := NormalizeLabel(text)
		if err != nil {
			return nil, fmt.Errorf("channel %d: %w", i+1, err)
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return nil, fmt.Errorf("container channel list is empty")
	}
	return adaptLegacySurroundLabels(labels), nil
}

func parseUint32(raw json.RawMessage) (uint32, error) {
	var number uint64
	if err := json.Unmarshal(raw, &number); err == nil {
		if number > 0xFFFFFFFF {
			return 0, fmt.Errorf("value exceeds 32 bits")
		}
		return uint32(number), nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf("must be a number or hexadecimal string")
	}
	text = strings.TrimSpace(text)
	base := 10
	if strings.HasPrefix(strings.ToLower(text), "0x") {
		text, base = text[2:], 16
	}
	number, err := strconv.ParseUint(text, base, 32)
	if err != nil {
		return 0, fmt.Errorf("must be a 32-bit integer")
	}
	return uint32(number), nil
}

func channelBitCount(mask uint32) int {
	count := 0
	for mask != 0 {
		count += int(mask & 1)
		mask >>= 1
	}
	return count
}

func sameLabels(left, right []Label) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func firstChannelList(object map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, key := range keys {
		value, ok := object[key]
		if !ok {
			continue
		}
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			return value, true
		}
	}
	return nil, false
}

func adaptLegacySurroundLabels(labels []Label) []Label {
	hasLeftSide, hasRightSide := false, false
	hasLeftRear, hasRightRear := false, false
	for _, label := range labels {
		switch label {
		case LabelLeftSurround:
			hasLeftSide = true
		case LabelRightSurround:
			hasRightSide = true
		case LabelLeftRearSurround:
			hasLeftRear = true
		case LabelRightRearSurround:
			hasRightRear = true
		}
	}
	adapted := append([]Label(nil), labels...)
	for i, label := range adapted {
		if label == LabelLeftRearSurround && hasLeftRear && !hasLeftSide {
			adapted[i] = LabelLeftSurround
		}
		if label == LabelRightRearSurround && hasRightRear && !hasRightSide {
			adapted[i] = LabelRightSurround
		}
	}
	return adapted
}
