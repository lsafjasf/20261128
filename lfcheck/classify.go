package lfcheck

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	strongCorrelation = 0.85
	polarityCutoff    = 0.90
	nearZeroMS        = 0.10
	delayCutoffMS     = 0.10
	routingGainDB     = 12.0
	lfeSmallDBFS      = -35.0
	lfeSmallRelative  = -20.0
)

func classify(result *Analysis) {
	levels := make(map[string]float64, len(result.ChannelInfo))
	active := make(map[string]bool, len(result.ChannelInfo))
	mainMax := -180.0
	for _, channel := range result.ChannelInfo {
		levels[channel.Role] = channel.LowRMSDBFS
		active[channel.Role] = channel.Active
		if channel.Role != "LFE" {
			mainMax = math.Max(mainMax, channel.LowRMSDBFS)
		}
	}
	lfeSmall := levels["LFE"] <= lfeSmallDBFS ||
		levels["LFE"]-mainMax <= lfeSmallRelative

	routed := classifyLFERouting(result, levels, active, lfeSmall)
	inverted := classifyPolarity(result, active)
	delayed := classifyDelay(result, active)

	switch {
	case routed && (inverted || delayed):
		result.Assessment = "LFE is low and coherent with the main channels; both routing/phase and timing evidence require correction."
	case routed:
		result.Assessment = "The low LFE signal is coherently present in one or more main channels at substantially higher gain: evidence of LFE rerouting or leakage, not normal main-channel music."
	case inverted:
		result.Assessment = "A common low-frequency waveform has opposite polarity across channel groups: the listed channels are phase-inverted relative to the reference group."
	case delayed:
		result.Assessment = "A common low-frequency waveform is time-shifted between channels: check routing, alignment, and bass-management bypass."
	default:
		result.Assessment = normalAssessment(result, levels, active, lfeSmall)
	}
}

func classifyLFERouting(result *Analysis, levels map[string]float64, active map[string]bool, lfeSmall bool) bool {
	if !lfeSmall || levels["LFE"] <= -90 {
		return false
	}
	var affected []string
	var strongest PairResult
	strongestGain := -180.0
	for _, role := range []string{"L", "R", "C"} {
		if !active[role] {
			continue
		}
		pair, ok := findPair(result, "LFE", role)
		if !ok {
			continue
		}
		pair = pairFromA(pair, "LFE")
		if math.Abs(pair.Best) < strongCorrelation ||
			math.Abs(pair.LagMS) > 1.0 ||
			pair.GainDB < routingGainDB {
			continue
		}
		affected = append(affected, role)
		if pair.GainDB > strongestGain {
			strongestGain = pair.GainDB
			strongest = pair
		}
	}
	if len(affected) == 0 {
		return false
	}
	code := "lfe_leakage"
	severity := "warning"
	if len(affected) > 1 {
		code = "lfe_rerouted"
		severity = "error"
	}
	lfeLevel := levels["LFE"]
	message := fmt.Sprintf(
		"LFE is %.1f dBFS, while its low-frequency waveform appears in %s with up to %+.1f dB makeup gain (r=%+.3f at %+.3f ms).",
		lfeLevel, strings.Join(affected, "/"), strongest.GainDB,
		strongest.Best, strongest.LagMS)
	result.Findings = append(result.Findings, Finding{
		Code:        code,
		Severity:    severity,
		Channels:    affected,
		Pair:        "LFE->" + strings.Join(affected, "/"),
		Correlation: strongest.Best,
		LagSamples:  strongest.LagSamples,
		LagMS:       strongest.LagMS,
		GainDB:      strongest.GainDB,
		Message:     message,
	})
	return true
}

func classifyPolarity(result *Analysis, active map[string]bool) bool {
	anchor := ""
	for _, role := range []string{"L", "R", "C", "LFE"} {
		if active[role] {
			anchor = role
			break
		}
	}
	if anchor == "" {
		return false
	}
	signs := map[string]int{anchor: 1}
	queue := []string{anchor}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, other := range []string{"L", "R", "C", "LFE"} {
			if other == current {
				continue
			}
			if _, seen := signs[other]; seen {
				continue
			}
			if !active[current] || !active[other] {
				continue
			}
			pair, ok := findPair(result, current, other)
			if !ok || math.Abs(pair.Best) < polarityCutoff ||
				math.Abs(pair.LagMS) > nearZeroMS {
				continue
			}
			sign := 1
			if pair.Best < 0 {
				sign = -1
			}
			signs[other] = signs[current] * sign
			queue = append(queue, other)
		}
	}
	var reference, opposite []string
	for _, role := range []string{"L", "R", "C", "LFE"} {
		sign, ok := signs[role]
		if !ok {
			continue
		}
		if sign > 0 {
			reference = append(reference, role)
		} else {
			opposite = append(opposite, role)
		}
	}
	bestEdge := PairResult{}
	for _, role := range opposite {
		for _, ref := range reference {
			pair, found := findPair(result, role, ref)
			if found && math.Abs(pair.Best) > math.Abs(bestEdge.Best) {
				bestEdge = pair
			}
		}
	}
	if len(opposite) == 0 {
		return false
	}
	sort.Strings(opposite)
	sort.Strings(reference)
	message := fmt.Sprintf(
		"%s %s opposite polarity to the reference group %s in the low band (strongest r=%+.3f at %+.3f ms); this is a 180-degree phase/polarity fault candidate.",
		strings.Join(opposite, "/"), pluralVerb(opposite, "has", "have"),
		strings.Join(reference, "/"), bestEdge.Best, bestEdge.LagMS)
	result.Findings = append(result.Findings, Finding{
		Code:        "polarity_inversion",
		Severity:    "error",
		Channels:    opposite,
		Reference:   reference,
		Pair:        bestEdge.A + "/" + bestEdge.B,
		Correlation: bestEdge.Best,
		LagSamples:  bestEdge.LagSamples,
		LagMS:       bestEdge.LagMS,
		Message:     message,
	})
	return true
}

