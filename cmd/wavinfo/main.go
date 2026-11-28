// Command wavinfo reads and rewrites the LIST/INFO tags of WAV files while
// leaving all other RIFF chunks (in particular the audio data chunk)
// byte-for-byte untouched.
//
// Usage:
//
//	wavinfo fields
//	wavinfo inspect [-json] FILE
//	wavinfo get FILE FIELD
//	wavinfo set FILE FIELD VALUE
//	wavinfo delete FILE FIELD
//	wavinfo apply [-json METADATA.json] FILE
//
// The -prefer flag of "apply" selects conflict resolution: embedded (the
// default), project or filename.
package main

import (
	"flag"
	"fmt"
	"os"

	"wavinfo"
)

func main() {
	prefer := flag.String("prefer", "embedded",
		"conflict resolution for apply: embedded|project|filename")
	flag.Parse()

	opts := wavinfo.CLIOptions{
		Args:   flag.Args(),
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	switch *prefer {
	case "embedded":
		opts.Preference = wavinfo.PreferEmbedded
	case "project":
		opts.Preference = wavinfo.PreferProject
	case "filename", "file", "name":
		opts.Preference = wavinfo.PreferFileName
	default:
		fmt.Fprintf(os.Stderr, "wavinfo: unknown -prefer value %q\n", *prefer)
		os.Exit(2)
	}

	if err := wavinfo.RunCLI(opts); err != nil {
		code := 1
		if cliErr, ok := err.(*wavinfo.CLIError); ok {
			code = cliErr.Code
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(code)
	}
}
