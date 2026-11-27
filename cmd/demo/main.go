// Demo: evaluate export ranges with a 2s transition handle on each side.
package main

import (
	"fmt"

	"exporter/handle"
)

func report(name string, p handle.RangePlan) {
	fmt.Printf("%-22s range [%6.1fs,%6.1fs)  window [%6.1fs,%6.1fs)\n",
		name,
		float64(p.In)/1000, float64(p.Out)/1000,
		float64(p.ExportStart)/1000, float64(p.ExportEnd)/1000)
	for _, b := range []struct {
		tag string
		b   handle.Boundary
	}{{"head", p.Head}, {"tail", p.Tail}} {
		fmt.Printf("  %-4s avail=%5.1fs deficit=%4.1fs freeze=%4.1fs -> %s\n",
			b.tag,
			float64(b.b.Available)/1000,
			float64(b.b.Deficit)/1000,
			float64(b.b.FreezeMs)/1000,
			b.b.Status)
	}
}

func main() {
	const (
		duration = 60_000 // 60s clip
		handleMs = 2_000  // 2s transition each side
	)
	cases := []struct {
		name    string
		in, out int64
	}{
		{"both lossless", 10_000, 50_000},
		{"head freeze", 1_000, 50_000},
		{"tail freeze", 10_000, 59_500},
		{"both reencode", 0, 60_000},
	}
	for _, c := range cases {
		p, err := handle.Plan(duration, c.in, c.out, handleMs)
		if err != nil {
			fmt.Println(c.name, "error:", err)
			continue
		}
		report(c.name, p)
	}

	// Range adjustment keeps subtitles and audio in sync.
	p, _ := handle.Plan(duration, 10_000, 50_000, handleMs)
	cues := []handle.Cue{{Start: 12_000, End: 14_000, Text: "hello"}}
	tracks := []handle.AudioTrack{{SourceStart: 9_000, Duration: 40_000}}
	l, _ := handle.Adjust(p, 8_000, 50_000, cues, tracks)
	fmt.Printf("after in 10s->8s: cue offset %.1fs, audio trim %.1fs\n",
		float64(l.Cues[0].Offset)/1000, float64(l.Audio[0].TrimHead)/1000)
}
