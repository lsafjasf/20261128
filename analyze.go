package ghostdiag

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
)

type analyzer struct {
	opts        Options
	fingerprint hash.Hash
	width       int
	height      int
	history     []Plane
	prev        *Plane
	frames      []FrameResult
}

// Analyze streams two frame sequences in lockstep. The returned input SHA-256
// covers every original and denoised luma sample, so parameter sweeps can prove
// they used the same pictures.
func Analyze(original, denoised Source, trajectory Trajectory, options Options) (*Report, error) {
	if original == nil || denoised == nil {
		return nil, errors.New("original and denoised sources are required")
	}
	a := &analyzer{opts: options.normalized(), fingerprint: sha256.New()}
	a.fingerprint.Write([]byte("ghostdiag/input/v1\x00"))
	var originalFormat, denoisedFormat Format
	for {
		orig, origErr := original.Next()
		den, denErr := denoised.Next()
		if errors.Is(origErr, ErrEndOfStream) || errors.Is(denErr, ErrEndOfStream) {
			if !errors.Is(origErr, ErrEndOfStream) || !errors.Is(denErr, ErrEndOfStream) {
				return nil, fmt.Errorf("original and denoised streams have different frame counts")
			}
			break
		}
		if origErr != nil {
			return nil, fmt.Errorf("read original frame: %w", origErr)
		}
		if denErr != nil {
			return nil, fmt.Errorf("read denoised frame: %w", denErr)
		}
		if a.width == 0 {
			a.width, a.height = orig.Plane.Width, orig.Plane.Height
			originalFormat = original.Format()
			denoisedFormat = denoised.Format()
		}
		box, explicit := trajectory.Box(orig.Index)
		result, err := a.push(orig, den, box, explicit)
		if err != nil {
			return nil, err
		}
		a.frames = append(a.frames, result)
	}
	if len(a.frames) == 0 {
		return nil, errors.New("no frames were read")
	}
	rate := originalFormat.FrameRate
	if rate <= 0 {
		rate = denoisedFormat.FrameRate
	}
	if rate <= 0 {
		rate = 25
	}
	report := &Report{
		Schema:  reportSchema,
		Input:   InputInfo{SHA256: hex.EncodeToString(a.fingerprint.Sum(nil)), Frames: len(a.frames), Width: a.width, Height: a.height, FrameRate: rate},
		Options: a.opts,
		Frames:  a.frames,
		Summary: Summary{Frames: len(a.frames), Counts: make(map[string]int)},
	}
	for i := range report.Frames {
		for _, finding := range report.Frames[i].Findings {
			report.Summary.Counts[finding.Classification]++
			if report.FirstGhost == nil && findingHasGhost(finding) {
				copy := finding
				report.FirstGhost = &copy
				frame := finding.Frame
				report.Summary.FirstGhostFrame = &frame
			}
		}
	}
	return report, nil
}

func findingHasGhost(finding Finding) bool {
	return finding.Classification == "temporal_ghost" ||
		finding.Classification == "mixed" && containsString(finding.Secondary, "temporal_ghost")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a *analyzer) writeFingerprint(orig, den Frame) {
	var buf [8]byte
	writeInt := func(v int64) {
		binary.LittleEndian.PutUint64(buf[:], uint64(v))
		a.fingerprint.Write(buf[:])
	}
	writeInt(int64(orig.Index))
	writeInt(int64(orig.Plane.Width))
	writeInt(int64(orig.Plane.Height))
	a.fingerprint.Write(orig.Plane.Pix)
	a.fingerprint.Write(den.Plane.Pix)
}
