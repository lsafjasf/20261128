package channelmap

import (
	"fmt"
	"sort"
	"strconv"
)

// BuildPlan validates that container channels and delivery slots form a
// one-to-one mapping, then returns the concrete source-to-delivery permutation.
// Duplicate matches are reported as overlap; missing channels, slots, or source
// references are reported as incomplete. Any non-bijective result is rejected.
func BuildPlan(layout Layout, delivery DeliveryMap) (*Plan, error) {
	source, sourceErr := normalizeSourceLayout(layout)
	if sourceErr != nil {
		return nil, sourceErr
	}
	targets, targetErr := normalizeDeliveryMap(delivery)
	if targetErr != nil {
		return nil, targetErr
	}

	if len(source.Channels) != len(targets.Channels) {
		return nil, rejection(issue(RejectIncomplete,
			"container has %d source channels but delivery map has %d slots",
			len(source.Channels), len(targets.Channels)))
	}

	issues := checkUniqueLabels(source, targets)
	rows, mappingIssues := buildMappingRows(source, targets)
	issues = append(issues, mappingIssues...)
	if len(issues) > 0 {
		return nil, rejection(issues...)
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Delivery.Position < rows[j].Delivery.Position
	})
	identity := true
	for _, row := range rows {
		if row.Source.Index != row.Delivery.Position-1 {
			identity = false
			break
		}
	}
	return &Plan{
		SchemaVersion: 1,
		Container:     source,
		Delivery:      targets,
		Mappings:      rows,
		Identity:      identity,
	}, nil
}

func normalizeSourceLayout(layout Layout) (Layout, error) {
	if len(layout.Channels) == 0 {
		return Layout{}, fmt.Errorf("container has no channels")
	}
	normalized := layout
	normalized.Channels = make([]Channel, len(layout.Channels))
	issues := make([]Issue, 0)
	seen := make(map[Label]int, len(layout.Channels))
	for i, raw := range layout.Channels {
		label, err := NormalizeLabel(string(raw.Label))
		if err != nil {
			issues = append(issues, issue(RejectInvalid,
				"container channel %d: %v", i+1, err))
			continue
		}
		if previous, ok := seen[label]; ok {
			issues = append(issues, issue(RejectOverlap,
				"container channels %d and %d both resolve to %s", previous+1, i+1, label))
		}
		seen[label] = i
		normalized.Channels[i] = Channel{
			Index: i, Position: i + 1, Label: label, Name: label.Name(),
		}
	}
	if len(issues) > 0 {
		return Layout{}, rejection(issues...)
	}
	return normalized, nil
}

func normalizeDeliveryMap(delivery DeliveryMap) (DeliveryMap, error) {
	normalized := delivery
	normalized.Channels = append([]DeliveryChannel(nil), delivery.Channels...)
	if len(normalized.Channels) == 0 {
		return DeliveryMap{}, rejection(issue(RejectIncomplete,
			"delivery map has no slots"))
	}

	allZero, anyZero := true, false
	for i := range normalized.Channels {
		if normalized.Channels[i].Position == 0 {
			anyZero = true
		} else {
			allZero = false
		}
		label, err := NormalizeLabel(string(normalized.Channels[i].Label))
		if err != nil {
			return DeliveryMap{}, rejection(issue(RejectInvalid,
				"delivery slot %d: %v", i+1, err))
		}
		normalized.Channels[i].Label = label
		normalized.Channels[i].Name = label.Name()
		if normalized.Channels[i].SourceRef != "" {
			ref, err := normalizeSourceRef(normalized.Channels[i].SourceRef)
			if err != nil {
				return DeliveryMap{}, rejection(issue(RejectInvalid,
					"delivery slot %d: %v", i+1, err))
			}
			normalized.Channels[i].SourceRef = ref
		}
	}
	if anyZero && !allZero {
		return DeliveryMap{}, rejection(issue(RejectIncomplete,
			"delivery map assigns positions to only some slots"))
	}
	if allZero {
		for i := range normalized.Channels {
			normalized.Channels[i].Position = i + 1
		}
	}
	if err := normalizeDeliveryPositions(normalized.Channels); err != nil {
		return DeliveryMap{}, err
	}
	return normalized, nil
}

func normalizeSourceRef(raw string) (string, error) {
	if _, err := strconv.Atoi(raw); err == nil {
		return raw, nil
	}
	label, err := NormalizeLabel(raw)
	if err != nil {
		return "", fmt.Errorf("invalid source reference %q", raw)
	}
	return string(label), nil
}

func checkUniqueLabels(source Layout, delivery DeliveryMap) []Issue {
	issues := make([]Issue, 0)
	deliveryLabels := make(map[Label]int, len(delivery.Channels))
	for i, channel := range delivery.Channels {
		if previous, ok := deliveryLabels[channel.Label]; ok {
			issues = append(issues, issue(RejectOverlap,
				"delivery slots %d and %d both target %s",
				previous+1, i+1, channel.Label))
		}
		deliveryLabels[channel.Label] = i
	}

	explicit := 0
	for _, channel := range delivery.Channels {
		if channel.SourceRef != "" {
			explicit++
		}
	}
	if explicit > 0 && explicit != len(delivery.Channels) {
		issues = append(issues, issue(RejectIncomplete,
			"only %d of %d delivery slots bind a container source",
			explicit, len(delivery.Channels)))
	}
	return issues
}

func buildMappingRows(source Layout, delivery DeliveryMap) ([]ChannelMapping, []Issue) {
	sourceByLabel := make(map[Label]Channel, len(source.Channels))
	for _, channel := range source.Channels {
		sourceByLabel[channel.Label] = channel
	}

	explicit := delivery.Channels[0].SourceRef != ""
	if explicit {
		return buildExplicitRows(source, delivery, sourceByLabel)
	}
	return buildImplicitRows(source, delivery, sourceByLabel)
}

