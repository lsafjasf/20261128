package ptpsim

import (
	"math"
	"sort"
	"time"
)

type capturePacket struct {
	recorder string
	seq      int
	samples  int
	at       time.Duration
	arrival  int
}

func simulateBuffer(cfg Config) BufferMetrics {
	if !cfg.Reorder.Enabled {
		return BufferMetrics{}
	}
	streams := []struct {
		id   string
		asym time.Duration
	}{{recorderA, cfg.PathAsymmetry.A}, {recorderB, cfg.PathAsymmetry.B}}

	packetSamples := int(math.Round(cfg.SampleRate * cfg.PacketInterval.Seconds()))
	var all []capturePacket
	var inversions int
	for _, stream := range streams {
		packets, count := reorderPackets(cfg, stream.id, packetSamples)
		all = append(all, packets...)
		inversions += count
	}

	before := 0
	for _, packet := range all {
		before += packet.samples
	}
	dropped := cfg.Reorder.Drops * packetSamples * len(streams)
	duplicated := cfg.Reorder.Duplicates * packetSamples * len(streams)
	after := before - dropped + duplicated

	return BufferMetrics{
		Packets:           len(all),
		ArrivalInversions: inversions,
		Drops:             cfg.Reorder.Drops * len(streams),
		Duplicates:        cfg.Reorder.Duplicates * len(streams),
		SamplesBefore:     before,
		SamplesAfter:      after,
		NetSampleDelta:    after - before,
	}
}

func reorderPackets(cfg Config, recorder string, packetSamples int) ([]capturePacket, int) {
	n := int(cfg.Duration / cfg.PacketInterval)
	packets := make([]capturePacket, 0, n)
	for i := 0; i < n; i++ {
		at := time.Duration(i) * cfg.PacketInterval
		packets = append(packets, capturePacket{
			recorder: recorder,
			seq:      i,
			samples:  packetSamples,
			at:       at,
			arrival:  i,
		})
	}

	swapWindow := cfg.Reorder.SwapWindow
	if swapWindow < 1 {
		swapWindow = 1
	}
	every := cfg.Reorder.Every
	if every < 1 {
		every = 1
	}
	for i := range packets {
		if packets[i].at < cfg.Reorder.Start || packets[i].at >= cfg.Reorder.End {
			continue
		}
		if i%every != 0 || i+swapWindow >= len(packets) {
			continue
		}
		packets[i].arrival, packets[i+swapWindow].arrival =
			packets[i+swapWindow].arrival, packets[i].arrival
	}

	var inversions int
	for i := 0; i < len(packets); i++ {
		for j := i + 1; j < len(packets); j++ {
			if packets[i].arrival > packets[j].arrival {
				inversions++
			}
		}
	}

	sort.Slice(packets, func(i, j int) bool {
		return packets[i].seq < packets[j].seq
	})
	return packets, inversions
}
