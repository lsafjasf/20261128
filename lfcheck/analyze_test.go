package lfcheck

import (
	"math"
	"strings"
	"testing"
)

const testRate = 48000

func testSignal(seconds int, phase float64, freqs ...float64) []float64 {
	return tone(seconds*testRate, testRate, phase, freqs...)
}

func broadLowSignal(seconds int, phase float64) []float64 {
	freqs := make([]float64, 28)
	for i := range freqs {
		freqs[i] = 31 + 3.1*float64(i)
	}
	return testSignal(seconds, phase, freqs...)
}

func TestNormalIndependentBass(t *testing.T) {
	n := 6 * testRate
	audio := sixChannels(
		testSignal(6, 0.1, 55, 75),
		testSignal(6, 1.7, 65, 95),
		testSignal(6, 0.9, 100, 110),
		testSignal(6, 2.4, 35, 85),
		n,
	)
	result, err := AnalyzeAudio(audio, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("unexpected findings: %+v", result.Findings)
	}
}

func TestSharedMainMusicIsNotLeakage(t *testing.T) {
	n := 6 * testRate
	music := testSignal(6, 0.2, 55, 80, 105)
	audio := sixChannels(
		music,
		music,
		scale(music, 0.5),
		testSignal(6, 1.1, 35, 70),
		n,
	)
	result, err := AnalyzeAudio(audio, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("shared program bass was classified as a fault: %+v", result.Findings)
	}
}

func TestLowLFEWithSharedMainMusicNeedsReference(t *testing.T) {
	n := 6 * testRate
	music := testSignal(6, 0.2, 55, 80, 105)
	audio := sixChannels(
		music,
		scale(music, 0.9),
		scale(music, 0.4),
		testSignal(6, 1.1, 35, 70),
		n,
	)
	audio.Samples[3] = scale(audio.Samples[3], 0.003)
	result, err := AnalyzeAudio(audio, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("normal main bass was classified as leakage: %+v", result.Findings)
	}
	if !containsText(result.Warnings, "cannot prove rerouting") {
		t.Fatalf("missing evidence warning: %+v", result.Warnings)
	}
}

func TestLowLERoutingToMains(t *testing.T) {
	n := 6 * testRate
	source := testSignal(6, 0.4, 40, 60, 95)
	audio := sixChannels(
		source,
		source,
		scale(source, 0.8),
		scale(source, 0.002),
		n,
	)
	result, err := AnalyzeAudio(audio, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(result, "lfe_rerouted") {
		t.Fatalf("missing rerouting finding: %+v", result.Findings)
	}
	finding := result.Findings[0]
	if len(finding.Channels) != 3 {
		t.Fatalf("affected channels = %v, want L/R/C", finding.Channels)
	}
	if finding.GainDB < 30 {
		t.Fatalf("makeup gain = %.2f dB, want >= 30", finding.GainDB)
	}
}

func TestPolarityInversion(t *testing.T) {
	n := 6 * testRate
	source := testSignal(6, 0.3, 50, 75, 100)
	audio := sixChannels(
		source,
		source,
		scale(source, -1),
		scale(source, -1),
		n,
	)
	result, err := AnalyzeAudio(audio, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(result, "polarity_inversion") {
		t.Fatalf("missing polarity finding: %+v", result.Findings)
	}
	finding := result.Findings[0]
	if len(finding.Channels) != 2 || finding.Correlation > -0.99 {
		t.Fatalf("unexpected polarity evidence: %+v", finding)
	}
}

func TestLFEDelay(t *testing.T) {
	n := 6 * testRate
	source := broadLowSignal(6, 0.7)
	lfeSource := scale(delay(source, 240), 0.5)
	audio := sixChannels(
		source,
		testSignal(6, 1.4, 70, 105),
		testSignal(6, 2.0, 60, 100),
		lfeSource,
		n,
	)
	result, err := AnalyzeAudio(audio, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(result, "delay_mismatch") {
		t.Fatalf("missing delay finding: %+v", result.Findings)
	}
	for _, finding := range result.Findings {
		if finding.Code == "delay_mismatch" &&
			(math.Abs(finding.LagMS-5.0) > 0.15 || finding.Channels[0] != "LFE") {
			t.Fatalf("delay evidence = %+v, want LFE at 5 ms", finding)
		}
	}
}

func containsText(values []string, text string) bool {
	for _, value := range values {
		if strings.Contains(value, text) {
			return true
		}
	}
	return false
}
