package webvtt

import (
	"testing"
	"time"
)

func TestMergeOverlappingCuesSameSpeaker(t *testing.T) {
	cues := []Cue{
		testCue("a", 0, seconds(2), `<v Alice>one</v>`),
		testCue("b", seconds(1), seconds(3), `<v Alice>two</v>`),
	}
	got := MergeOverlapping(cues)
	if len(got) != 1 {
		t.Fatalf("got %d cues, want 1", len(got))
	}
	if got[0].Start != 0 || got[0].End != 3*time.Second {
		t.Fatalf("timing = %v -> %v", got[0].Start, got[0].End)
	}
	if got[0].Payload != "<v Alice>one</v>\n<v Alice>two</v>" {
		t.Fatalf("payload = %q", got[0].Payload)
	}
	if got[0].Text("") != "one\ntwo" {
		t.Fatalf("text = %q", got[0].Text(""))
	}
}

func TestMergeOverlappingCuesDifferentSpeakersStaySeparate(t *testing.T) {
	cues := []Cue{
		testCue("a", 0, seconds(3), `<v Alice>one</v>`),
		testCue("b", seconds(1), seconds(2), `<v Bob>two</v>`),
		testCue("c", seconds(1), seconds(2), `<v Alice>three</v>`),
	}
	got := MergeOverlapping(cues)
	if len(got) != 2 {
		t.Fatalf("got %d cues, want 2: %#v", len(got), got)
	}
	if got[0].Payload != "<v Alice>one</v>\n<v Alice>three</v>" {
		t.Fatalf("first cue payload = %q", got[0].Payload)
	}
	if got[1].Payload != "<v Bob>two</v>" {
		t.Fatalf("second cue payload = %q", got[1].Payload)
	}
}

func TestMergeOverlappingCuesDoesNotMixUntaggedAndVoiced(t *testing.T) {
	cues := []Cue{
		testCue("a", 0, seconds(2), "plain"),
		testCue("b", seconds(1), seconds(3), `<v Alice>voiced</v>`),
	}
	if got := MergeOverlapping(cues); len(got) != 2 {
		t.Fatalf("got %d cues, want 2", len(got))
	}
}

func TestMergeOverlappingCuesKeepsContainingEnd(t *testing.T) {
	cues := []Cue{
		testCue("a", 0, seconds(10), `<v Alice>long</v>`),
		testCue("b", seconds(1), seconds(2), `<v Alice>short</v>`),
	}
	got := MergeOverlapping(cues)
	if len(got) != 1 || got[0].End != 10*time.Second {
		t.Fatalf("merged cues = %#v", got)
	}
}

func TestMergeOverlappingCuesSpeakerSetsMustMatch(t *testing.T) {
	cues := []Cue{
		testCue("a", 0, seconds(3), `<v Alice><v Bob>one</v></v>`),
		testCue("b", seconds(1), seconds(2), `<v Bob><v Alice>two</v></v>`),
		testCue("c", seconds(1), seconds(2), `<v Alice>three</v>`),
	}
	got := MergeOverlapping(cues)
	if len(got) != 2 {
		t.Fatalf("got %d cues, want 2", len(got))
	}
	if len(got[0].Speakers()) != 2 || len(got[1].Speakers()) != 1 {
		t.Fatalf("speakers were mixed: %#v", got)
	}
}

func TestMergeAdjacentOption(t *testing.T) {
	cues := []Cue{
		testCue("a", 0, time.Second, `<v Alice>one</v>`),
		testCue("b", 2*time.Second, 3*time.Second, `<v Alice>two</v>`),
	}
	if got := MergeCues(cues, MergeOptions{}); len(got) != 2 {
		t.Fatalf("default merge produced %d cues", len(got))
	}
	got := MergeCues(cues, MergeOptions{MergeAdjacent: true, MaxGap: time.Second})
	if len(got) != 1 {
		t.Fatalf("gap merge produced %d cues", len(got))
	}
}

func testCue(id string, start, end time.Duration, payload string) Cue {
	return Cue{
		ID:      id,
		Start:   start,
		End:     end,
		Payload: payload,
		Nodes:   ParseCueMarkup(payload),
	}
}

func seconds(n int) time.Duration {
	return time.Duration(n) * time.Second
}
