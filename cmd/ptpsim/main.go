package main

import (
	"flag"
	"fmt"
	"os"

	"ptpfailover/internal/ptpsim"
)

func main() {
	jsonOutput := flag.Bool("json", false, "emit the report as JSON")
	events := flag.Bool("events", false, "print the PTP event log")
	flag.Parse()

	report := ptpsim.Run(ptpsim.DefaultConfig())
	if *events {
		ptpsim.PrintEvents(os.Stdout, report.Events)
	}
	if *jsonOutput {
		if err := ptpsim.WriteJSON(os.Stdout, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	ptpsim.WriteReport(os.Stdout, report)
}
