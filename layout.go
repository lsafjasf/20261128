package subtitlelayout

import (
	"fmt"
	"math"
)

// WidthTable maps each glyph's base rune to its advance width. Widths use
// whatever unit the caller uses for MaxWidth and margins.
type WidthTable map[rune]float64

// Options configures the horizontal layout area. Margins are inside MaxWidth,
// so the usable content width is MaxWidth - MarginLeft - MarginRight.
type Options struct {
	MaxWidth    float64
	MarginLeft  float64
	MarginRight float64
}

// Line is one laid-out line. Width excludes the configured margins.
type Line struct {
	Text  string
	Width float64
}

// Layouter wraps text using only glyph advance widths.
type Layouter struct {
	widths       WidthTable
	contentWidth float64
}

// NewLayouter validates the width table and horizontal layout options. The
// width table is copied, so callers may reuse or mutate theirs afterwards.
func NewLayouter(widths WidthTable, opts Options) (*Layouter, error) {
	if math.IsNaN(opts.MaxWidth) || math.IsInf(opts.MaxWidth, 0) || opts.MaxWidth < 0 {
		return nil, fmt.Errorf("%w: max width must be finite and non-negative, got %v", ErrInvalidWidth, opts.MaxWidth)
	}
	if math.IsNaN(opts.MarginLeft) || math.IsInf(opts.MarginLeft, 0) || opts.MarginLeft < 0 {
		return nil, fmt.Errorf("%w: left margin must be finite and non-negative, got %v", ErrNegativeMargin, opts.MarginLeft)
	}
	if math.IsNaN(opts.MarginRight) || math.IsInf(opts.MarginRight, 0) || opts.MarginRight < 0 {
		return nil, fmt.Errorf("%w: right margin must be finite and non-negative, got %v", ErrNegativeMargin, opts.MarginRight)
	}

	table := make(WidthTable, len(widths))
	for r, width := range widths {
		if math.IsNaN(width) || math.IsInf(width, 0) || width < 0 {
			return nil, fmt.Errorf("%w: glyph U+%04X has width %v", ErrInvalidWidth, r, width)
		}
		table[r] = width
	}

	return &Layouter{
		widths:       table,
		contentWidth: opts.MaxWidth - opts.MarginLeft - opts.MarginRight,
	}, nil
}

// ContentWidth returns the width available to line content after margins.
func (l *Layouter) ContentWidth() float64 {
	if l == nil {
		return 0
	}
	return l.contentWidth
}

// Layout wraps text into lines. Explicit newlines are preserved, including
// blank lines and a final empty line after a trailing newline. Combining marks
// are kept attached to their preceding base rune; nonspacing and enclosing
// marks occupy no width, while spacing combining marks use their table width
// when present.
func (l *Layouter) Layout(text string) ([]Line, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: nil layouter", ErrTextDoesNotFit)
	}

	segments, err := splitSegments(text)
	if err != nil {
		return nil, err
	}

	lines := make([]Line, 0, len(segments))
	for i, segment := range segments {
		clusters, err := l.clusters(segment)
		if err != nil {
			return nil, err
		}
		wrapped, err := l.wrap(clusters, i+1)
		if err != nil {
			return nil, err
		}
		lines = append(lines, wrapped...)
	}
	return lines, nil
}

// Layout is a convenience wrapper around NewLayouter and Layouter.Layout.
func Layout(text string, widths WidthTable, opts Options) ([]Line, error) {
	l, err := NewLayouter(widths, opts)
	if err != nil {
		return nil, err
	}
	return l.Layout(text)
}
