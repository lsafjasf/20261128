package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	dir := flag.String("dir", ".", "directory containing the exported image sequence")
	ext := flag.String("ext", "", "only include files with this extension, e.g. .png (empty = all files)")
	pattern := flag.String("pattern", "", "regex with one capture group for the sequence number (default: last digit run)")
	fps := flag.Float64("fps", 0, "export frame rate; required unless two reference timestamps are given")
	start := flag.String("start", "", "RFC3339 timestamp of the anchor frame (default anchor: first frame)")
	startSeq := flag.Int("start-seq", -1, "sequence number of -start (default: first exported frame)")
	ref2 := flag.String("ref2", "", "RFC3339 timestamp of a second reference frame, used to derive fps")
	ref2Seq := flag.Int("ref2-seq", -1, "sequence number of -ref2")
	flag.Parse()

	if err := run(*dir, *ext, *pattern, *fps, *start, *startSeq, *ref2, *ref2Seq); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dir, ext, pattern string, fps float64, start string, startSeq int, ref2 string, ref2Seq int) error {
	names, err := listFiles(dir, ext)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no image files found in %s", dir)
	}
	frames, err := CollectFrames(names, pattern)
	if err != nil {
		return err
	}

	ref, err := buildReference(frames, fps, start, startSeq, ref2, ref2Seq)
	if err != nil {
		return err
	}

	fmt.Printf("frames exported : %d\n", len(frames))
	fmt.Printf("sequence range  : %d - %d\n", frames[0].Seq, frames[len(frames)-1].Seq)
	fmt.Printf("frame rate      : %.6g fps\n", ref.FPS)
	fmt.Printf("real duration   : %s (from sequence numbers, not file count)\n", ref.Duration(frames))
	fmt.Println()
	fmt.Print(FormatTimeline(ref, frames))

	gaps := FindGaps(frames)
	if len(gaps) == 0 {
		fmt.Println("no gaps: sequence is complete")
		return nil
	}
	total := 0
	for _, g := range gaps {
		total += g.Len()
	}
	fmt.Printf("\ngaps: %d gap(s), %d frame(s) missing\n", len(gaps), total)

	first := gaps[0]
	fmt.Printf("\nfirst gap after frame #%d (%d missing), fill candidates:\n", first.AfterSeq, first.Len())
	for _, c := range FillCandidates(ref, first) {
		fmt.Printf("  #%06d  expected at %s\n", c.Seq, c.Time.Format(time.RFC3339Nano))
	}
	return nil
}

func listFiles(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext != "" && !strings.EqualFold(filepath.Ext(e.Name()), ext) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func buildReference(frames []Frame, fps float64, start string, startSeq int, ref2 string, ref2Seq int) (Reference, error) {
	anchorSeq := startSeq
	if anchorSeq < 0 {
		anchorSeq = frames[0].Seq
	}
	var anchorTime time.Time
	if start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return Reference{}, fmt.Errorf("bad -start: %w", err)
		}
		anchorTime = t
	}
	if ref2 != "" {
		t2, err := time.Parse(time.RFC3339, ref2)
		if err != nil {
			return Reference{}, fmt.Errorf("bad -ref2: %w", err)
		}
		if ref2Seq < 0 {
			return Reference{}, fmt.Errorf("-ref2-seq is required with -ref2")
		}
		return NewReference(anchorSeq, anchorTime, ref2Seq, t2)
	}
	if fps <= 0 {
		return Reference{}, fmt.Errorf("-fps must be positive (or provide -ref2/-ref2-seq to derive it)")
	}
	return Reference{AnchorSeq: anchorSeq, AnchorTime: anchorTime, FPS: fps}, nil
}
