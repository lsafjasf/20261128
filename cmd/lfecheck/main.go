package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/lsafjasf/20261128/lfcheck"
)

const (
	exitClean  = 0
	exitIssues = 1
	exitUsage  = 2
)

type options struct {
	json     bool
	markdown bool
	layout   string
	low      float64
	high     float64
	maxLagMS float64
	paths    []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, code, stop := parseOptions(args, stderr)
	if stop {
		return code
	}
	var results []lfcheck.Analysis
	for _, path := range opts.paths {
		result, err := lfcheck.AnalyzeFile(path, lfcheck.Options{
			LowHz: opts.low, HighHz: opts.high, MaxLagMS: opts.maxLagMS,
			Layout: opts.layout,
		})
		if err != nil {
			fmt.Fprintf(stderr, "input error: %v\n", err)
			return exitUsage
		}
		results = append(results, result)
	}

	if opts.json {
		encodeJSON(stdout, results)
	} else if opts.markdown {
		writeMarkdown(stdout, results)
	} else {
		writeText(stdout, results)
	}

	for _, result := range results {
		if len(result.Findings) > 0 {
			return exitIssues
		}
	}
	return exitClean
}

func parseOptions(args []string, stderr io.Writer) (options, int, bool) {
	opts := options{}
	flags := flag.NewFlagSet("lfecheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&opts.json, "json", false, "emit machine-readable JSON")
	flags.BoolVar(&opts.markdown, "markdown", false, "emit Markdown")
	flags.StringVar(&opts.layout, "layout", "", "override encoded order, e.g. L,R,C,LFE,Ls,Rs")
	flags.Float64Var(&opts.low, "low-hz", 20, "low-band high-pass edge in Hz")
	flags.Float64Var(&opts.high, "high-hz", 120, "low-band low-pass edge in Hz")
	flags.Float64Var(&opts.maxLagMS, "max-lag-ms", 50, "maximum absolute lag searched")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: lfecheck [options] <program.wav> [program.wav ...]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, exitClean, true
		}
		return options{}, exitUsage, true
	}
	opts.paths = flags.Args()
	if len(opts.paths) == 0 {
		fmt.Fprintln(stderr, "at least one WAV file is required")
		flags.Usage()
		return options{}, exitUsage, true
	}
	if opts.json && opts.markdown {
		fmt.Fprintln(stderr, "--json and --markdown are mutually exclusive")
		return options{}, exitUsage, true
	}
	if opts.low < 0 || opts.high <= opts.low || opts.maxLagMS <= 0 {
		fmt.Fprintln(stderr, "invalid low/high band or lag limit")
		return options{}, exitUsage, true
	}
	return opts, exitClean, false
}

func encodeJSON(writer io.Writer, results []lfcheck.Analysis) {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if len(results) == 1 {
		_ = encoder.Encode(results[0])
		return
	}
	_ = encoder.Encode(results)
}

func writeText(writer io.Writer, results []lfcheck.Analysis) {
	for index, result := range results {
		if index > 0 {
			fmt.Fprintln(writer)
		}
		fmt.Fprintf(writer, "%s  %.3f s  %d Hz  %d channels\n",
			filepath.Base(result.Source), result.Duration, result.SampleRate, result.Channels)
		fmt.Fprintf(writer, "  layout: %s   band: %.0f-%.0f Hz\n",
			strings.Join(result.Layout, " "), result.LowHz, result.HighHz)
		fmt.Fprintln(writer, "  levels (low-band RMS):")
		for _, channel := range result.ChannelInfo {
			flagText := ""
			if !channel.Active {
				flagText = "  [low]"
			}
			fmt.Fprintf(writer, "    %-3s ch%d %-4s %8.2f dBFS  %+6.2f dB rel main%s\n",
				channel.Role, channel.Index+1, channel.Label,
				channel.LowRMSDBFS, channel.LowRelativeToMainDB, flagText)
		}
		fmt.Fprintln(writer, "  pairs:")
		fmt.Fprintln(writer, "    pair        zero      best     lag(ms)   gain(A->B)")
		for _, pair := range result.Pairs {
			if pair.LagReliable {
				fmt.Fprintf(writer, "    %-8s %+9.3f %+9.3f %+9.3f   %+7.2f dB\n",
					pair.A+"/"+pair.B, pair.ZeroLag, pair.Best, pair.LagMS, pair.GainDB)
			} else {
				fmt.Fprintf(writer, "    %-8s %+9.3f %+9.3f %9s   %9s\n",
					pair.A+"/"+pair.B, pair.ZeroLag, pair.Best, "-", "-")
			}
		}
		if len(result.Findings) == 0 {
			fmt.Fprintln(writer, "  findings: none")
		} else {
			for _, finding := range result.Findings {
				fmt.Fprintf(writer, "  FINDING [%s/%s] %s\n",
					finding.Code, finding.Severity, finding.Message)
			}
		}
		for _, warning := range result.Warnings {
			fmt.Fprintf(writer, "  WARNING: %s\n", warning)
		}
		fmt.Fprintf(writer, "  assessment: %s\n", result.Assessment)
	}
}

func writeMarkdown(writer io.Writer, results []lfcheck.Analysis) {
	fmt.Fprintln(writer, "# Low-frequency routing report")
	fmt.Fprintln(writer)
	for _, result := range results {
		fmt.Fprintf(writer, "## %s\n\n", filepath.Base(result.Source))
		fmt.Fprintf(writer, "`%d Hz`, `%d ch`, `%.3f s`, band `%.0f-%.0f Hz`  \n",
			result.SampleRate, result.Channels, result.Duration, result.LowHz, result.HighHz)
		fmt.Fprintf(writer, "Layout: `%s`\n\n", strings.Join(result.Layout, " "))
		fmt.Fprintln(writer, "| role | ch | label | low RMS dBFS | rel main dB |")
		fmt.Fprintln(writer, "|---|---:|---|---:|---:|")
		for _, channel := range result.ChannelInfo {
			fmt.Fprintf(writer, "| %s | %d | %s | %.2f | %+.2f |\n",
				channel.Role, channel.Index+1, channel.Label,
				channel.LowRMSDBFS, channel.LowRelativeToMainDB)
		}
		fmt.Fprintln(writer)
		fmt.Fprintln(writer, "| pair | zero-lag r | best r | lag ms | gain A->B dB |")
		fmt.Fprintln(writer, "|---|---:|---:|---:|---:|")
		for _, pair := range result.Pairs {
			if pair.LagReliable {
				fmt.Fprintf(writer, "| %s/%s | %+.3f | %+.3f | %+.3f | %+.2f |\n",
					pair.A, pair.B, pair.ZeroLag, pair.Best, pair.LagMS, pair.GainDB)
			} else {
				fmt.Fprintf(writer, "| %s/%s | %+.3f | %+.3f | - | - |\n",
					pair.A, pair.B, pair.ZeroLag, pair.Best)
			}
		}
		fmt.Fprintln(writer)
		if len(result.Findings) == 0 {
			fmt.Fprintln(writer, "**Findings:** none.")
		} else {
			fmt.Fprintln(writer, "**Findings:**")
			for _, finding := range result.Findings {
				fmt.Fprintf(writer, "- `%s` (%s): %s\n", finding.Code, finding.Severity, finding.Message)
			}
		}
		for _, warning := range result.Warnings {
			fmt.Fprintf(writer, "- Note: %s\n", warning)
		}
		fmt.Fprintf(writer, "\n**Assessment:** %s\n\n", result.Assessment)
	}
}
