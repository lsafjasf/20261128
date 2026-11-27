package channelmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// VerifyReadback decodes a persisted mapping and checks it independently
// against the validated plan. It never trusts the in-memory mapping rows.
func VerifyReadback(data []byte, expected *Plan, mappingFile string) *ReadbackEvidence {
	digest := sha256.Sum256(data)
	evidence := &ReadbackEvidence{
		SchemaVersion: 1,
		MappingFile:   mappingFile,
		MappingSHA256: hex.EncodeToString(digest[:]),
	}

	var decoded Plan
	if err := json.Unmarshal(data, &decoded); err != nil {
		evidence.Checks = append(evidence.Checks, ReadbackCheck{
			Name: "json_readback", Passed: false, Detail: err.Error(),
		})
		return evidence
	}
	evidence.Checks = append(evidence.Checks, ReadbackCheck{
		Name: "json_readback", Passed: true,
		Detail: fmt.Sprintf("decoded %d mapping rows", len(decoded.Mappings)),
	})

	bySource := append([]ChannelMapping(nil), decoded.Mappings...)
	sort.SliceStable(bySource, func(i, j int) bool {
		return bySource[i].Source.Index < bySource[j].Source.Index
	})
	byDelivery := append([]ChannelMapping(nil), decoded.Mappings...)
	sort.SliceStable(byDelivery, func(i, j int) bool {
		return byDelivery[i].Delivery.Position < byDelivery[j].Delivery.Position
	})

	evidence.SourceEncodedOrder = make([]Label, 0, len(bySource))
	for _, row := range bySource {
		evidence.SourceEncodedOrder = append(evidence.SourceEncodedOrder, row.Source.Label)
	}
	evidence.DeliveryReadbackOrder = make([]Label, 0, len(byDelivery))
	for _, row := range byDelivery {
		evidence.DeliveryReadbackOrder = append(evidence.DeliveryReadbackOrder, row.Delivery.Label)
		evidence.Rows = append(evidence.Rows, ReadbackRow{
			SourceIndex:      row.Source.Index,
			SourcePosition:   row.Source.Position,
			SourceLabel:      row.Source.Label,
			DeliveryPosition: row.Delivery.Position,
			DeliveryLabel:    row.Delivery.Label,
		})
	}

	sourceCheck, sourceComplete := checkSourceCoverage(decoded.Mappings)
	evidence.Checks = append(evidence.Checks, sourceCheck)
	deliveryCheck, deliveryComplete := checkDeliveryCoverage(decoded.Mappings)
	evidence.Checks = append(evidence.Checks, deliveryCheck)
	evidence.Checks = append(evidence.Checks,
		checkNoOverlap(decoded.Mappings, sourceComplete, deliveryComplete))
	evidence.Checks = append(evidence.Checks,
		checkSourceOrder(evidence.SourceEncodedOrder, expected))
	evidence.Checks = append(evidence.Checks,
		checkDeliveryOrder(evidence.DeliveryReadbackOrder, expected))
	evidence.Checks = append(evidence.Checks, checkPlanEquality(&decoded, expected))

	evidence.Verified = true
	for _, check := range evidence.Checks {
		if !check.Passed {
			evidence.Verified = false
			break
		}
	}
	return evidence
}

func checkSourceCoverage(rows []ChannelMapping) (ReadbackCheck, bool) {
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		if seen[row.Source.Index] {
			return ReadbackCheck{
				Name: "source_coverage", Passed: false,
				Detail: fmt.Sprintf("source index %d appears more than once", row.Source.Index),
			}, false
		}
		seen[row.Source.Index] = true
	}
	for index := 0; index < len(rows); index++ {
		if !seen[index] {
			return ReadbackCheck{
				Name: "source_coverage", Passed: false,
				Detail: fmt.Sprintf("source index %d is missing", index),
			}, false
		}
	}
	return ReadbackCheck{
		Name: "source_coverage", Passed: true,
		Detail: fmt.Sprintf("all %d source channels read back exactly once", len(rows)),
	}, true
}

