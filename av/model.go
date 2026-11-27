package av

import "avsync/mp4"

// SampleModel is one sample with fully resolved timing, in decode order.
type SampleModel struct {
	Index int
	DTS   int64 // decode timestamp, track timescale ticks
	CTS   int64 // composition (presentation) timestamp, ticks
	Dur   int64 // duration, ticks
	Sync  bool
}

// TrackModel is the analysis view of one track.
type TrackModel struct {
	Handler   string
	Timescale int64
	Samples   []SampleModel
	Elst      []mp4.ElstEntry
}

// Model is the analysis view of a whole file.
type Model struct {
	MovieTimescale int64
	Video          *TrackModel
	Audio          *TrackModel
}

// BuildModel extracts the timing model from a demuxed MP4.
func BuildModel(f *mp4.File) *Model {
	m := &Model{MovieTimescale: int64(f.MovieTimescale)}
	for _, t := range f.Tracks {
		tm := &TrackModel{
			Handler:   t.Handler,
			Timescale: int64(t.Timescale),
			Elst:      t.Elst,
		}
		for _, s := range t.Samples {
			tm.Samples = append(tm.Samples, SampleModel{
				Index: s.Index,
				DTS:   s.DTS,
				CTS:   s.CTS,
				Dur:   s.Dur,
				Sync:  s.Sync,
			})
		}
		switch t.Handler {
		case "vide":
			if m.Video == nil {
				m.Video = tm
			}
		case "soun":
			if m.Audio == nil {
				m.Audio = tm
			}
		}
	}
	return m
}

// Ms converts track ticks to milliseconds.
func (t *TrackModel) Ms(ticks int64) float64 {
	return float64(ticks) * 1000 / float64(t.Timescale)
}

// Cuts returns the decode-order indices of video sync samples that start a
// new shot (every sync sample except the first one).
func (m *Model) Cuts() []int {
	var cuts []int
	for _, s := range m.Video.Samples {
		if s.Sync && s.Index > 0 {
			cuts = append(cuts, s.Index)
		}
	}
	return cuts
}

// ReorderDelay is the maximum CTS-DTS offset across video samples.
func (m *Model) ReorderDelay() int64 {
	var max int64
	for _, s := range m.Video.Samples {
		if d := s.CTS - s.DTS; d > max {
			max = d
		}
	}
	return max
}

// FrameDur is the modal video sample duration.
func (m *Model) FrameDur() int64 {
	counts := map[int64]int{}
	for _, s := range m.Video.Samples {
		counts[s.Dur]++
	}
	var best int64
	var bestN int
	for d, n := range counts {
		if n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

// AudioFrameDur is the modal audio sample duration.
func (m *Model) AudioFrameDur() int64 {
	counts := map[int64]int{}
	for _, s := range m.Audio.Samples {
		counts[s.Dur]++
	}
	var best int64
	var bestN int
	for d, n := range counts {
		if n > bestN {
			best, bestN = d, n
		}
	}
	return best
}
