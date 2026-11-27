package mp4safe

import (
	"fmt"
	"os"
)

// BoxInfo is a JSON-friendly box boundary record.
type BoxInfo struct {
	Type        string    `json:"type"`
	Class       Class     `json:"class"`
	Start       uint64    `json:"start"`
	End         uint64    `json:"end"`
	HeaderSize  uint64    `json:"headerSize"`
	PayloadSize uint64    `json:"payloadSize"`
	Removable   bool      `json:"removablePadding"`
	UserType    string    `json:"userType,omitempty"`
	Children    []BoxInfo `json:"children,omitempty"`
}

// TrackInfo summarizes a track sample table.
type TrackInfo struct {
	ID         uint32 `json:"id"`
	Handler    string `json:"handler"`
	ChunkTable string `json:"chunkTable,omitempty"`
	Chunks     int    `json:"chunks"`
	Samples    int    `json:"samples"`
	Bytes      uint64 `json:"bytes"`
}

// Inspection is the complete boundary and preservation view of a file.
type Inspection struct {
	File        string      `json:"file"`
	Bytes       uint64      `json:"bytes"`
	Boxes       []BoxInfo   `json:"boxes"`
	Tracks      []TrackInfo `json:"tracks"`
	Unsupported []string    `json:"unsupported,omitempty"`
	Digests     Digests     `json:"digests"`
}

// InspectFile parses path and reports boundaries, tables, and digests.
func InspectFile(path string) (*Inspection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	parsed, err := Parse(f, info.Size())
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return parsed.Inspect(path)
}

// Inspect creates a report for an already parsed file.
func (f *File) Inspect(name string) (*Inspection, error) {
	digests, err := f.ComputeDigests()
	if err != nil {
		return nil, err
	}
	report := &Inspection{
		File: name, Bytes: f.Size, Unsupported: f.Unsupported, Digests: digests,
	}
	for _, b := range f.Boxes {
		report.Boxes = append(report.Boxes, boxInfo(b))
	}
	for _, t := range f.Tracks {
		ti := TrackInfo{
			ID: t.ID, Handler: t.Handler, Chunks: 0, Samples: len(t.Samples), Bytes: t.TotalBytes,
		}
		if t.Chunks != nil {
			ti.ChunkTable = t.Chunks.Box.TypeName()
			ti.Chunks = len(t.Chunks.Entries)
		}
		report.Tracks = append(report.Tracks, ti)
	}
	if report.Boxes == nil {
		report.Boxes = []BoxInfo{}
	}
	if report.Tracks == nil {
		report.Tracks = []TrackInfo{}
	}
	return report, nil
}

func boxInfo(b *Box) BoxInfo {
	info := BoxInfo{
		Type: b.TypeName(), Class: b.Class, Start: b.Start, End: b.End,
		HeaderSize: b.HeaderSize, PayloadSize: b.PayloadSize, Removable: b.Removable,
	}
	if b.HasUserType {
		info.UserType = fmt.Sprintf("%x", b.UserType[:])
	}
	for _, child := range b.Children {
		info.Children = append(info.Children, boxInfo(child))
	}
	return info
}
