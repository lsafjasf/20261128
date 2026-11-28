// Package subs parses subtitle containers into normalized documents.
package subs

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Cue is a single subtitle entry with timestamps on a nanosecond wall clock.
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string // normalized text, lines joined with '\n'
}

// Timebase describes how the source timestamps were represented.
type Timebase struct {
	// Scale is the raw ticks-per-second denominator as written in the file,
	// e.g. "1000" for millisecond SRT/VTT, "100" for centisecond ASS.
	Scale string
	// FrameRate is the effective frame grid inferred from timestamp
	// residues. Zero means no frame grid could be established (wall clock).
	FrameRate float64
	// Offset is the declared timeline offset (e.g. WebVTT X-TIMESTAMP-MAP).
	Offset time.Duration
	// Source records how the timebase was derived.
	Source string
}

// Document is a parsed and normalized subtitle track.
type Document struct {
	Path     string
	Format   string
	Language string
	Timebase Timebase
	Cues     []Cue

	contentHash  string
	timelineHash string
}

// ContentHash hashes the normalized cue texts only, so the same translation
// in different container formats still compares equal.
func (d *Document) ContentHash() string {
	if d.contentHash == "" {
		h := sha256.New()
		for i, cue := range d.Cues {
			if i > 0 {
				h.Write([]byte{0x1e})
			}
			h.Write([]byte(cue.Text))
		}
		d.contentHash = hex.EncodeToString(h.Sum(nil))
	}
	return d.contentHash
}

// TimelineHash hashes the normalized start/end sequence only.
func (d *Document) TimelineHash() string {
	if d.timelineHash == "" {
		var b strings.Builder
		for _, cue := range d.Cues {
			b.WriteString(cue.Start.String())
			b.WriteByte(' ')
			b.WriteString(cue.End.String())
			b.WriteByte('\n')
		}
		h := sha256.Sum256([]byte(b.String()))
		d.timelineHash = hex.EncodeToString(h[:])
	}
	return d.timelineHash
}

// CueCount returns the number of cues.
func (d *Document) CueCount() int { return len(d.Cues) }
