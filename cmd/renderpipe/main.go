package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"renderpipe"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

const usage = `Usage:
  renderpipe render --project DIR --output DIR [--json]
  renderpipe plan   --project DIR --output DIR [--json]
  renderpipe verify [--json] ARTIFACT_DIR

The task ID, source hash, artifact directory, frame order, and file names are
derived only from project content and render settings. They never contain the
server's local timezone.
`

func runRender(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", "", "project root")
	output := flags.String("output", "", "output root")
	flags.StringVar(output, "out", "", "alias for --output")
	asJSON := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "render does not accept positional arguments")
		return 2
	}

	result, err := renderpipe.Render(*project, *output)
	if err != nil {
		fmt.Fprintf(stderr, "render failed: %v\n", err)
		return 1
	}
	if *asJSON {
		return printJSON(stdout, stderr, result)
	}
	status := "created"
	if result.Reused {
		status = "reused"
	}
	fmt.Fprintf(stdout, "status=%s\n", status)
	fmt.Fprintf(stdout, "task_id=%s\n", result.TaskID)
	fmt.Fprintf(stdout, "source_sha256=%s\n", result.SourceHash)
	fmt.Fprintf(stdout, "config_sha256=%s\n", result.ConfigHash)
	fmt.Fprintf(stdout, "frame_set_sha256=%s\n", result.FrameSetHash)
	fmt.Fprintf(stdout, "media_sha256=%s\n", result.MediaHash)
	fmt.Fprintf(stdout, "artifact=%s\n", result.Directory)
	fmt.Fprintf(stdout, "media=%s\n", result.MediaPath)
	return 0
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", "", "project root")
	output := flags.String("output", "", "output root")
	flags.StringVar(output, "out", "", "alias for --output")
	asJSON := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "plan does not accept positional arguments")
		return 2
	}

	plan, err := renderpipe.PlanTask(*project, *output)
	if err != nil {
		fmt.Fprintf(stderr, "plan failed: %v\n", err)
		return 1
	}
	if *asJSON {
		return printJSON(stdout, stderr, plan)
	}
	fmt.Fprintf(stdout, "task_id=%s\n", plan.TaskID)
	fmt.Fprintf(stdout, "source_sha256=%s\n", plan.SourceHash)
	fmt.Fprintf(stdout, "config_sha256=%s\n", plan.ConfigHash)
	fmt.Fprintf(stdout, "artifact=%s\n", plan.Directory)
	fmt.Fprintf(stdout, "frame_pattern=%s\n", plan.FramePattern)
	return 0
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	jsonAtEnd := false
	if len(args) > 0 && args[len(args)-1] == "--json" {
		jsonAtEnd = true
		args = args[:len(args)-1]
	}
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(stderr)
	asJSON := flags.Bool("json", jsonAtEnd, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "verify requires exactly one artifact directory")
		return 2
	}

	manifest, err := renderpipe.VerifyArtifact(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "verify failed: %v\n", err)
		return 1
	}
	if *asJSON {
		return printJSON(stdout, stderr, manifest)
	}
	fmt.Fprintf(stdout, "status=valid\n")
	fmt.Fprintf(stdout, "task_id=%s\n", manifest.TaskID)
	fmt.Fprintf(stdout, "source_sha256=%s\n", manifest.SourceHash)
	fmt.Fprintf(stdout, "frame_set_sha256=%s\n", manifest.FrameSetHash)
	fmt.Fprintf(stdout, "media_sha256=%s\n", manifest.Media.SHA256)
	return 0
}

func printJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "encode output: %v\n", err)
		return 1
	}
	return 0
}