func buildImplicitRows(source Layout, delivery DeliveryMap,
	sourceByLabel map[Label]Channel) ([]ChannelMapping, []Issue) {
	rowsBySlot := make(map[int]ChannelMapping, len(delivery.Channels))
	used := make(map[int]bool, len(source.Channels))
	issues := make([]Issue, 0)

	for _, target := range delivery.Channels {
		channel, ok := sourceByLabel[target.Label]
		if !ok || used[channel.Index] {
			continue
		}
		rowsBySlot[target.Position] = ChannelMapping{
			Source: channel, Delivery: target,
			Reordered: channel.Index != target.Position-1,
		}
		used[channel.Index] = true
	}

	for _, target := range delivery.Channels {
		if _, ok := rowsBySlot[target.Position]; ok {
			continue
		}
		candidates := surroundFallbackCandidates(source, target.Label, used)
		switch len(candidates) {
		case 1:
			channel := candidates[0]
			rowsBySlot[target.Position] = ChannelMapping{
				Source: channel, Delivery: target,
				Reordered: channel.Index != target.Position-1,
			}
			used[channel.Index] = true
		case 0:
			issues = append(issues, issue(RejectIncomplete,
				"delivery slot %d (%s) has no unused container channel",
				target.Position, target.Label))
		default:
			issues = append(issues, issue(RejectOverlap,
				"delivery slot %d (%s) ambiguously matches %d container channels",
				target.Position, target.Label, len(candidates)))
		}
	}

	for _, channel := range source.Channels {
		if !used[channel.Index] {
			issues = append(issues, issue(RejectIncomplete,
				"container channel %d (%s) has no delivery slot",
				channel.Position, channel.Label))
		}
	}
	rows := make([]ChannelMapping, 0, len(rowsBySlot))
	for _, row := range rowsBySlot {
		rows = append(rows, row)
	}
	return rows, issues
}

func buildExplicitRows(source Layout, delivery DeliveryMap,
	sourceByLabel map[Label]Channel) ([]ChannelMapping, []Issue) {
	rowsBySlot := make(map[int]ChannelMapping, len(delivery.Channels))
	used := make(map[int]int, len(source.Channels))
	issues := make([]Issue, 0)
	for _, target := range delivery.Channels {
		channel, ok := resolveSourceRef(target.SourceRef, source, sourceByLabel)
		if !ok {
			issues = append(issues, issue(RejectIncomplete,
				"delivery slot %d references missing container source %q",
				target.Position, target.SourceRef))
			continue
		}
		if previous, exists := used[channel.Index]; exists {
			issues = append(issues, issue(RejectOverlap,
				"container channel %s is bound by delivery slots %d and %d",
				channel.Label, previous, target.Position))
			continue
		}
		if !labelsCompatible(source, channel.Label, target.Label, true) {
			issues = append(issues, issue(RejectMismatch,
				"delivery slot %d (%s) binds container channel %s",
				target.Position, target.Label, channel.Label))
			continue
		}
		used[channel.Index] = target.Position
		rowsBySlot[target.Position] = ChannelMapping{
			Source: channel, Delivery: target,
			Reordered: channel.Index != target.Position-1,
		}
	}
	for _, channel := range source.Channels {
		if _, ok := used[channel.Index]; !ok {
			issues = append(issues, issue(RejectIncomplete,
				"container channel %d (%s) is not bound by the delivery map",
				channel.Position, channel.Label))
		}
	}
	rows := make([]ChannelMapping, 0, len(rowsBySlot))
	for _, row := range rowsBySlot {
		rows = append(rows, row)
	}
	return rows, issues
}

func resolveSourceRef(ref string, source Layout, byLabel map[Label]Channel) (Channel, bool) {
	if index, err := strconv.Atoi(ref); err == nil {
		if index < 0 || index >= len(source.Channels) {
			return Channel{}, false
		}
		return source.Channels[index], true
	}
	if channel, ok := byLabel[Label(ref)]; ok {
		return channel, true
	}
	candidates := surroundFallbackCandidates(source, Label(ref), nil)
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return Channel{}, false
}

func surroundFallbackCandidates(source Layout, target Label, used map[int]bool) []Channel {
	family := surroundFamily(target)
	if family == "" {
		return nil
	}
	candidates := make([]Channel, 0, 1)
	for _, channel := range source.Channels {
		if surroundFamily(channel.Label) != family {
			continue
		}
		if used != nil && used[channel.Index] {
			continue
		}
		candidates = append(candidates, channel)
	}
	if len(candidates) != 1 {
		return nil
	}
	return candidates
}

func surroundFamily(label Label) string {
	switch label {
	case LabelLeftSurround, LabelLeftRearSurround:
		return "left-surround"
	case LabelRightSurround, LabelRightRearSurround:
		return "right-surround"
	default:
		return ""
	}
}

func labelsCompatible(source Layout, sourceLabel, targetLabel Label, allowGeneric bool) bool {
	if sourceLabel == targetLabel {
		return true
	}
	if allowGeneric && (isGenericLabel(sourceLabel) || isGenericLabel(targetLabel)) {
		return true
	}
	family := surroundFamily(sourceLabel)
	if family != "" && family == surroundFamily(targetLabel) {
		return len(surroundFallbackCandidates(source, targetLabel, nil)) == 1
	}
	return false
}

func isGenericLabel(label Label) bool {
	text := string(label)
	if len(text) <= 2 || text[:2] != "CH" {
		return false
	}
	_, err := strconv.Atoi(text[2:])
	return err == nil
}
