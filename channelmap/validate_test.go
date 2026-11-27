package channelmap

import (
	"errors"
	"testing"
)

func testLayout(labels ...Label) Layout {
	return Layout{
		Container: "test-container",
		Codec:     "PCM",
		Channels:  makeChannels(labels),
		Source:    "test.wav",
	}
}

func deliveryFromLabels(name string, labels ...Label) DeliveryMap {
	channels := make([]DeliveryChannel, len(labels))
	for i, label := range labels {
		channels[i] = DeliveryChannel{
			Position: i + 1,
			Label:    label,
			Name:     label.Name(),
		}
	}
	return DeliveryMap{Name: name, Channels: channels}
}

func TestBuildPlanCenterSurroundPermutation(t *testing.T) {
	layout := testLayout(LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround)
	delivery := deliveryFromLabels("broadcast-5.1",
		LabelLeft, LabelRight, LabelLeftSurround, LabelRightSurround,
		LabelCenter, LabelLFE)

	plan, err := BuildPlan(layout, delivery)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.Identity {
		t.Fatal("BuildPlan() marked a reordered delivery as identity")
	}
	if len(plan.Mappings) != 6 {
		t.Fatalf("got %d mapping rows, want 6", len(plan.Mappings))
	}

	wantSourceByDelivery := []int{0, 1, 4, 5, 2, 3}
	for i, wantSource := range wantSourceByDelivery {
		got := plan.Mappings[i]
		if got.Delivery.Position != i+1 {
			t.Errorf("row %d delivery position = %d, want %d", i, got.Delivery.Position, i+1)
		}
		if got.Source.Index != wantSource {
			t.Errorf("delivery %d (%s) source index = %d, want %d",
				got.Delivery.Position, got.Delivery.Label, got.Source.Index, wantSource)
		}
		wantReordered := wantSource != i
		if got.Reordered != wantReordered {
			t.Errorf("delivery %d reordered = %t, want %t",
				got.Delivery.Position, got.Reordered, wantReordered)
		}
	}
}

func TestBuildPlanIdentity(t *testing.T) {
	labels := []Label{LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround}
	plan, err := BuildPlan(testLayout(labels...), deliveryFromLabels("delivery", labels...))
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if !plan.Identity {
		t.Fatal("BuildPlan() should mark identical order as identity")
	}
	for _, row := range plan.Mappings {
		if row.Reordered {
			t.Errorf("identity row for %s marked reordered", row.Source.Label)
		}
	}
}

func TestBuildPlanRejectsOverlap(t *testing.T) {
	layout := testLayout(LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround)
	delivery := deliveryFromLabels("duplicate",
		LabelLeft, LabelRight, LabelLeftSurround, LabelLeftSurround,
		LabelCenter, LabelLFE)

	_, err := BuildPlan(layout, delivery)
	if !IsRejectionCode(err, RejectOverlap) {
		t.Fatalf("BuildPlan() error = %v, want overlap rejection", err)
	}
}

func TestBuildPlanRejectsDuplicateDeliveryPosition(t *testing.T) {
	delivery := DeliveryMap{Channels: []DeliveryChannel{
		{Position: 1, Label: LabelLeft},
		{Position: 1, Label: LabelRight},
	}}
	_, err := BuildPlan(testLayout(LabelLeft, LabelRight), delivery)
	if !IsRejectionCode(err, RejectOverlap) {
		t.Fatalf("BuildPlan() error = %v, want overlap rejection", err)
	}
}

func TestBuildPlanRejectsIncomplete(t *testing.T) {
	layout := testLayout(LabelLeft, LabelRight, LabelCenter, LabelLFE,
		LabelLeftSurround, LabelRightSurround)
	delivery := deliveryFromLabels("missing",
		LabelLeft, LabelRight, LabelCenter, LabelLFE, LabelLeftSurround)

	_, err := BuildPlan(layout, delivery)
	if !IsRejectionCode(err, RejectIncomplete) {
		t.Fatalf("BuildPlan() error = %v, want incomplete rejection", err)
	}
}

func TestBuildPlanRejectsMissingSourceLabel(t *testing.T) {
	layout := testLayout(LabelLeft, LabelRight, LabelCenter)
	delivery := deliveryFromLabels("wrong", LabelLeft, LabelRight, LabelLeftSurround)

	_, err := BuildPlan(layout, delivery)
	if !IsRejectionCode(err, RejectIncomplete) {
		t.Fatalf("BuildPlan() error = %v, want incomplete rejection", err)
	}
}

