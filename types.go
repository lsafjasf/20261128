// Package webvtt parses WebVTT documents and cue payload markup.
//
// The parser keeps both the original cue payload and a structured markup tree.
// This makes it safe to preserve annotations that are not understood while
// still exposing voice and language spans to callers.
package webvtt

import (
	"time"
)

// NodeType identifies a cue payload node.
type NodeType uint8

const (
	NodeText NodeType = iota
	NodeBold
	NodeItalic
	NodeUnderline
	NodeClass
	NodeVoice
	NodeLanguage
	NodeRuby
	NodeRubyText
	NodeTimestamp
	NodeUnknown
)

// String returns the WebVTT tag name for a node type.
func (t NodeType) String() string {
	switch t {
	case NodeText:
		return "text"
	case NodeBold:
		return "b"
	case NodeItalic:
		return "i"
	case NodeUnderline:
		return "u"
	case NodeClass:
		return "c"
	case NodeVoice:
		return "v"
	case NodeLanguage:
		return "lang"
	case NodeRuby:
		return "ruby"
	case NodeRubyText:
		return "rt"
	case NodeTimestamp:
		return "timestamp"
	case NodeUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// Node is one component of a parsed cue payload.
//
// Raw contains the exact source text represented by the node, including a
// container's start tag, children, and end tag. StartTag and EndTag contain the
// exact tag lexemes. Text contains the entity-decoded contents of a text node.
// Annotation is the annotation following the tag name with surrounding
// whitespace trimmed; it is kept for unknown tags as well as voice and language
// tags.
type Node struct {
	Type       NodeType
	Raw        string
	StartTag   string
	EndTag     string
	Text       string
	Annotation string
	Voice      string
	Language   string
	Classes    []string
	Children   []Node
}

// Cue is a WebVTT cue.
//
// Payload is the original cue payload with line endings normalized to "\n".
// Nodes is its parsed representation. Start and End are relative to the media
// timeline.
type Cue struct {
	ID       string
	Start    time.Duration
	End      time.Duration
	Settings string
	Payload  string
	Nodes    []Node
}

// RawText returns the original cue payload.
func (c Cue) RawText() string {
	return c.Payload
}

// PlainText returns all cue text in payload order, with known markup removed.
func (c Cue) PlainText() string {
	c = parsedCue(c)
	return RenderText(c.Nodes, RenderOptions{})
}

// Text returns cue text for language, applying the same fallback rules as
// RenderText.
func (c Cue) Text(language string) string {
	c = parsedCue(c)
	return RenderText(c.Nodes, RenderOptions{Language: language})
}

// Document is a parsed WebVTT file.
type Document struct {
	Header string
	Cues   []Cue
}
