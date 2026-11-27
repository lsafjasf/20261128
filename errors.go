package subtitlelayout

import "errors"

var (
	// ErrUnknownGlyph reports a rune that has no entry in the width table.
	ErrUnknownGlyph = errors.New("subtitlelayout: unknown glyph")
	// ErrNegativeMargin reports a negative or non-finite horizontal margin.
	ErrNegativeMargin = errors.New("subtitlelayout: invalid margin")
	// ErrTextDoesNotFit reports text that cannot fit without violating a line
	// break rule or an indivisible cluster wider than the available width.
	ErrTextDoesNotFit = errors.New("subtitlelayout: text does not fit")
	// ErrInvalidWidth reports an invalid max width or glyph width.
	ErrInvalidWidth = errors.New("subtitlelayout: invalid width")
	// ErrInvalidUTF8 reports malformed UTF-8 input.
	ErrInvalidUTF8 = errors.New("subtitlelayout: invalid UTF-8")
)
