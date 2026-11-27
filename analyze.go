package opusmap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// PacketReport is the framing result for one Ogg audio packet.
type PacketReport struct {
	PacketSeq   uint64       `json:"packet_seq"`
	PageSeq     uint32       `json:"page_seq"`
	Granule     int64        `json:"granule"`
	Size        int          `json:"size"`
	Duration48k int          `json:"duration_48k"`
	Streams     []OpusPacket `json:"streams,omitempty"`
}

// Report is the complete OpusHead, mapping, framing, and issue set.
type Report struct {
	Head    OpusHead       `json:"head"`
	Mapping Mapping        `json:"mapping"`
	Packets []PacketReport `json:"packets"`
	Issues  []Issue        `json:"issues"`
}

// Valid reports whether no container, mapping, or packet issues were found.
func (r *Report) Valid() bool {
	return r != nil && len(r.Issues) == 0
}

// Scan reads an Ogg Opus logical stream and validates OpusHead, every packed
// multistream audio packet, and the reconstructed family 2 channel mapping.
// Errors in individual packets are collected in Report.Issues with PacketSeq.
func Scan(r io.Reader) (*Report, error) {
	assemblers := make(map[uint32]*pageAssembler)
	var selected uint32
	selectedSet := false
	var report *Report
	var selectedAssembler *pageAssembler

	for {
		page, err := ReadOggPage(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			return report, err
		}
		if selectedSet && page.Serial != selected {
			continue
		}

		assembler := assemblers[page.Serial]
		if assembler == nil {
			assembler = &pageAssembler{
				serial: page.Serial, nextPage: page.Sequence,
			}
			assemblers[page.Serial] = assembler
		}
		packets := assembler.feed(page)
		for _, packet := range packets {
			if !selectedSet {
				if packet.seq != 0 || !bytes.HasPrefix(packet.data, []byte("OpusHead")) {
					continue
				}
				selected = packet.serial
				selectedSet = true
				selectedAssembler = assembler
			}
			if packet.serial != selected {
				continue
			}
			if report == nil {
				var scanErr error
				report, scanErr = startReport(packet)
				if scanErr != nil {
					return report, scanErr
				}
				report.Issues = append(report.Issues, assembler.takeIssues()...)
				continue
			}
			processPacket(report, packet)
		}
		if selectedSet && page.Serial == selected {
			report.Issues = append(report.Issues, assembler.takeIssues()...)
		}
	}

	if !selectedSet || report == nil {
		return nil, fmt.Errorf("no Opus logical stream found")
	}
	report.Issues = append(report.Issues, selectedAssembler.takeIssues()...)
	if selectedAssembler.partialOpen {
		report.Issues = append(report.Issues, newIssue(
			CodeTruncatedPacket, selectedAssembler.packetSeq,
			selectedAssembler.partialPage, -1, -1,
			"stream ends with an incomplete packet",
		))
	}
	report.Issues = dedupeIssues(report.Issues)
	return report, nil
}

func startReport(packet oggPacket) (*Report, error) {
	parsed := DecodeOpusHead(packet.data)
	if parsed.Head == nil {
		return &Report{Issues: parsed.Issues}, fmt.Errorf(
			"packet %d: cannot parse OpusHead", packet.seq,
		)
	}
	mapping, mappingIssues := ReconstructMapping(parsed.Head)
	return &Report{
		Head:    *parsed.Head,
		Mapping: mapping,
		Issues:  append(parsed.Issues, mappingIssues...),
	}, nil
}