func TestBuildPlanRejectsExplicitSourceOverlap(t *testing.T) {
	delivery := DeliveryMap{Channels: []DeliveryChannel{
		{Position: 1, Label: LabelLeft, SourceRef: "FL"},
		{Position: 2, Label: LabelRight, SourceRef: "FL"},
	}}
	_, err := BuildPlan(testLayout(LabelLeft, LabelRight), delivery)
	if !IsRejectionCode(err, RejectOverlap) {
		t.Fatalf("BuildPlan() error = %v, want overlap rejection", err)
	}
}

func TestBuildPlanRejectsExplicitMismatch(t *testing.T) {
	delivery := DeliveryMap{Channels: []DeliveryChannel{
		{Position: 1, Label: LabelLeft, SourceRef: "FR"},
		{Position: 2, Label: LabelRight, SourceRef: "FL"},
	}}
	_, err := BuildPlan(testLayout(LabelLeft, LabelRight), delivery)
	if !IsRejectionCode(err, RejectMismatch) {
		t.Fatalf("BuildPlan() error = %v, want mismatch rejection", err)
	}
}

func TestBuildPlanAcceptsExplicitIndexBindingsForGenericChannels(t *testing.T) {
	layout := testLayout("CH1", "CH2")
	delivery := DeliveryMap{Channels: []DeliveryChannel{
		{Position: 1, Label: LabelLeft, SourceRef: "0"},
		{Position: 2, Label: LabelRight, SourceRef: "1"},
	}}
	plan, err := BuildPlan(layout, delivery)
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.Mappings[0].Source.Label != "CH1" {
		t.Fatalf("first source = %s, want CH1", plan.Mappings[0].Source.Label)
	}
}

func TestParseDeliveryRejectsPartialSourceBindings(t *testing.T) {
	data := []byte(`{
		"channels": [
			{"slot": 1, "label": "L", "source": "FL"},
			{"slot": 2, "label": "R"}
		]
	}`)
	_, err := ParseDelivery(data)
	if !IsRejectionCode(err, RejectIncomplete) {
		t.Fatalf("ParseDelivery() error = %v, want incomplete rejection", err)
	}
}

func TestParseDeliveryRejectsEmptyMapAsIncomplete(t *testing.T) {
	_, err := ParseDelivery([]byte(`{"channels": []}`))
	if !IsRejectionCode(err, RejectIncomplete) {
		t.Fatalf("ParseDelivery() error = %v, want incomplete rejection", err)
	}
}

func TestParseDeliverySourcePositionIsOneBased(t *testing.T) {
	delivery, err := ParseDelivery([]byte(`{
		"channels": [
			{"slot": 1, "label": "L", "source_position": 2},
			{"slot": 2, "label": "R", "source_position": 1}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}
	if delivery.Channels[0].SourceRef != "1" || delivery.Channels[1].SourceRef != "0" {
		t.Fatalf("source positions parsed as %q/%q, want 1/0",
			delivery.Channels[0].SourceRef, delivery.Channels[1].SourceRef)
	}
}

func TestParseDeliveryLabelPositionObject(t *testing.T) {
	delivery, err := ParseDelivery([]byte(`{"L": 1, "R": 2}`))
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}
	if len(delivery.Channels) != 2 {
		t.Fatalf("got %d channels, want 2", len(delivery.Channels))
	}
	if delivery.Channels[0].Label != LabelLeft || delivery.Channels[0].Position != 1 {
		t.Fatalf("unexpected first channel: %+v", delivery.Channels[0])
	}
}

func TestParseDeliverySlotLabelObject(t *testing.T) {
	delivery, err := ParseDelivery([]byte(`{"channels": {"2": "R", "1": "L"}}`))
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}
	if len(delivery.Channels) != 2 {
		t.Fatalf("got %d channels, want 2", len(delivery.Channels))
	}
	if delivery.Channels[0].Position != 1 || delivery.Channels[0].Label != LabelLeft {
		t.Fatalf("unexpected first channel: %+v", delivery.Channels[0])
	}
}

func TestParseDeliveryChannelMapObject(t *testing.T) {
	delivery, err := ParseDelivery([]byte(`{"channel_map": {"L": 1, "R": 2}}`))
	if err != nil {
		t.Fatalf("ParseDelivery() error = %v", err)
	}
	if len(delivery.Channels) != 2 || delivery.Channels[1].Label != LabelRight {
		t.Fatalf("unexpected delivery channels: %+v", delivery.Channels)
	}
}

func TestRejectionErrorCarriesIssues(t *testing.T) {
	layout := testLayout(LabelLeft, LabelRight)
	delivery := deliveryFromLabels("duplicate", LabelLeft, LabelLeft)
	_, err := BuildPlan(layout, delivery)
	var rejectionError *RejectionError
	if !errors.As(err, &rejectionError) {
		t.Fatalf("error = %v, want *RejectionError", err)
	}
	if len(rejectionError.Issues) == 0 {
		t.Fatal("rejection contains no issues")
	}
}
