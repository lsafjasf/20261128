package wavinfo

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// Preference selects which source wins when a project field and the file name
// suggest different values for the same INFO id.
type Preference int

const (
	// PreferEmbedded keeps the value already stored inside the WAV file.
	PreferEmbedded Preference = iota
	// PreferProject always takes the project-supplied value.
	PreferProject
	// PreferFileName always takes the value parsed from the file name.
	PreferFileName
)

// File is a WAV (RIFF) document together with its parsed INFO list. All chunks
// other than the LIST INFO chunk are carried through WriteTo byte-for-byte.
type File struct {
	riff *RIFFFile
	info *InfoList

	// infoIndex is the position of the LIST INFO chunk inside riff.chunks;
	// -1 means the file currently has no INFO list.
	infoIndex int
}

// Parse decodes a WAV stream. Files without a LIST INFO chunk are accepted and
// start with an empty InfoList.
func Parse(r io.Reader) (*File, error) {
	riff, err := ParseRIFF(r)
	if err != nil {
		return nil, err
	}
	if riff.FormType != "WAVE" {
		return nil, fmt.Errorf("wavinfo: not a WAVE file (RIFF form type %q)", riff.FormType)
	}

	file := &File{riff: riff, infoIndex: -1}
	for i, chunk := range riff.chunks {
		if chunk.ID != "LIST" || len(chunk.Data) < 4 || string(chunk.Data[0:4]) != "INFO" {
			continue
		}
		list, perr := parseInfoList(chunk.Data)
		if perr != nil {
			return nil, perr
		}
		file.info = list
		file.infoIndex = i
		break
	}
	if file.info == nil {
		file.info = &InfoList{}
	}
	return file, nil
}

// Info returns the file's INFO list (never nil); edits made on it are written
// back by WriteTo.
func (f *File) Info() *InfoList {
	return f.info
}

// ChunkIDs returns the ordered list of top-level chunk ids, useful for tests
// and inspection. A freshly added LIST INFO chunk appears on the next write.
func (f *File) ChunkIDs() []string {
	ids := make([]string, len(f.riff.chunks))
	for i, c := range f.riff.chunks {
		ids[i] = c.ID
	}
	return ids
}

// WriteTo serializes the WAV document. Audio data and every other non-INFO
// chunk are emitted from their original on-disk bytes; only the LIST INFO
// chunk is rebuilt, and the outer RIFF size is recomputed accordingly.
func (f *File) WriteTo(w io.Writer) (int64, error) {
	encoded := f.info.encode()
	if f.infoIndex >= 0 {
		chunk := &f.riff.chunks[f.infoIndex]
		chunk.Data = encoded
		chunk.MarkDirty()
	} else if len(f.info.entries) > 0 {
		f.insertInfoChunk(encoded)
	}
	return f.riff.WriteTo(w)
}

// insertInfoChunk places a new LIST INFO chunk before the first data chunk, or
// at the end of the file when no data chunk exists (a common placement for
// metadata-only or non-audio RIFF documents).
func (f *File) insertInfoChunk(data []byte) {
	chunk := Chunk{ID: "LIST", Data: data}
	insertAt := len(f.riff.chunks)
	for i, c := range f.riff.chunks {
		if c.ID == "data" {
			insertAt = i
			break
		}
	}
	f.riff.chunks = append(f.riff.chunks, Chunk{})
	copy(f.riff.chunks[insertAt+1:], f.riff.chunks[insertAt:])
	f.riff.chunks[insertAt] = chunk
	f.infoIndex = insertAt
}

// ApplyResult reports how a batch of project field values was applied.
type ApplyResult struct {
	// Applied lists INFO ids that received a value.
	Applied []string
	// Skipped lists project keys that could not be resolved to an INFO id.
	Skipped []string
	// Conflicted lists ids where embedded and project values disagreed; the
	// winner depends on the chosen Preference.
	Conflicted []Conflict
}

// Conflict describes a disagreement between the embedded tag and an external
// value for one field.
type Conflict struct {
	ID       string
	Embedded string
	External string
	Winner   string // "embedded" or "external"
}

// ApplyProjectFields merges project-side metadata into the INFO list. Keys may
// be INFO ids in any case or names from ResolveField. When a key also appears
// in fileNameValues, the preference argument decides which value wins; an
// embedded tag still takes precedence under PreferEmbedded.
func (f *File) ApplyProjectFields(fields map[string]string, fileNameValues map[string]string, pref Preference) (*ApplyResult, error) {
	res := &ApplyResult{}

	// Resolve every project key first; multiple project keys can map onto the
	// same INFO id, and file-name keys are already INFO ids.
	projectByID := make(map[string]string, len(fields))
	for k := range fields {
		id, ok := ResolveField(k)
		if !ok {
			res.Skipped = append(res.Skipped, k)
			continue
		}
		projectByID[id] = fields[k]
	}

	// Process the union of resolved ids in deterministic order.
	idSet := make(map[string]bool, len(projectByID)+len(fileNameValues))
	for id := range projectByID {
		idSet[id] = true
	}
	for id := range fileNameValues {
		idSet[id] = true
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		projectValue := projectByID[id]
		fileValue := fileNameValues[id]
		embedded, hasEmbedded := f.info.Get(id)

		chosen, external := chooseValue(embedded, hasEmbedded, projectValue, fileValue, pref)
		if hasEmbedded && external != "" && embedded != external {
			winner := "embedded"
			if chosen == external {
				winner = "external"
			}
			res.Conflicted = append(res.Conflicted, Conflict{
				ID:       id,
				Embedded: embedded,
				External: external,
				Winner:   winner,
			})
		}
		if chosen == "" {
			continue
		}
		if err := f.info.Set(id, chosen); err != nil {
			return nil, err
		}
		res.Applied = append(res.Applied, id)
	}
	return res, nil
}

func chooseValue(embedded string, hasEmbedded bool, project, file string, pref Preference) (chosen, external string) {
	switch pref {
	case PreferProject:
		if project != "" {
			return project, firstNonEmpty(file, embedded)
		}
	case PreferFileName:
		if file != "" {
			return file, firstNonEmpty(project, embedded)
		}
	default: // PreferEmbedded
		if hasEmbedded {
			return embedded, firstNonEmpty(project, file)
		}
	}
	// Either the preferred source supplied nothing, or no embedded value
	// exists. Fall back through project, then file name, then embedded.
	if project != "" {
		return project, file
	}
	if file != "" {
		return file, project
	}
	if hasEmbedded {
		return embedded, file
	}
	return file, file
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ParseFileName extracts candidate metadata from a WAV file name. The base
// name is split on " - " (space-dash-space): the first segment becomes the
// INAM title and, when a second segment exists, it becomes the IART artist.
// Keys in the returned map are resolved INFO ids.
func ParseFileName(name string) map[string]string {
	base := path.Base(name)
	base = strings.TrimSuffix(base, path.Ext(base))
	out := map[string]string{}
	parts := strings.SplitN(base, " - ", 2)
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i == 0 {
			out["INAM"] = part
		} else {
			out["IART"] = part
		}
	}
	return out
}

// Bytes is a convenience wrapper that returns the full serialized document.
func (f *File) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
