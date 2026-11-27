package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"opusmap"
)

func main() {
	jsonOutput := flag.Bool("json", false, "emit the complete report as JSON")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: opusmap [-json] FILE\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	name := "-"
	if flag.NArg() == 1 {
		name = flag.Arg(0)
	}
	input := io.Reader(os.Stdin)
	var file *os.File
	if name != "-" {
		var err error
		file, err = os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "opusmap: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	}

	report, err := opusmap.Scan(input)
	if err != nil {
		if report != nil && *jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(report)
		}
		fmt.Fprintf(os.Stderr, "opusmap: %v\n", err)
		os.Exit(1)
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "opusmap: %v\n", err)
			os.Exit(1)
		}
	} else {
		printReport(report)
	}
	if !report.Valid() {
		os.Exit(2)
	}
}

func printReport(report *opusmap.Report) {
	h := report.Head
	fmt.Printf(
		"OpusHead v%d: channels=%d pre-skip=%d input-rate=%d gain=%d "+
			"family=%d N=%d M=%d mapping=%v\n",
		h.Version, h.ChannelCount, h.PreSkip, h.InputSampleRate, h.OutputGain,
		h.MappingFamily,
		h.StreamCount, h.CoupledCount, h.Mapping,
	)
	if h.MappingFamily == 2 {
		fmt.Printf("family 2 layout: order=%d non-diegetic-stereo=%t\n",
			report.Mapping.AmbisonicOrder, report.Mapping.NonDiegeticStereo)
	}
	fmt.Printf("decoded channels=%d audio-packets=%d\n",
		report.Mapping.DecodedChannels, len(report.Packets))
	for _, stream := range report.Mapping.Streams {
		kind := "mono"
		if stream.Coupled {
			kind = "coupled stereo"
		}
		fmt.Printf("stream %d: %s, used-by=%v\n", stream.Stream, kind, stream.UsedBy)
	}
	for _, channel := range report.Mapping.Channels {
		source := channel.Source
		switch source.Kind {
		case opusmap.SourceSilence:
			fmt.Printf("channel %d: %s <- silence\n", channel.Channel, channel.Name)
		case opusmap.SourceInvalid:
			fmt.Printf(
				"channel %d: %s <- INVALID decoded index %d\n",
				channel.Channel, channel.Name, channel.MappingIndex,
			)
		default:
			fmt.Printf(
				"channel %d: %s <- %s (stream %d, decoded index %d)\n",
				channel.Channel, channel.Name, source.Kind,
				source.Stream, source.DecodedIndex,
			)
		}
	}
	if len(report.Issues) == 0 {
		fmt.Println("ok: mapping and packet framing are consistent")
		return
	}
	fmt.Printf("issues (%d):\n", len(report.Issues))
	for _, issue := range report.Issues {
		fmt.Printf("  [%s] %s\n", issue.Code, issue.Error())
	}
}
