package medialib

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
)

// Kind discriminates the three stable-identity layers.
type Kind string

const (
	KindMaster   Kind = "master"
	KindSubclip  Kind = "subclip"
	KindInstance Kind = "instance"
)

// ID is an OPAQUE stable identifier. Equality of names, paths or source
// ranges never implies equality of IDs. Two subclips cut from the same
// master over the same range are distinct identities and keep distinct IDs.
type ID struct {
	Kind Kind
	Hex  string
}

func (i ID) String() string { return string(i.Kind) + "_" + i.Hex }

func (i ID) Valid() bool {
	return (i.Kind == KindMaster || i.Kind == KindSubclip || i.Kind == KindInstance) && i.Hex != ""
}

var idCounter uint64

func newID(k Kind) ID {
	// 8 random bytes + monotonic suffix: unique even under identical inputs,
	// and stable for the lifetime of the object (never regenerated).
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("medialib: cannot read crypto/rand: " + err.Error())
	}
	seq := atomic.AddUint64(&idCounter, 1)
	return ID{Kind: k, Hex: hex.EncodeToString(b[:]) + "-" + itoa(seq)}
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TimeRange is a half-open source interval [Start, End), in media time units.
// It is a VALUE attribute of a subclip, never part of its identity.
type TimeRange struct {
	Start int64
	End   int64
}

func (r TimeRange) Valid() bool { return r.Start >= 0 && r.End > r.Start }

func (r TimeRange) Contains(other TimeRange) bool {
	return other.Start >= r.Start && other.End <= r.End
}

// MasterMedia is the source asset. Identity = ID; path/name are mutable data.
type MasterMedia struct {
	ID       ID
	Name     string
	FilePath string
	Duration int64
}

// Subclip references a master by ID over a source range. Name, markers and
// range are mutable; the subclip ID never changes and never collides with
// another subclip, even one with the identical master and range.
type Subclip struct {
	ID      ID
	Master  ID
	Name    string
	Range   TimeRange
	Markers map[string]string
}

// Instance is a timeline placement of one subclip. It survives renames and
// retrims of the referenced subclip because the link is by stable ID.
type Instance struct {
	ID       ID
	Subclip  ID
	Timeline string
	At       int64
	Length   int64
}
