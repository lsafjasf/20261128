package ltc

import (
	"fmt"
	"sort"
)

func BuildAnalysis(timelines []Timeline) Analysis {
	a := Analysis{
		SchemaVersion: 1,
		Assumptions: []string{
			"The LTC time address itself has no calendar date; dates are accepted only from SMPTE ST 309 user bits.",
			"Frame order in each input is treated as observed order. A missing-date midnight wrap increments only a relative day.",
			"DDMMYY dates are century-ambiguous; six-digit MJD dates are absolute.",
			"Drop-frame uses nominal 30 fps and 29.97 counting, dropping labels 00 and 01 at every minute not divisible by ten.",
		},
		Sources: timelines,
	}
	a.CrossSource = CrossSource(timelines)
	return a
}

func CrossSource(timelines []Timeline) Merge {
	m := Merge{Ordering: "ambiguous"}
	var ambiguousReasons []string
	for _, t := range timelines {
		for _, s := range t.Segments {
			item := MergeItem{
				Source:        t.Label,
				Segment:       s.Index,
				StartTimecode: s.StartTimecode,
				EndTimecode:   s.EndTimecode,
				StartDate:     s.StartDate,
				StartUTC:      s.StartUTC,
				DateStatus:    s.DateStatus,
				Chronology:    s.Chronology,
			}
			m.Items = append(m.Items, item)
			if s.StartUTC == "" {
				ambiguousReasons = append(ambiguousReasons,
					fmt.Sprintf("%s segment %d has no absolute date", t.Label, s.Index))
			} else if s.Ambiguous {
				ambiguousReasons = append(ambiguousReasons,
					fmt.Sprintf("%s segment %d contains unresolved chronology", t.Label, s.Index))
			}
		}
	}
	if len(m.Items) == 0 {
		m.Reason = "no timeline segments to merge"
		return m
	}
	if len(ambiguousReasons) > 0 {
		m.Reason = ambiguousReasons[0]
		if len(ambiguousReasons) > 1 {
			m.Reason += fmt.Sprintf(" (and %d more)", len(ambiguousReasons)-1)
		}
		return m
	}
	sort.SliceStable(m.Items, func(i, j int) bool {
		if m.Items[i].StartUTC == m.Items[j].StartUTC {
			if m.Items[i].Source == m.Items[j].Source {
				return m.Items[i].Segment < m.Items[j].Segment
			}
			return m.Items[i].Source < m.Items[j].Source
		}
		return m.Items[i].StartUTC < m.Items[j].StartUTC
	})
	m.Ordering = "determined"
	m.Reason = "all segments have absolute ST 309 dates and continuous chronology"
	return m
}