func classifyDelay(result *Analysis, active map[string]bool) bool {
	found := false
	for _, pair := range result.Pairs {
		if !active[pair.A] || !active[pair.B] {
			continue
		}
		if math.Abs(pair.Best) < strongCorrelation ||
			pair.LagAtSearchEdge ||
			math.Abs(pair.LagMS) < delayCutoffMS {
			continue
		}
		if pair.Best < 0 && math.Abs(pair.LagMS) <= nearZeroMS {
			continue
		}
		delayed, reference := pair.B, pair.A
		lagMS := pair.LagMS
		lagSamples := pair.LagSamples
		if lagMS < 0 {
			delayed, reference = pair.A, pair.B
			lagMS = -lagMS
			lagSamples = -lagSamples
		}
		message := fmt.Sprintf(
			"%s is delayed by %.3f ms (%d samples at %d Hz) relative to %s; low-band correlation is %+.3f.",
			delayed, lagMS, lagSamples, result.SampleRate, reference, pair.Best)
		result.Findings = append(result.Findings, Finding{
			Code:        "delay_mismatch",
			Severity:    "warning",
			Channels:    []string{delayed},
			Reference:   []string{reference},
			Pair:        reference + "/" + delayed,
			Correlation: pair.Best,
			LagSamples:  lagSamples,
			LagMS:       lagMS,
			Message:     message,
		})
		found = true
	}
	return found
}

func normalAssessment(result *Analysis, levels map[string]float64, active map[string]bool, lfeSmall bool) string {
	sharedMain := false
	maxMainPair := 0.0
	for _, pair := range result.Pairs {
		if pair.A == "LFE" || pair.B == "LFE" {
			continue
		}
		if active[pair.A] && active[pair.B] && math.Abs(pair.Best) >= 0.70 {
			sharedMain = true
			maxMainPair = math.Max(maxMainPair, math.Abs(pair.Best))
		}
	}
	lfeCoherent := false
	for _, role := range []string{"L", "R", "C"} {
		pair, ok := findPair(result, "LFE", role)
		if ok && active[role] && math.Abs(pair.Best) >= 0.70 {
			lfeCoherent = true
		}
	}
	if sharedMain {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Main channels share low-frequency content (max |r|=%.2f); without coherence to LFE this is reported as program music, not leakage.", maxMainPair))
	}
	if lfeCoherent && !lfeSmall {
		return "Low-frequency content is coherent with LFE, but the LFE level is not anomalous. Treat this as shared/intentional LFE content unless source-stem routing proves otherwise."
	}
	if lfeSmall && sharedMain && !lfeCoherent {
		result.Warnings = append(result.Warnings,
			"LFE is low and the main channels share bass, but no usable LFE reference remains; signal-only analysis cannot prove rerouting.")
		return "Low-frequency energy is shared by main channels, but it is not coherent with LFE. Treat it as normal program bass unless source-stem routing proves otherwise."
	}
	if lfeSmall && lfeCoherent {
		return "LFE is low and weakly coherent with the mains, but the correlation/gain evidence does not reach the conservative routing threshold. Review the source stems before correcting routing."
	}
	if lfeSmall {
		return "LFE is low, but no coherent copy of it was found in L/R/C. Low-frequency music in the mains is not, by itself, classified as leakage."
	}
	return "No coherent LFE routing, polarity inversion, or timing mismatch was found in the selected low band."
}

func periodicWarnings(result Analysis) []string {
	var warnings []string
	for _, pair := range result.Pairs {
		if math.Abs(pair.Best) >= 0.95 && math.Abs(pair.ZeroLag) >= 0.90 &&
			math.Abs(pair.LagMS) >= delayCutoffMS {
			warnings = append(warnings,
				fmt.Sprintf("%s/%s is strongly correlated at both zero lag and %+.3f ms; a narrow-band waveform can make lag ambiguous by whole periods.",
					pair.A, pair.B, pair.LagMS))
		}
	}
	return warnings
}

func pluralVerb(values []string, singular, plural string) string {
	if len(values) == 1 {
		return singular
	}
	return plural
}
