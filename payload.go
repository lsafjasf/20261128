package webvtt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type tokenKind uint8

const (
	tokenText tokenKind = iota
	tokenStart
	tokenEnd
	tokenTimestamp
	tokenUnknown
)

type payloadToken struct {
	kind       tokenKind
	raw        string
	start      int
	end        int
	nodeType   NodeType
	name       string
	annotation string
	classes    []string
}

// ParseCuePayload parses WebVTT cue payload markup into a node tree.
//
// Malformed and unknown tags are retained as NodeUnknown nodes instead of
// being discarded. A payload with no recognizable markup is valid, so this
// function currently only returns an error for an internal parsing failure.
func ParseCuePayload(payload string) ([]Node, error) {
	return ParseCueMarkup(payload), nil
}

// ParseCueMarkup is the non-error form of ParseCuePayload.
func ParseCueMarkup(payload string) []Node {
	tokens := lexPayload(payload)
	if len(tokens) == 0 {
		return nil
	}
	root := &payloadBuilder{end: len(payload)}
	stack := []*payloadBuilder{root}

	for _, tok := range tokens {
		parent := stack[len(stack)-1]
		switch tok.kind {
		case tokenText, tokenTimestamp:
			parent.children = append(parent.children, &payloadBuilder{
				node:  tokenNode(tok),
				start: tok.start,
				end:   tok.end,
			})
		case tokenUnknown:
			parent.children = append(parent.children, &payloadBuilder{
				node:  tokenNode(tok),
				start: tok.start,
				end:   tok.end,
			})
		case tokenStart:
			child := &payloadBuilder{
				node:  tokenNode(tok),
				start: tok.start,
				end:   tok.end,
			}
			parent.children = append(parent.children, child)
			if isContainerType(tok.nodeType) {
				stack = append(stack, child)
			}
		case tokenEnd:
			match := findOpenTag(stack, tok.nodeType)
			if match < 0 {
				parent.children = append(parent.children, &payloadBuilder{
					node:  tokenNode(tok),
					start: tok.start,
					end:   tok.end,
				})
				continue
			}
			for i := len(stack) - 1; i >= match; i-- {
				open := stack[i]
				if i == match {
					open.node.EndTag = tok.raw
					open.end = tok.end
				} else if open.end < tok.start {
					open.end = tok.start
				}
			}
			stack = stack[:match]
		}
	}

	for i := len(stack) - 1; i >= 1; i-- {
		stack[i].end = len(payload)
	}
	return buildNodes(root, payload)
}

type payloadBuilder struct {
	node     Node
	start    int
	end      int
	children []*payloadBuilder
}

