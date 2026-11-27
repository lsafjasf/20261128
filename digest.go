package mp4safe

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

// TrackDigest is a hash-of-sample-hashes for one track.
type TrackDigest struct {
	TrackID uint32 `json:"trackId"`
	Handler string `json:"handler"`
	Samples int    `json:"samples"`
	Bytes   uint64 `json:"bytes"`
	SHA256  string `json:"sha256"`
}

// MediaDigest identifies an mdat payload without changing it.
type MediaDigest struct {
	Index  int    `json:"index"`
	Offset uint64 `json:"offset"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// UnknownDigest identifies an unrecognized box's exact bytes.
type UnknownDigest struct {
	Path     string `json:"path"`
	UserType string `json:"userType,omitempty"`
	Offset   uint64 `json:"offset"`
	Bytes    uint64 `json:"bytes"`
	SHA256   string `json:"sha256"`
}

// Digests is the preservation check performed before committing a rewrite.
type Digests struct {
	Tracks        []TrackDigest   `json:"tracks"`
	MediaPayloads []MediaDigest   `json:"mediaPayloads"`
	UnknownBoxes  []UnknownDigest `json:"unknownBoxes"`
}

// ValidateMediaRanges ensures every sample extent lies inside an mdat payload.
func (f *File) ValidateMediaRanges() error {
	payloads := make([]*Box, 0)
	for _, b := range f.Boxes {
		if b.Class == ClassMedia {
			payloads = append(payloads, b)
		}
	}
	for _, t := range f.Tracks {
		for _, s := range t.Samples {
			if s.Size == 0 {
				continue
			}
			end := s.Offset + uint64(s.Size)
			if end < s.Offset {
				return fmt.Errorf("track %d sample %d offset overflow", t.ID, s.Index)
			}
			ok := false
			for _, m := range payloads {
				if s.Offset >= m.PayloadStart() && end <= m.End {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("track %d sample %d [%d,%d) is outside mdat payload",
					t.ID, s.Index, s.Offset, end)
			}
		}
	}
	return nil
}

// ComputeDigests hashes each sample, each mdat payload, and each unknown box.
func (f *File) ComputeDigests() (Digests, error) {
	if err := f.ValidateMediaRanges(); err != nil {
		return Digests{}, err
	}
	var out Digests
	for _, t := range f.Tracks {
		d, err := f.trackDigest(t)
		if err != nil {
			return Digests{}, err
		}
		out.Tracks = append(out.Tracks, d)
	}
	for _, b := range f.Boxes {
		if b.Class != ClassMedia {
			continue
		}
		sum, err := f.rangeSHA256(b.PayloadStart(), b.PayloadSize)
		if err != nil {
			return Digests{}, err
		}
		out.MediaPayloads = append(out.MediaPayloads, MediaDigest{
			Index: len(out.MediaPayloads), Offset: b.PayloadStart(), Bytes: b.PayloadSize, SHA256: sum,
		})
	}
	for _, b := range f.allBoxes {
		if b.Class != ClassUnknown {
			continue
		}
		sum, err := f.rangeSHA256(b.Start, b.Size())
		if err != nil {
			return Digests{}, err
		}
		d := UnknownDigest{
			Path: b.Path(), Offset: b.Start, Bytes: b.Size(), SHA256: sum,
		}
		if b.HasUserType {
			d.UserType = hex.EncodeToString(b.UserType[:])
		}
		out.UnknownBoxes = append(out.UnknownBoxes, d)
	}
	if out.Tracks == nil {
		out.Tracks = []TrackDigest{}
	}
	if out.MediaPayloads == nil {
		out.MediaPayloads = []MediaDigest{}
	}
	if out.UnknownBoxes == nil {
		out.UnknownBoxes = []UnknownDigest{}
	}
	return out, nil
}

func (f *File) trackDigest(t *Track) (TrackDigest, error) {
	agg := sha256.New()
	for _, s := range t.Samples {
		h := sha256.New()
		if _, err := io.Copy(h, io.NewSectionReader(f.Reader, int64(s.Offset), int64(s.Size))); err != nil {
			return TrackDigest{}, fmt.Errorf("hash track %d sample %d: %w", t.ID, s.Index, err)
		}
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(s.Size))
		agg.Write(size[:])
		agg.Write(h.Sum(nil))
	}
	return TrackDigest{
		TrackID: t.ID,
		Handler: t.Handler,
		Samples: len(t.Samples),
		Bytes:   t.TotalBytes,
		SHA256:  hex.EncodeToString(agg.Sum(nil)),
	}, nil
}

func (f *File) rangeSHA256(off, size uint64) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(f.Reader, int64(off), int64(size))); err != nil {
		return "", fmt.Errorf("hash [%d,%d): %w", off, off+size, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompareDigests verifies that every sample, media payload, and unknown box
// retained its exact content.
func CompareDigests(before, after Digests) error {
	if len(before.Tracks) != len(after.Tracks) {
		return fmt.Errorf("track count changed: %d -> %d", len(before.Tracks), len(after.Tracks))
	}
	for i := range before.Tracks {
		a, b := before.Tracks[i], after.Tracks[i]
		if a.TrackID != b.TrackID || a.Handler != b.Handler || a.Samples != b.Samples || a.Bytes != b.Bytes {
			return fmt.Errorf("track %d metadata changed", a.TrackID)
		}
		if a.SHA256 != b.SHA256 {
			return fmt.Errorf("track %d sample hash changed: %s -> %s", a.TrackID, a.SHA256, b.SHA256)
		}
	}
	if len(before.MediaPayloads) != len(after.MediaPayloads) {
		return fmt.Errorf("mdat count changed: %d -> %d", len(before.MediaPayloads), len(after.MediaPayloads))
	}
	for i := range before.MediaPayloads {
		a, b := before.MediaPayloads[i], after.MediaPayloads[i]
		if a.Bytes != b.Bytes || a.SHA256 != b.SHA256 {
			return fmt.Errorf("mdat payload %d changed", i)
		}
	}
	if len(before.UnknownBoxes) != len(after.UnknownBoxes) {
		return fmt.Errorf("unknown box count changed: %d -> %d", len(before.UnknownBoxes), len(after.UnknownBoxes))
	}
	for i := range before.UnknownBoxes {
		a, b := before.UnknownBoxes[i], after.UnknownBoxes[i]
		if a.Path != b.Path || a.UserType != b.UserType || a.Bytes != b.Bytes || a.SHA256 != b.SHA256 {
			return fmt.Errorf("unknown box %s changed", a.Path)
		}
	}
	return nil
}
