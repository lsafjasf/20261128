package mp4safe

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// RemovedPadding reports one removed padding box.
type RemovedPadding struct {
	Type  string `json:"type"`
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
	Bytes uint64 `json:"bytes"`
}

// RewriteResult describes a verified safe rewrite.
type RewriteResult struct {
	Input          string           `json:"input"`
	Output         string           `json:"output"`
	InputBytes     uint64           `json:"inputBytes"`
	OutputBytes    uint64           `json:"outputBytes"`
	RemovedPadding []RemovedPadding `json:"removedPadding"`
	UpdatedOffsets int              `json:"updatedOffsets"`
	InputSHA256    string           `json:"inputSha256"`
	OutputSHA256   string           `json:"outputSha256"`
	Before         Digests          `json:"before"`
	After          Digests          `json:"after"`
}

type offsetPatch struct {
	offset uint64
	value  []byte
}

// StripPadding removes only top-level free/skip boxes and updates the
// standardized stco/co64 chunk offsets. The operation is verified against
// sample, mdat, and unknown-box digests before the output is committed.
func StripPadding(inPath, outPath string) (*RewriteResult, error) {
	if inPath == "" || outPath == "" {
		return nil, fmt.Errorf("input and output paths are required")
	}
	src, err := os.Open(inPath)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat input: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input is not a regular file")
	}
	beforeFile, err := Parse(src, info.Size())
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if len(beforeFile.Unsupported) > 0 {
		return nil, fmt.Errorf("unsafe container features: %v", beforeFile.Unsupported)
	}
	beforeDigests, err := beforeFile.ComputeDigests()
	if err != nil {
		return nil, fmt.Errorf("digest input: %w", err)
	}
	padding := beforeFile.Padding()
	patches, updated, err := planOffsetPatches(beforeFile, padding)
	if err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(outPath), "."+filepath.Base(outPath)+".mp4safe-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("set output mode: %w", err)
	}
	bw := bufio.NewWriterSize(tmp, 1<<20)
	if err := copyWithoutPadding(bw, src, beforeFile.Size, padding, patches); err != nil {
		return nil, fmt.Errorf("write output: %w", err)
	}
	if err := bw.Flush(); err != nil {
		return nil, fmt.Errorf("flush output: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return nil, fmt.Errorf("sync output: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close output: %w", err)
	}

	out, err := os.Open(tmpName)
	if err != nil {
		return nil, fmt.Errorf("reopen output: %w", err)
	}
	outInfo, err := out.Stat()
	if err != nil {
		out.Close()
		return nil, fmt.Errorf("stat output: %w", err)
	}
	afterFile, err := Parse(out, outInfo.Size())
	if err != nil {
		out.Close()
		return nil, fmt.Errorf("parse output: %w", err)
	}
	afterDigests, err := afterFile.ComputeDigests()
	if err != nil {
		out.Close()
		return nil, fmt.Errorf("digest output: %w", err)
	}
	if err := CompareDigests(beforeDigests, afterDigests); err != nil {
		out.Close()
		return nil, fmt.Errorf("preservation check failed: %w", err)
	}
	if len(afterFile.Padding()) != 0 {
		out.Close()
		return nil, fmt.Errorf("preservation check failed: padding remains")
	}
	outputHash, err := hashReader(out)
	out.Close()
	if err != nil {
		return nil, fmt.Errorf("hash output: %w", err)
	}
	inputHash, err := hashPath(inPath)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return nil, fmt.Errorf("commit output: %w", err)
	}
	committed = true

	result := &RewriteResult{
		Input:          inPath,
		Output:         outPath,
		InputBytes:     beforeFile.Size,
		OutputBytes:    afterFile.Size,
		UpdatedOffsets: updated,
		InputSHA256:    inputHash,
		OutputSHA256:   outputHash,
		Before:         beforeDigests,
		After:          afterDigests,
	}
	for _, b := range padding {
		result.RemovedPadding = append(result.RemovedPadding, RemovedPadding{
			Type: b.TypeName(), Start: b.Start, End: b.End, Bytes: b.Size(),
		})
	}
	if result.RemovedPadding == nil {
		result.RemovedPadding = []RemovedPadding{}
	}
	return result, nil
}

