package av

import "avsync/mp4"

// BuildFixedTracks converts the demuxed file into mux-ready tracks with the
// diagnosed defect corrected. The correction is localized at each cut: it
// removes the per-cut discontinuities instead of applying one constant
// offset to the whole audio track.
func BuildFixedTracks(f *mp4.File, diag *Diagnosis) ([]*mp4.MuxTrack, uint32) {
	movieTS := f.MovieTimescale
	if movieTS == 0 {
		movieTS = 1000
	}
	m := BuildModel(f)
	var out []*mp4.MuxTrack
	for _, t := range f.Tracks {
		mt := &mp4.MuxTrack{
			ID:        t.ID,
			Handler:   t.Handler,
			Codec:     t.Codec,
			Timescale: t.Timescale,
			Elst:      t.Elst,
		}
		for _, s := range t.Samples {
			mt.Samples = append(mt.Samples, mp4.MuxSample{
				DTS:  s.DTS,
				CTS:  s.CTS,
				Dur:  s.Dur,
				Sync: s.Sync,
				Data: append([]byte{}, f.Data(t, s)...),
			})
		}
		switch {
		case t.Handler == "soun" && diag.Cause == CauseReorderWait:
			removeTimelineJumps(mt.Samples, m.AudioFrameDur())
		case t.Handler == "vide" && diag.Cause == CauseClockReset:
			removeTimelineJumps(mt.Samples, m.FrameDur())
		case t.Handler == "vide" && diag.Cause == CauseEditList:
			mt.Elst = correctedEditList(mt, movieTS)
		}
		out = append(out, mt)
	}
	return out, movieTS
}

// removeTimelineJumps re-stamps a track's decode timeline so that the step
// between consecutive samples is exactly the nominal sample duration. Only
// samples after a detected jump are moved, and only by the accumulated
// excess: the first samples keep their original timestamps.
func removeTimelineJumps(samples []mp4.MuxSample, nominalDur int64) {
	if len(samples) == 0 {
		return
	}
	tol := nominalDur / 2
	var excess int64
	for i := 1; i < len(samples); i++ {
		samples[i].DTS -= excess
		samples[i].CTS -= excess
		step := samples[i].DTS - samples[i-1].DTS
		if step > nominalDur+tol {
			excess += step - nominalDur
			samples[i].DTS -= step - nominalDur
			samples[i].CTS -= step - nominalDur
		}
	}
}

// correctedEditList replaces a defective edit list with a single entry that
// maps the first presented frame to movie time zero. If the media already
// starts at zero, the edit list is dropped entirely.
func correctedEditList(tr *mp4.MuxTrack, movieTS uint32) []mp4.ElstEntry {
	if len(tr.Samples) == 0 {
		return nil
	}
	minCTS := tr.Samples[0].CTS
	var mediaDur int64
	for _, s := range tr.Samples {
		if s.CTS < minCTS {
			minCTS = s.CTS
		}
		mediaDur += s.Dur
	}
	if minCTS == 0 {
		return nil
	}
	return []mp4.ElstEntry{{
		SegmentDuration: mediaDur * int64(movieTS) / int64(tr.Timescale),
		MediaTime:       minCTS,
		RateInteger:     1,
	}}
}
