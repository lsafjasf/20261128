package gen

import "avsync/mp4"

// Defect selects which muxing bug the synthetic file exhibits.
type Defect int

const (
	DefectNone        Defect = iota // clean reference file
	DefectReorderWait               // audio PTS jumps at cuts (held during reorder flush)
	DefectEditList                  // per-shot elst entries double-count the reorder delay
	DefectClockReset                // video DTS/CTS restart with an extra offset per shot
)

// ParseDefect maps a CLI name to a Defect.
func ParseDefect(s string) (Defect, bool) {
	switch s {
	case "none":
		return DefectNone, true
	case "reorder":
		return DefectReorderWait, true
	case "elst":
		return DefectEditList, true
	case "clockreset":
		return DefectClockReset, true
	}
	return DefectNone, false
}

const (
	videoTimescale = 90000
	frameDur       = 3600 // 25 fps
	reorderDepth   = 2
	reorderDelay   = reorderDepth * frameDur // 80 ms
	audioTimescale = 48000
	audioFrameDur  = 1024
	movieTimescale = 1000
)

// Generate writes a synthetic MP4 with the given defect. Video payloads are
// dummy bytes; only the timing tables matter for the analysis.
func Generate(path string, defect Defect, shots, framesPerShot int) error {
	vt := &mp4.MuxTrack{ID: 1, Handler: "vide", Codec: "avc1",
		Timescale: videoTimescale, Width: 1280, Height: 720}
	at := &mp4.MuxTrack{ID: 2, Handler: "soun", Codec: "mp4a", Timescale: audioTimescale}

	shotDur := int64(framesPerShot) * frameDur
	var videoClockShift int64 // cumulative per-shot shift for clock-reset
	for shot := 0; shot < shots; shot++ {
		base := int64(shot) * shotDur
		if defect == DefectClockReset && shot > 0 {
			videoClockShift += reorderDelay
		}
		// Decode order with reorderDepth 2: 0, 3, 1, 2, 6, 4, 5, ...
		order := decodeOrder(framesPerShot)
		for di, pi := range order {
			dts := base + int64(di)*frameDur + videoClockShift
			cts := base + int64(pi)*frameDur + videoClockShift
			vt.Samples = append(vt.Samples, mp4.MuxSample{
				DTS:  dts,
				CTS:  cts,
				Dur:  frameDur,
				Sync: pi == 0,
				Data: videoPayload(shot, pi),
			})
		}
		if defect == DefectEditList {
			vt.Elst = append(vt.Elst, mp4.ElstEntry{
				SegmentDuration: shotDur * movieTimescale / videoTimescale,
				MediaTime:       base + int64(shot+1)*reorderDelay,
				RateInteger:     1,
			})
		}
	}

	// Audio timeline in audio ticks; reorderDelay converted to audio ticks.
	audioDelay := int64(reorderDelay) * audioTimescale / videoTimescale
	totalAudio := int64(shots) * shotDur * audioTimescale / videoTimescale
	var audioShift int64
	cut := 1 // next cut index to apply for the reorder defect
	for pts := int64(0); pts < totalAudio; pts += audioFrameDur {
		if defect == DefectReorderWait {
			cutPres := int64(cut) * shotDur * audioTimescale / videoTimescale
			for cut < shots && pts >= cutPres {
				audioShift += audioDelay
				cut++
				cutPres = int64(cut) * shotDur * audioTimescale / videoTimescale
			}
		}
		at.Samples = append(at.Samples, mp4.MuxSample{
			DTS:  pts + audioShift,
			CTS:  pts + audioShift,
			Dur:  audioFrameDur,
			Sync: true,
			Data: audioPayload(pts),
		})
	}
	return mp4.Mux(path, []*mp4.MuxTrack{vt, at}, movieTimescale)
}

// decodeOrder returns presentation indices in decode order for one shot,
// emulating an IBBPBBP... pattern with two B-frames.
func decodeOrder(n int) []int {
	var order []int
	if n == 0 {
		return order
	}
	order = append(order, 0)
	for p := 3; p < n; p += 3 {
		order = append(order, p, p-2, p-1)
	}
	// Tail: remaining frames in presentation order.
	next := ((n - 1) / 3) * 3
	if (n-1)%3 == 0 && n > 1 {
		next = n - 1
	}
	seen := make(map[int]bool, len(order))
	for _, v := range order {
		seen[v] = true
	}
	for i := next; i < n; i++ {
		if !seen[i] {
			order = append(order, i)
		}
	}
	return order
}

func videoPayload(shot, frame int) []byte {
	b := make([]byte, 400+(shot*37+frame*11)%200)
	for i := range b {
		b[i] = byte(shot*31 + frame*7 + i)
	}
	return b
}

func audioPayload(pts int64) []byte {
	b := make([]byte, 180)
	for i := range b {
		b[i] = byte(pts>>3) + byte(i)
	}
	return b
}
