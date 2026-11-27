package channelmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// LoadDelivery reads an external delivery channel map from JSON.
func LoadDelivery(path string) (DeliveryMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DeliveryMap{}, fmt.Errorf("read delivery map %s: %w", path, err)
	}
	delivery, err := ParseDelivery(data)
	if err != nil {
		return DeliveryMap{}, fmt.Errorf("parse delivery map %s: %w", path, err)
	}
	delivery.Source = path
	return delivery, nil
}

// ParseDelivery accepts either an array of channel labels or an object with a
// "channels" array. Objects may contain "slot" (one-based position), "label",
// and an optional "source"/"source_ref"/"source_index" binding.
func ParseDelivery(data []byte) (DeliveryMap, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return DeliveryMap{}, fmt.Errorf("delivery map is empty")
	}

	var delivery DeliveryMap
	if trimmed[0] == '[' {
		channels, err := parseDeliveryChannels(trimmed)
		if err != nil {
			return DeliveryMap{}, err
		}
		delivery.Channels = channels
	} else if trimmed[0] == '{' {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return DeliveryMap{}, fmt.Errorf("decode delivery map: %w", err)
		}
		if value, ok := firstRaw(object, "name", "layout", "delivery"); ok {
			_ = json.Unmarshal(value, &delivery.Name)
		}
		rawChannels, ok := firstRaw(object, "channels", "channel_map", "channel_order",
			"track_channels", "order")
		if ok {
			channels, err := parseDeliveryChannelValue(rawChannels)
			if err != nil {
				return DeliveryMap{}, err
			}
			delivery.Channels = channels
		} else {
			channels, err := parseLabelPositionObject(object)
			if err != nil {
				return DeliveryMap{}, err
			}
			delivery.Channels = channels
		}
	} else {
		return DeliveryMap{}, fmt.Errorf("delivery map must be a JSON object or array")
	}

	if len(delivery.Channels) == 0 {
		return DeliveryMap{}, rejection(issue(RejectIncomplete,
			"delivery map has no channels"))
	}
	if err := normalizeDeliveryPositions(delivery.Channels); err != nil {
		return DeliveryMap{}, err
	}
	return delivery, nil
}

func parseDeliveryChannelValue(raw json.RawMessage) ([]DeliveryChannel, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return parseDeliveryObject(trimmed)
	}
	return parseDeliveryChannels(trimmed)
}

func parseDeliveryChannels(raw json.RawMessage) ([]DeliveryChannel, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, fmt.Errorf("delivery channels must be an array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, fmt.Errorf("decode delivery channels: %w", err)
	}

	channels := make([]DeliveryChannel, 0, len(items))
	allPositions, anyPosition, explicitSources := true, false, 0
	for i, item := range items {
		channel, hasPosition, hasSource, err := parseDeliveryChannel(item)
		if err != nil {
			return nil, fmt.Errorf("delivery channel %d: %w", i+1, err)
		}
		allPositions = allPositions && hasPosition
		anyPosition = anyPosition || hasPosition
		if hasSource {
			explicitSources++
		}
		channels = append(channels, channel)
	}
	if anyPosition && !allPositions {
		return nil, rejection(issue(RejectIncomplete,
			"delivery map assigns positions to only some channels"))
	}
	if explicitSources > 0 && explicitSources != len(channels) {
		return nil, rejection(issue(RejectIncomplete,
			"delivery map binds only %d of %d channels to container sources",
			explicitSources, len(channels)))
	}
	if !anyPosition {
		for i := range channels {
			channels[i].Position = i + 1
		}
	}
	return channels, nil
}

func parseDeliveryChannel(raw json.RawMessage) (DeliveryChannel, bool, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return DeliveryChannel{}, false, false, err
		}
		label, err := NormalizeLabel(text)
		if err != nil {
			return DeliveryChannel{}, false, false, err
		}
		return DeliveryChannel{Label: label, Name: label.Name()}, false, false, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return DeliveryChannel{}, false, false, fmt.Errorf("channel must be a string or object: %w", err)
	}
	labelText, ok := firstString(object, "label", "channel", "channel_label",
		"mapped_channel", "name")
	if !ok {
		return DeliveryChannel{}, false, false, fmt.Errorf("missing channel label")
	}
	label, err := NormalizeLabel(labelText)
	if err != nil {
		return DeliveryChannel{}, false, false, err
	}

	channel := DeliveryChannel{Label: label, Name: label.Name()}
	if rawPosition, ok := firstRaw(object, "position", "slot", "order"); ok {
		position, err := parsePositiveInt(rawPosition)
		if err != nil {
			return DeliveryChannel{}, false, false, fmt.Errorf("invalid position: %w", err)
		}
		channel.Position = position
	}

	sourceRef, hasSource, err := parseSourceRef(object)
	if err != nil {
		return DeliveryChannel{}, false, false, err
	}
	channel.SourceRef = sourceRef
	return channel, channel.Position != 0, hasSource, nil
}