func checkDeliveryCoverage(rows []ChannelMapping) (ReadbackCheck, bool) {
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		position := row.Delivery.Position
		if seen[position] {
			return ReadbackCheck{
				Name: "delivery_coverage", Passed: false,
				Detail: fmt.Sprintf("delivery position %d appears more than once", position),
			}, false
		}
		seen[position] = true
	}
	for position := 1; position <= len(rows); position++ {
		if !seen[position] {
			return ReadbackCheck{
				Name: "delivery_coverage", Passed: false,
				Detail: fmt.Sprintf("delivery position %d is missing", position),
			}, false
		}
	}
	return ReadbackCheck{
		Name: "delivery_coverage", Passed: true,
		Detail: fmt.Sprintf("all %d delivery slots read back exactly once", len(rows)),
	}, true
}

func checkNoOverlap(rows []ChannelMapping, sourceComplete, deliveryComplete bool) ReadbackCheck {
	sourceLabels := make(map[Label]bool, len(rows))
	deliveryLabels := make(map[Label]bool, len(rows))
	for _, row := range rows {
		if sourceLabels[row.Source.Label] {
			return ReadbackCheck{
				Name: "no_overlap", Passed: false,
				Detail: fmt.Sprintf("source label %s appears more than once", row.Source.Label),
			}
		}
		if deliveryLabels[row.Delivery.Label] {
			return ReadbackCheck{
				Name: "no_overlap", Passed: false,
				Detail: fmt.Sprintf("delivery label %s appears more than once", row.Delivery.Label),
			}
		}
		sourceLabels[row.Source.Label] = true
		deliveryLabels[row.Delivery.Label] = true
	}
	if !sourceComplete || !deliveryComplete {
		return ReadbackCheck{Name: "no_overlap", Passed: false, Detail: "coverage is incomplete"}
	}
	return ReadbackCheck{Name: "no_overlap", Passed: true, Detail: "source and delivery rows are unique"}
}

func checkSourceOrder(actual []Label, expected *Plan) ReadbackCheck {
	if expected == nil {
		return ReadbackCheck{Name: "source_order_matches_container", Passed: false,
			Detail: "expected plan is nil"}
	}
	expectedOrder := make([]Label, len(expected.Container.Channels))
	for i, channel := range expected.Container.Channels {
		expectedOrder[i] = channel.Label
	}
	if !reflect.DeepEqual(actual, expectedOrder) {
		return ReadbackCheck{
			Name: "source_order_matches_container", Passed: false,
			Detail: fmt.Sprintf("container order %v, readback order %v", expectedOrder, actual),
		}
	}
	return ReadbackCheck{
		Name: "source_order_matches_container", Passed: true,
		Detail: "readback preserves container encoded order",
	}
}

func checkDeliveryOrder(actual []Label, expected *Plan) ReadbackCheck {
	if expected == nil {
		return ReadbackCheck{Name: "delivery_order_matches_map", Passed: false,
			Detail: "expected plan is nil"}
	}
	expectedOrder := make([]Label, len(expected.Delivery.Channels))
	for i, channel := range expected.Delivery.Channels {
		expectedOrder[i] = channel.Label
	}
	if !reflect.DeepEqual(actual, expectedOrder) {
		return ReadbackCheck{
			Name: "delivery_order_matches_map", Passed: false,
			Detail: fmt.Sprintf("delivery map order %v, readback order %v", expectedOrder, actual),
		}
	}
	return ReadbackCheck{
		Name: "delivery_order_matches_map", Passed: true,
		Detail: "readback reconstructs the external delivery order",
	}
}

func checkPlanEquality(actual, expected *Plan) ReadbackCheck {
	if expected == nil || !reflect.DeepEqual(actual, expected) {
		return ReadbackCheck{
			Name: "mapping_matches_validation", Passed: false,
			Detail: "persisted mapping differs from the validated mapping",
		}
	}
	return ReadbackCheck{
		Name: "mapping_matches_validation", Passed: true,
		Detail: "every persisted mapping row matches the validated plan",
	}
}