func buildNodes(builder *payloadBuilder, source string) []Node {
	nodes := make([]Node, 0, len(builder.children))
	for _, child := range builder.children {
		node := child.node
		node.Children = buildNodes(child, source)
		if child.start >= 0 && child.end >= child.start && child.end <= len(source) {
			node.Raw = source[child.start:child.end]
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func tokenNode(tok payloadToken) Node {
	node := Node{
		Type:       tok.nodeType,
		StartTag:   tok.raw,
		Annotation: tok.annotation,
		Classes:    append([]string(nil), tok.classes...),
	}
	switch tok.kind {
	case tokenText:
		node.Text = decodeEntities(tok.raw)
	case tokenEnd:
		node.Type = NodeUnknown
		node.StartTag = ""
		node.EndTag = tok.raw
	case tokenTimestamp:
		node.Text = tok.annotation
	case tokenUnknown:
		if strings.HasPrefix(tok.raw, "</") {
			node.StartTag = ""
			node.EndTag = tok.raw
		}
	}
	if tok.nodeType == NodeVoice {
		node.Voice = tok.annotation
	}
	if tok.nodeType == NodeLanguage {
		node.Language = tok.annotation
	}
	return node
}

func isContainerType(t NodeType) bool {
	switch t {
	case NodeBold, NodeItalic, NodeUnderline, NodeClass,
		NodeVoice, NodeLanguage, NodeRuby, NodeRubyText:
		return true
	default:
		return false
	}
}

func findOpenTag(stack []*payloadBuilder, want NodeType) int {
	for i := len(stack) - 1; i >= 1; i-- {
		if stack[i].node.Type == want {
			return i
		}
	}
	return -1
}

func lexPayload(payload string) []payloadToken {
	var tokens []payloadToken
	for pos := 0; pos < len(payload); {
		if payload[pos] != '<' {
			next := strings.IndexByte(payload[pos:], '<')
			if next < 0 {
				next = len(payload) - pos
			}
			raw := payload[pos : pos+next]
			tokens = append(tokens, payloadToken{
				kind:     tokenText,
				raw:      raw,
				start:    pos,
				end:      pos + next,
				nodeType: NodeText,
			})
			pos += next
			continue
		}

		closeAt := strings.IndexByte(payload[pos:], '>')
		if closeAt < 0 {
			tokens = append(tokens, payloadToken{
				kind:     tokenText,
				raw:      payload[pos:],
				start:    pos,
				end:      len(payload),
				nodeType: NodeText,
			})
			break
		}
		end := pos + closeAt + 1
		token := classifyTag(payload[pos:end], pos, end)
		tokens = append(tokens, token)
		pos = end
	}
	return tokens
}

func classifyTag(raw string, start, end int) payloadToken {
	token := payloadToken{
		kind:     tokenUnknown,
		raw:      raw,
		start:    start,
		end:      end,
		nodeType: NodeUnknown,
	}
	if len(raw) < 3 || raw[0] != '<' || raw[len(raw)-1] != '>' {
		return token
	}
	inner := raw[1 : len(raw)-1]
	if strings.HasPrefix(inner, "/") {
		name := strings.ToLower(inner[1:])
		if isKnownTagName(name) {
			token.kind = tokenEnd
			token.name = name
			token.nodeType = tagNodeType(name)
		}
		return token
	}
	if _, err := parseTimestamp(inner); err == nil {
		token.kind = tokenTimestamp
		token.nodeType = NodeTimestamp
		token.annotation = inner
		return token
	}
	name, nodeType, ok := parseTagName(inner)
	if !ok {
		if name != "" {
			token.name = name
			if classes, annotation, suffixOK := parseTagSuffix(inner[len(name):]); suffixOK {
				token.classes = classes
				token.annotation = annotation
			}
		}
		return token
	}
	classes, annotation, ok := parseTagSuffix(inner[len(name):])
	if !ok {
		return token
	}
	switch nodeType {
	case NodeVoice, NodeLanguage:
		if annotation == "" {
			return token
		}
	case NodeBold, NodeItalic, NodeUnderline, NodeRuby, NodeRubyText:
		if annotation != "" || len(classes) != 0 {
			return token
		}
	}
	token.kind = tokenStart
	token.name = name
	token.nodeType = nodeType
	token.classes = classes
	token.annotation = annotation
	return token
}

func parseTagSuffix(suffix string) ([]string, string, bool) {
	var classes []string
	pos := 0
	for pos < len(suffix) && suffix[pos] == '.' {
		pos++
		begin := pos
		for pos < len(suffix) && suffix[pos] != '.' && !isVTTWhitespace(suffix[pos]) {
			pos++
		}
		if begin == pos {
			return nil, "", false
		}
		classes = append(classes, suffix[begin:pos])
	}
	if pos < len(suffix) && !isVTTWhitespace(suffix[pos]) {
		return nil, "", false
	}
	return classes, strings.TrimSpace(suffix[pos:]), true
}

func isVTTWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func isKnownTagName(name string) bool {
	switch name {
	case "b", "i", "u", "c", "v", "lang", "ruby", "rt":
		return true
	default:
		return false
	}
}

func tagNodeType(name string) NodeType {
	switch name {
	case "b":
		return NodeBold
	case "i":
		return NodeItalic
	case "u":
		return NodeUnderline
	case "c":
		return NodeClass
	case "v":
		return NodeVoice
	case "lang":
		return NodeLanguage
	case "ruby":
		return NodeRuby
	case "rt":
		return NodeRubyText
	default:
		return NodeUnknown
	}
}

func parseTagName(inner string) (string, NodeType, bool) {
	nameEnd := 0
	for nameEnd < len(inner) && isASCIIAlpha(inner[nameEnd]) {
		nameEnd++
	}
	if nameEnd == 0 {
		return "", NodeUnknown, false
	}
	name := strings.ToLower(inner[:nameEnd])
	nodeType := tagNodeType(name)
	if nodeType == NodeUnknown {
		return name, NodeUnknown, false
	}
	return name, nodeType, true
}

func isASCIIAlpha(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func decodeEntities(text string) string {
	if !strings.Contains(text, "&") {
		return text
	}
	var out strings.Builder
	for {
		amp := strings.IndexByte(text, '&')
		if amp < 0 {
			out.WriteString(text)
			break
		}
		out.WriteString(text[:amp])
		text = text[amp:]
		semi := strings.IndexByte(text, ';')
		if semi < 0 {
			out.WriteString(text)
			break
		}
		entity := text[:semi+1]
		if value, ok := webVTTEntity(entity); ok {
			out.WriteString(value)
			text = text[semi+1:]
			continue
		}
		out.WriteString(text[:1])
		text = text[1:]
	}
	return out.String()
}

func webVTTEntity(entity string) (string, bool) {
	switch entity {
	case "&amp;":
		return "&", true
	case "&lt;":
		return "<", true
	case "&gt;":
		return ">", true
	case "&lrm;":
		return "\u200e", true
	case "&rlm;":
		return "\u200f", true
	case "&nbsp;":
		return "\u00a0", true
	default:
		return "", false
	}
}

// ParseTimestamp parses a WebVTT timestamp such as 00:01.250 or
// 01:02:03.004 into a duration.
func ParseTimestamp(value string) (time.Duration, error) {
	return parseTimestamp(value)
}

func parseTimestamp(value string) (time.Duration, error) {
	if strings.Count(value, ":") == 1 {
		return parseClockTimestamp(value, false)
	}
	if strings.Count(value, ":") == 2 {
		return parseClockTimestamp(value, true)
	}
	return 0, fmt.Errorf("webvtt: invalid timestamp %q", value)
}

func parseClockTimestamp(value string, hasHours bool) (time.Duration, error) {
	parts := strings.Split(value, ":")
	clock := parts[len(parts)-1]
	dot := strings.IndexByte(clock, '.')
	if dot < 0 || len(clock)-dot-1 != 3 {
		return 0, fmt.Errorf("webvtt: invalid timestamp %q", value)
	}
	seconds, err := parseFixedDigits(clock[:dot], 2)
	if err != nil || seconds > 59 {
		return 0, fmt.Errorf("webvtt: invalid timestamp %q", value)
	}
	milliseconds, err := parseFixedDigits(clock[dot+1:], 3)
	if err != nil {
		return 0, fmt.Errorf("webvtt: invalid timestamp %q", value)
	}
	minutesDigits := parts[len(parts)-2]
	minutes, err := parseFixedDigits(minutesDigits, 2)
	if err != nil || minutes > 59 {
		return 0, fmt.Errorf("webvtt: invalid timestamp %q", value)
	}
	var hours int64
	if hasHours {
		hours, err = parseVariableDigits(parts[0])
		if err != nil {
			return 0, fmt.Errorf("webvtt: invalid timestamp %q", value)
		}
	}
	total := (hours*60+int64(minutes))*60 + int64(seconds)
	return time.Duration(total)*time.Second + time.Duration(milliseconds)*time.Millisecond, nil
}

func parseFixedDigits(value string, width int) (int, error) {
	if len(value) != width {
		return 0, fmt.Errorf("expected %d digits", width)
	}
	return parseNumber(value)
}

func parseVariableDigits(value string) (int64, error) {
	if len(value) < 2 {
		return 0, fmt.Errorf("expected at least two digits")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func parseNumber(value string) (int, error) {
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return 0, fmt.Errorf("not a number")
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return n, nil
}