func parseSourceRef(object map[string]json.RawMessage) (string, bool, error) {
	var refs []string
	if value, ok := firstRaw(object, "source", "source_ref", "source_channel",
		"source_label", "container_channel", "embedded_channel"); ok {
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return "", false, fmt.Errorf("source reference must be a string")
		}
		text = strings.TrimSpace(text)
		if text != "" {
			if _, err := strconv.Atoi(text); err != nil {
				label, err := NormalizeLabel(text)
				if err != nil {
					return "", false, fmt.Errorf("invalid source reference: %w", err)
				}
				text = string(label)
			}
			refs = append(refs, text)
		}
	}
	if value, ok := firstRaw(object, "source_index"); ok {
		index, err := parseNonNegativeInt(value)
		if err != nil {
			return "", false, fmt.Errorf("invalid source index: %w", err)
		}
		refs = append(refs, strconv.Itoa(index))
	}
	if value, ok := firstRaw(object, "source_position"); ok {
		position, err := parsePositiveInt(value)
		if err != nil {
			return "", false, fmt.Errorf("invalid source position: %w", err)
		}
		refs = append(refs, strconv.Itoa(position-1))
	}
	if len(refs) > 1 && refs[0] != refs[1] {
		return "", false, fmt.Errorf("conflicting source references")
	}
	if len(refs) == 0 {
		return "", false, nil
	}
	return refs[0], true, nil
}

func normalizeDeliveryPositions(channels []DeliveryChannel) error {
	count := len(channels)
	seen := make(map[int]int, count)
	for i := range channels {
		position := channels[i].Position
		if position < 1 || position > count {
			return rejection(issue(RejectIncomplete,
				"delivery position %d is outside the complete 1..%d range", position, count))
		}
		if previous, ok := seen[position]; ok {
			return rejection(issue(RejectOverlap,
				"delivery slots %d and %d both use position %d",
				previous+1, i+1, position))
		}
		seen[position] = i
	}
	sort.SliceStable(channels, func(i, j int) bool {
		return channels[i].Position < channels[j].Position
	})
	return nil
}

func parseLabelPositionObject(object map[string]json.RawMessage) ([]DeliveryChannel, error) {
	channels := make([]DeliveryChannel, 0, len(object))
	for key, value := range object {
		switch strings.ToLower(key) {
		case "name", "layout", "delivery", "source":
			continue
		}
		position, err := parsePositiveInt(value)
		if err != nil {
			return nil, fmt.Errorf("delivery channel %q must have a one-based position", key)
		}
		label, err := NormalizeLabel(key)
		if err != nil {
			return nil, err
		}
		channels = append(channels, DeliveryChannel{
			Position: position, Label: label, Name: label.Name(),
		})
	}
	return channels, nil
}

func parseDeliveryObject(raw json.RawMessage) ([]DeliveryChannel, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode delivery channel object: %w", err)
	}
	if len(object) == 0 {
		return nil, fmt.Errorf("delivery channel object is empty")
	}

	slotToLabel := true
	for key, value := range object {
		if _, err := strconv.Atoi(strings.TrimSpace(key)); err != nil {
			slotToLabel = false
			break
		}
		value = bytes.TrimSpace(value)
		if len(value) == 0 || value[0] != '"' {
			slotToLabel = false
			break
		}
	}
	if slotToLabel {
		channels := make([]DeliveryChannel, 0, len(object))
		for key, value := range object {
			position, _ := strconv.Atoi(strings.TrimSpace(key))
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return nil, fmt.Errorf("decode delivery label for slot %d: %w", position, err)
			}
			label, err := NormalizeLabel(text)
			if err != nil {
				return nil, fmt.Errorf("delivery slot %d: %w", position, err)
			}
			channels = append(channels, DeliveryChannel{
				Position: position, Label: label, Name: label.Name(),
			})
		}
		return channels, nil
	}
	return parseLabelPositionObject(object)
}

func firstRaw(object map[string]json.RawMessage, keys ...string) (json.RawMessage, bool) {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func firstString(object map[string]json.RawMessage, keys ...string) (string, bool) {
	value, ok := firstRaw(object, keys...)
	if !ok {
		return "", false
	}
	var text string
	if json.Unmarshal(value, &text) != nil {
		return "", false
	}
	return text, true
}

func parsePositiveInt(raw json.RawMessage) (int, error) {
	value, err := parseNonNegativeInt(raw)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, fmt.Errorf("must be at least 1")
	}
	return value, nil
}

func parseNonNegativeInt(raw json.RawMessage) (int, error) {
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		if number < 0 {
			return 0, fmt.Errorf("must not be negative")
		}
		return number, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	number, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || number < 0 {
		return 0, fmt.Errorf("must be a non-negative integer")
	}
	return number, nil
}
