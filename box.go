package mp4safe

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Class describes how a box participates in a safe container rewrite.
type Class string

const (
	ClassStructure Class = "structure"
	ClassMedia     Class = "media"
	ClassIndex     Class = "index"
	ClassPadding   Class = "padding"
	ClassUnknown   Class = "unknown"
)

// Box is an ISO base media file format box with file offsets in [Start, End).
type Box struct {
	Type        [4]byte
	UserType    [16]byte
	HasUserType bool
	Start       uint64
	End         uint64
	HeaderSize  uint64
	PayloadSize uint64
	Class       Class
	Removable   bool
	Children    []*Box
	Parent      *Box
	Depth       int
}

func (b *Box) TypeName() string {
	return TypeName(b.Type)
}

func (b *Box) Path() string {
	var parts []string
	for cur := b; cur != nil; cur = cur.Parent {
		parts = append(parts, cur.TypeName())
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

func (b *Box) Size() uint64 {
	return b.End - b.Start
}

func (b *Box) PayloadStart() uint64 {
	return b.Start + b.HeaderSize
}

// TypeName returns the four-character code when printable, otherwise hex.
func TypeName(t [4]byte) string {
	for _, c := range t {
		if c < 0x20 || c > 0x7e {
			return "0x" + hex.EncodeToString(t[:])
		}
	}
	return string(t[:])
}

func (b *Box) String() string {
	return fmt.Sprintf("%s@%d+%d %s", b.TypeName(), b.Start, b.Size(), b.Class)
}

var knownBoxTypes = map[[4]byte]bool{
	{'f', 't', 'y', 'p'}: true, {'s', 't', 'y', 'p'}: true,
	{'m', 'o', 'o', 'v'}: true, {'m', 'v', 'h', 'd'}: true,
	{'t', 'r', 'a', 'k'}: true, {'t', 'k', 'h', 'd'}: true,
	{'m', 'd', 'i', 'a'}: true, {'m', 'd', 'h', 'd'}: true,
	{'h', 'd', 'l', 'r'}: true, {'m', 'i', 'n', 'f'}: true,
	{'v', 'm', 'h', 'd'}: true, {'s', 'm', 'h', 'd'}: true,
	{'h', 'm', 'h', 'd'}: true, {'n', 'm', 'h', 'd'}: true,
	{'d', 'i', 'n', 'f'}: true, {'d', 'r', 'e', 'f'}: true,
	{'u', 'r', 'l', ' '}: true, {'u', 'r', 'n', ' '}: true,
	{'s', 't', 'b', 'l'}: true, {'s', 't', 's', 'd'}: true,
	{'s', 't', 't', 's'}: true, {'c', 't', 't', 's'}: true,
	{'s', 't', 's', 'c'}: true, {'s', 't', 's', 'z'}: true,
	{'s', 't', 'z', '2'}: true, {'s', 't', 'c', 'o'}: true,
	{'c', 'o', '6', '4'}: true, {'s', 't', 's', 's'}: true,
	{'s', 't', 's', 'h'}: true, {'p', 'a', 'd', 'b'}: true,
	{'s', 't', 'd', 'p'}: true, {'s', 'd', 't', 'p'}: true,
	{'s', 'b', 'g', 'p'}: true, {'s', 'g', 'p', 'd'}: true,
	{'s', 'u', 'b', 's'}: true, {'s', 'a', 'i', 'z'}: true,
	{'s', 'a', 'i', 'o'}: true, {'m', 'v', 'e', 'x'}: true,
	{'m', 'e', 'h', 'd'}: true, {'t', 'r', 'e', 'x'}: true,
	{'t', 'r', 'e', 'p'}: true, {'m', 'o', 'o', 'f'}: true,
	{'m', 'f', 'h', 'd'}: true, {'t', 'r', 'a', 'f'}: true,
	{'t', 'f', 'h', 'd'}: true, {'t', 'f', 'd', 't'}: true,
	{'t', 'r', 'u', 'n'}: true, {'m', 'f', 'r', 'a'}: true,
	{'t', 'f', 'r', 'a'}: true, {'m', 'f', 'r', 'o'}: true,
	{'e', 'd', 't', 's'}: true, {'e', 'l', 's', 't'}: true,
	{'u', 'd', 't', 'a'}: true, {'m', 'e', 't', 'a'}: true,
	{'i', 'l', 's', 't'}: true, {'i', 'o', 'd', 's'}: true,
	{'g', 'm', 'h', 'd'}: true, {'g', 'm', 'i', 'n'}: true,
	{'t', 'e', 'x', 't'}: true, {'s', 'i', 'd', 'x'}: true,
	{'s', 's', 'i', 'x'}: true, {'m', 'd', 'a', 't'}: true,
	{'f', 'r', 'e', 'e'}: true, {'s', 'k', 'i', 'p'}: true,
	{'w', 'i', 'd', 'e'}: true, {'u', 'u', 'i', 'd'}: true,
	{'i', 'l', 'o', 'c'}: true, {'p', 'i', 't', 'm'}: true,
	{'i', 'i', 'n', 'f'}: true, {'i', 'r', 'e', 'f'}: true,
	{'i', 'p', 'r', 'p'}: true, {'i', 'p', 'c', 'o'}: true,
	{'s', 'i', 'n', 'f'}: true, {'f', 'r', 'm', 'a'}: true,
	{'s', 'c', 'h', 'm'}: true, {'s', 'c', 'h', 'i'}: true,
	{'t', 'e', 'n', 'c'}: true, {'p', 's', 's', 'h'}: true,
	{'s', 'e', 'n', 'c'}: true, {'c', 'o', 'l', 'r'}: true,
	{'p', 'a', 's', 'p'}: true, {'c', 'l', 'a', 'p'}: true,
	{'b', 't', 'r', 't'}: true, {'a', 'v', 'c', 'C'}: true,
	{'h', 'v', 'c', 'C'}: true, {'e', 's', 'd', 's'}: true,
	{'m', 'p', '4', 'a'}: true, {'a', 'v', 'c', '1'}: true,
	{'h', 'v', 'c', '1'}: true, {'h', 'e', 'v', '1'}: true,
	{'e', 'n', 'c', 'v'}: true, {'e', 'n', 'c', 'a'}: true,
}

func isKnownType(t [4]byte) bool {
	return knownBoxTypes[t]
}

var containerTypes = map[[4]byte]bool{
	{'m', 'o', 'o', 'v'}: true, {'t', 'r', 'a', 'k'}: true,
	{'m', 'd', 'i', 'a'}: true, {'m', 'i', 'n', 'f'}: true,
	{'s', 't', 'b', 'l'}: true, {'e', 'd', 't', 's'}: true,
	{'d', 'i', 'n', 'f'}: true, {'u', 'd', 't', 'a'}: true,
	{'m', 'v', 'e', 'x'}: true, {'m', 'o', 'o', 'f'}: true,
	{'t', 'r', 'a', 'f'}: true, {'m', 'f', 'r', 'a'}: true,
}

func isFreeOrSkip(t [4]byte) bool {
	return t == [4]byte{'f', 'r', 'e', 'e'} || t == [4]byte{'s', 'k', 'i', 'p'}
}