func planOffsetPatches(f *File, padding []*Box) ([]offsetPatch, int, error) {
	var patches []offsetPatch
	updated := 0
	for _, t := range f.Tracks {
		if t.Chunks == nil {
			continue
		}
		width := uint64(4)
		if t.Chunks.Wide {
			width = 8
		}
		for i, old := range t.Chunks.Entries {
			removed := uint64(0)
			for _, p := range padding {
				if old >= p.Start && old < p.End {
					return nil, 0, fmt.Errorf("chunk offset %d points inside %s padding at %d",
						old, p.TypeName(), p.Start)
				}
				if old >= p.End {
					removed += p.Size()
				}
			}
			if removed > old {
				return nil, 0, fmt.Errorf("chunk offset %d underflows removal", old)
			}
			newOffset := old - removed
			if !t.Chunks.Wide && newOffset > math.MaxUint32 {
				return nil, 0, fmt.Errorf("chunk offset %d does not fit stco", newOffset)
			}
			if newOffset == old {
				continue
			}
			var value [8]byte
			if t.Chunks.Wide {
				binary.BigEndian.PutUint64(value[:], newOffset)
				patches = append(patches, offsetPatch{
					offset: t.Chunks.EntryStart + uint64(i)*width,
					value:  append([]byte(nil), value[:]...),
				})
			} else {
				binary.BigEndian.PutUint32(value[:4], uint32(newOffset))
				patches = append(patches, offsetPatch{
					offset: t.Chunks.EntryStart + uint64(i)*width,
					value:  append([]byte(nil), value[:4]...),
				})
			}
			updated++
		}
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].offset < patches[j].offset })
	for i := 1; i < len(patches); i++ {
		if patches[i].offset < patches[i-1].offset+uint64(len(patches[i-1].value)) {
			return nil, 0, fmt.Errorf("overlapping offset patches at %d", patches[i].offset)
		}
	}
	return patches, updated, nil
}

func copyWithoutPadding(dst io.Writer, src io.ReaderAt, size uint64, padding []*Box, patches []offsetPatch) error {
	cursor := uint64(0)
	for _, p := range padding {
		if p.Start < cursor || p.End > size {
			return fmt.Errorf("invalid padding range [%d,%d)", p.Start, p.End)
		}
		if err := copyPatchedRange(dst, src, cursor, p.Start, patches); err != nil {
			return err
		}
		cursor = p.End
	}
	return copyPatchedRange(dst, src, cursor, size, patches)
}

func copyPatchedRange(dst io.Writer, src io.ReaderAt, start, end uint64, patches []offsetPatch) error {
	const blockSize = 1 << 20
	buf := make([]byte, blockSize+16)
	first := sort.Search(len(patches), func(i int) bool {
		return patches[i].offset+uint64(len(patches[i].value)) > start
	})
	for off := start; off < end; {
		chunkEnd := off + blockSize
		if chunkEnd > end {
			chunkEnd = end
		}
		for i := first; i < len(patches) && patches[i].offset < chunkEnd; i++ {
			patchEnd := patches[i].offset + uint64(len(patches[i].value))
			if patchEnd > chunkEnd {
				chunkEnd = patchEnd
			}
		}
		if chunkEnd > end {
			chunkEnd = end
		}
		n := chunkEnd - off
		if n > uint64(len(buf)) {
			return fmt.Errorf("internal copy block too large: %d", n)
		}
		if err := readFullAt(src, buf[:n], off); err != nil {
			return err
		}
		for i := first; i < len(patches) && patches[i].offset < chunkEnd; i++ {
			p := patches[i]
			patchEnd := p.offset + uint64(len(p.value))
			if p.offset < off || patchEnd > chunkEnd {
				continue
			}
			copy(buf[p.offset-off:], p.value)
		}
		if _, err := dst.Write(buf[:n]); err != nil {
			return err
		}
		off = chunkEnd
		for first < len(patches) && patches[first].offset+uint64(len(patches[first].value)) <= off {
			first++
		}
	}
	return nil
}

func readFullAt(r io.ReaderAt, p []byte, off uint64) error {
	n, err := r.ReadAt(p, int64(off))
	if err != nil && err != io.EOF {
		return err
	}
	if n != len(p) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sum, err := hashReader(f)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return sum, nil
}
