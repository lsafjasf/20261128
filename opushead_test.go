package opusmap

import (
	"encoding/binary"
	"strings"
	"testing"
)

func testHead(channels, family, streams, coupled int, mapping []byte) []byte {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1
	head[9] = byte(channels)
	binary.LittleEndian.PutUint16(head[10:12], 312)
	binary.LittleEndian.PutUint32(head[12:16], 48000)
	head[18] = byte(family)
	if family == 0 {
		return head
	}
	head = append(head, byte(streams), byte(coupled))
	head = append(head, mapping...)
	return head
}

func TestFamily2ContradictoryMappingStopsAfterValidPrefix(t *testing.T) {
	head := testHead(6, 2, 2, 1, []byte{0, 1, 2, 3, 4, 5})
	parsed := DecodeOpusHead(head)
	if parsed.Head == nil {
		t.Fatal("expected partial OpusHead")
	}
	if got := len(parsed.Issues); got != 3 {
		t.Fatalf("got %d mapping issues, want 3: %v", got, parsed.Issues)
	}
	for i, issue := range parsed.Issues {
		if issue.Code != CodeIllegalMapping {
			t.Errorf("issue %d code = %q", i, issue.Code)
		}
		if issue.PacketSeq != 0 {
			t.Errorf("issue %d packet = %d, want 0", i, issue.PacketSeq)
		}
		if issue.Channel != i+3 {
			t.Errorf("issue %d channel = %d, want %d", i, issue.Channel, i+3)
		}
	}

	mapping, _ := ReconstructMapping(parsed.Head)
	for channel := 0; channel < 3; channel++ {
		if mapping.Channels[channel].Source.Kind == SourceInvalid {
			t.Errorf("channel %d should have a decoded source", channel)
		}
	}
	for channel := 3; channel < 6; channel++ {
		if mapping.Channels[channel].Source.Kind != SourceInvalid {
			t.Errorf("channel %d source = %q, want invalid",
				channel, mapping.Channels[channel].Source.Kind)
		}
	}
}

func TestFamily2ValidMappingAndCoupling(t *testing.T) {
	head := testHead(6, 2, 3, 2, []byte{0, 1, 2, 3, 4, 255})
	parsed := DecodeOpusHead(head)
	if len(parsed.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", parsed.Issues)
	}
	mapping, issues := ReconstructMapping(parsed.Head)
	if len(issues) != 0 {
		t.Fatalf("unexpected reconstruction issues: %v", issues)
	}
	if mapping.AmbisonicOrder != 1 || !mapping.NonDiegeticStereo {
		t.Fatalf("layout = order %d, non-diegetic %t",
			mapping.AmbisonicOrder, mapping.NonDiegeticStereo)
	}
	if !mapping.Streams[0].Coupled || !mapping.Streams[1].Coupled ||
		mapping.Streams[2].Coupled {
		t.Fatalf("unexpected coupling: %+v", mapping.Streams)
	}
	if mapping.Channels[0].Source.Kind != SourceStereoLeft ||
		mapping.Channels[1].Source.Kind != SourceStereoRight {
		t.Fatalf("first stereo pair not reconstructed: %+v", mapping.Channels[:2])
	}
	if mapping.Channels[5].Source.Kind != SourceSilence {
		t.Fatalf("channel 5 source = %q, want silence",
			mapping.Channels[5].Source.Kind)
	}
	if !strings.Contains(mapping.Channels[4].Name, "non-diegetic left") {
		t.Fatalf("channel 4 name = %q", mapping.Channels[4].Name)
	}
}

func TestFamily2ChannelCountValidation(t *testing.T) {
	valid := []int{1, 3, 4, 6, 9, 11, 16, 18}
	for _, channels := range valid {
		if !validAmbisonicChannelCount(channels) {
			t.Errorf("channel count %d should be valid", channels)
		}
	}
	for _, channels := range []int{0, 2, 5, 7, 8, 10, 12} {
		if validAmbisonicChannelCount(channels) {
			t.Errorf("channel count %d should be invalid", channels)
		}
	}
}

func TestMalformedCouplingDoesNotPanic(t *testing.T) {
	head := testHead(6, 2, 1, 2, []byte{0, 1, 2, 3, 4, 5})
	parsed := DecodeOpusHead(head)
	if len(parsed.Issues) == 0 {
		t.Fatal("expected malformed coupling issue")
	}
	mapping, issues := ReconstructMapping(parsed.Head)
	if len(issues) == 0 {
		t.Fatal("expected reconstruction issue")
	}
	if mapping.Channels[0].Source.Kind == SourceInvalid {
		t.Fatal("valid stereo channel should still be reconstructed")
	}
}
