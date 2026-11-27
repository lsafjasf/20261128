// Command lc3demo builds a synthetic re-packetized LC3 stream and prints the
// exact PCM-sample timeline, including loss gaps and a truncated tail frame.
// It demonstrates why a fixed-duration player drifts when the encoder switches
// between 10 ms and 7.5 ms frames.
package main

import (
	"fmt"

	"lc3timeline/lc3"
)

func main() {
	const rate = uint32(48000)

	// Two 10 ms packets, then a switch to 7.5 ms. One packet is dropped, and
	// the final packet carries a short (truncated) tail frame.
	p0, _ := lc3.Packetize(0, lc3.Dur10ms, []int{120, 120}, make([]byte, 240))
	p1, _ := lc3.Packetize(2, lc3.Dur10ms, []int{120, 120}, make([]byte, 240))
	p2, _ := lc3.Packetize(4, lc3.Dur7p5ms, []int{90, 90}, make([]byte, 180))
	// packet with seqs 6,7 intentionally missing (lost)
	p4, _ := lc3.Packetize(8, lc3.Dur7p5ms, []int{90}, make([]byte, 90))
	tail, _ := lc3.Packetize(9, lc3.Dur7p5ms, []int{90}, make([]byte, 40)) // truncated

	raw, err := lc3.MarshalPackets([]lc3.Packet{p0, p1, p2, p4, tail})
	if err != nil {
		panic(err)
	}
	frames, err := lc3.ParsePacketStream(raw)
	if err != nil {
		panic(err)
	}
	tl, err := lc3.Build(rate, frames)
	if err != nil {
		panic(err)
	}

	fmt.Printf("sample rate: %d Hz\n\n", rate)
	fmt.Printf("%-5s %-6s %-8s %-14s %-8s %s\n", "kind", "seq", "octets", "sample interval", "length", "note")
	for _, seg := range tl.Segments {
		kind, note := "frame", ""
		if seg.Kind == lc3.KindGap {
			kind, note = "GAP", fmt.Sprintf("%d frame(s) lost", seg.Lost)
		} else if seg.Octets > 0 && len(payloadOf(frames, seg.Seq)) < seg.Octets {
			note = "truncated tail"
		}
		fmt.Printf("%-5s %-6d %-8d [%-5d,%-5d)   %-8d %s\n",
			kind, seg.Seq, seg.Octets, seg.Start, seg.End, seg.Length(), note)
	}

	fmt.Printf("\nreceived=%d lost=%d tailTruncated=%v\n", tl.Received, tl.Lost, tl.TailTruncated)
	fmt.Printf("exact total: %d samples (%.3f ms)\n", tl.TotalSamples(),
		float64(tl.TotalSamples())/float64(rate)*1000)

	naive := uint64(tl.Received+tl.Lost) * 480
	fmt.Printf("fixed-10ms (buggy) total: %d samples -> drift of %d samples\n",
		naive, int64(naive)-int64(tl.TotalSamples()))
}

func payloadOf(frames []lc3.Frame, seq uint16) []byte {
	for _, f := range frames {
		if f.Seq == seq {
			return f.Payload
		}
	}
	return nil
}
