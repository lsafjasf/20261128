package main

import (
	"fmt"
	"io"

	"dsdtable/dsd"
)

func printManifest(w io.Writer, m *dsd.Manifest) {
	name := m.Path
	if name == "" {
		name = "(memory)"
	}
	fmt.Fprintf(w, "file: %s\n", name)
	fmt.Fprintf(w, "format: %s  channels: %d  bit-rate: %d Hz  packing: %s  bit-order: %s\n",
		m.Format, m.Channels, m.SampleRate, m.Packing, m.BitOrder)
	fmt.Fprintf(w, "audio-bytes: %d  valid-samples(total): %d\n", m.AudioBytes, m.TotalValidSamples())
	if m.BlockSize > 0 {
		fmt.Fprintf(w, "block-size: %d bytes/channel  declared-samples/channel: %d\n",
			m.BlockSize, m.DeclaredSamples)
	}

	fmt.Fprintln(w, "\nCHANNEL TABLE")
	fmt.Fprintln(w, "CH  ID      VALID_BYTES  VALID_SAMPLES  PAD_BYTES  PAD_BITS  MISSING")
	for _, channel := range m.ChannelTables {
		fmt.Fprintf(w, "%-3d %-7s %11d  %13d  %9d  %8d  %7d\n",
			channel.Index, channel.ID, channel.ValidBytes, channel.ValidSamples,
			channel.PaddingBytes, channel.PaddingBits, channel.MissingBytes)
		for _, segment := range channel.Segments {
			fmt.Fprintf(w, "    seg chunk=%d offset=%d stride=%d unit=%d units=%d valid=%d",
				segment.Chunk, segment.Offset, segment.Stride, segment.UnitBytes,
				segment.Units, segment.ValidBytes)
			if segment.TailLayout != "" {
				fmt.Fprintf(w, " tail=%s@%d stored=%d valid=%d pad=%d missing=%d",
					segment.TailLayout, segment.TailOffset, segment.TailStoredBytes,
					segment.TailValidBytes, segment.TailPaddingBytes, segment.TailMissingBytes)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintln(w, "\nPADDING")
	if len(m.Padding) == 0 {
		fmt.Fprintln(w, "none")
	} else {
		for _, padding := range m.Padding {
			fmt.Fprintf(w, "kind=%-20s chunk=%d channel=%d offset=%d length=%d bits=%d synthetic=%t",
				padding.Kind, padding.Chunk, padding.Channel, padding.FileOffset,
				padding.Length, padding.Bits, padding.Synthetic)
			if padding.Reason != "" {
				fmt.Fprintf(w, "  # %s", padding.Reason)
			}
			fmt.Fprintln(w)
		}
	}
	if len(m.Warnings) > 0 {
		fmt.Fprintln(w, "\nWARNINGS")
		for _, warning := range m.Warnings {
			fmt.Fprintf(w, "- %s\n", warning)
		}
	}
}