func processPacket(report *Report, packet oggPacket) {
	switch packet.seq {
	case 1:
		if !bytes.HasPrefix(packet.data, []byte("OpusTags")) {
			report.Issues = append(report.Issues, newIssue(
				CodeBadPacket, packet.seq, packet.startPage, -1, -1,
				"second packet is not OpusTags",
			))
		}
		return
	case 0:
		return
	}

	if report.Head.StreamCount <= 0 {
		report.Issues = append(report.Issues, newIssue(
			CodeMissingStream, packet.seq, packet.startPage, -1, -1,
			"cannot split packet because OpusHead stream count is %d",
			report.Head.StreamCount,
		))
		return
	}
	streams, err := ParseMultistreamPacket(packet.data, report.Head.StreamCount)
	entry := PacketReport{
		PacketSeq: packet.seq,
		PageSeq:   packet.endPage,
		Granule:   packet.granule,
		Size:      len(packet.data),
		Streams:   streams,
	}
	if len(streams) != 0 {
		entry.Duration48k = streams[0].Duration48k
	}
	report.Packets = append(report.Packets, entry)
	if err == nil {
		return
	}

	code := CodeBadPacket
	stream := len(streams)
	var missing *missingStreamError
	var mismatch *durationMismatchError
	if errors.As(err, &missing) {
		code = CodeMissingStream
		stream = missing.stream
	} else if errors.As(err, &mismatch) {
		code = CodeDurationMismatch
		stream = mismatch.stream
	}
	report.Issues = append(report.Issues, newIssue(
		code, packet.seq, packet.endPage, stream, -1, "%v", err,
	))
}

func dedupeIssues(issues []Issue) []Issue {
	seen := make(map[string]bool, len(issues))
	out := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		key := fmt.Sprintf(
			"%s|%d|%d|%d|%d|%s",
			issue.Code, issue.PacketSeq, issue.PageSeq,
			issue.Stream, issue.Channel, issue.Message,
		)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, issue)
	}
	return out
}

type oggPacket struct {
	data      []byte
	seq       uint64
	serial    uint32
	startPage uint32
	endPage   uint32
	granule   int64
	eos       bool
}

type pageAssembler struct {
	serial      uint32
	nextPage    uint32
	havePage    bool
	packetSeq   uint64
	partial     []byte
	partialPage uint32
	partialOpen bool
	skipFirst   bool
	issues      []Issue
}

func (a *pageAssembler) takeIssues() []Issue {
	issues := a.issues
	a.issues = nil
	return issues
}

func (a *pageAssembler) feed(page OggPage) []oggPacket {
	if a.havePage && page.Sequence != a.nextPage {
		a.issues = append(a.issues, newIssue(
			CodeBadOgg, a.packetSeq, page.Sequence, -1, -1,
			"page sequence jumps from %d to %d", a.nextPage-1, page.Sequence,
		))
	}
	if page.Continued() {
		if !a.partialOpen {
			a.issues = append(a.issues, newIssue(
				CodeTruncatedPacket, a.packetSeq, page.Sequence, -1, -1,
				"continued page has no preceding packet fragment",
			))
			a.skipFirst = true
			a.packetSeq++
		}
	} else if a.partialOpen {
		a.issues = append(a.issues, newIssue(
			CodeTruncatedPacket, a.packetSeq, page.Sequence, -1, -1,
			"packet fragment ends before a page without the continued flag",
		))
		a.partial = nil
		a.partialOpen = false
		a.packetSeq++
	}
	a.havePage = true
	a.nextPage = page.Sequence + 1

	var packets []oggPacket
	offset := 0
	for _, lacing := range page.Segments {
		chunkEnd := offset + int(lacing)
		if chunkEnd > len(page.Data) {
			a.issues = append(a.issues, newIssue(
				CodeBadOgg, a.packetSeq, page.Sequence, -1, -1,
				"lacing table exceeds page data",
			))
			break
		}
		if a.skipFirst {
			offset = chunkEnd
			if lacing < 255 {
				a.skipFirst = false
			}
			continue
		}
		if !a.partialOpen {
			a.partialPage = page.Sequence
			a.partialOpen = true
		}
		a.partial = append(a.partial, page.Data[offset:chunkEnd]...)
		offset = chunkEnd
		if lacing < 255 {
			packets = append(packets, oggPacket{
				data:      append([]byte(nil), a.partial...),
				seq:       a.packetSeq,
				serial:    page.Serial,
				startPage: a.partialPage,
				endPage:   page.Sequence,
				granule:   page.Granule,
				eos:       page.EOS(),
			})
			a.packetSeq++
			a.partial = nil
			a.partialOpen = false
		}
	}
	return packets
}
