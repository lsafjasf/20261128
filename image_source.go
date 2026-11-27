package ghostdiag

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ImageSource reads a directory of PNG/JPEG frames in natural filename order.
type ImageSource struct {
	paths []string
	index int
	rate  float64
}

// NewImageSource validates and sorts a frame directory.
func NewImageSource(dir string, frameRate float64) (*ImageSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read image directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg":
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("image directory %q contains no PNG/JPEG frames", dir)
	}
	sort.Slice(paths, func(i, j int) bool {
		return naturalLess(filepath.Base(paths[i]), filepath.Base(paths[j]))
	})
	if frameRate <= 0 {
		frameRate = 25
	}
	return &ImageSource{paths: paths, rate: frameRate}, nil
}

func (s *ImageSource) Format() Format {
	return Format{FrameRate: s.rate, PixelFormat: "rgb", Origin: "images"}
}

func (s *ImageSource) Next() (Frame, error) {
	if s.index >= len(s.paths) {
		return Frame{}, ErrEndOfStream
	}
	path := s.paths[s.index]
	file, err := os.Open(path)
	if err != nil {
		return Frame{}, fmt.Errorf("open frame %q: %w", path, err)
	}
	img, _, decodeErr := image.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return Frame{}, fmt.Errorf("decode frame %q: %w", path, decodeErr)
	}
	if closeErr != nil {
		return Frame{}, fmt.Errorf("close frame %q: %w", path, closeErr)
	}
	plane := FromImage(img)
	frame := Frame{
		Index:     s.index,
		Timestamp: float64(s.index) / s.rate,
		Plane:     plane,
	}
	s.index++
	return frame, nil
}

func naturalLess(a, b string) bool {
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ad, bd := isDigit(a[ai]), isDigit(b[bi])
		if ad && bd {
			as, bs := ai, bi
			for ai < len(a) && isDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && isDigit(b[bi]) {
				bi++
			}
			an, _ := strconv.Atoi(a[as:ai])
			bn, _ := strconv.Atoi(b[bs:bi])
			if an != bn {
				return an < bn
			}
			continue
		}
		if a[ai] != b[bi] {
			return a[ai] < b[bi]
		}
		ai++
		bi++
	}
	return len(a) < len(b)
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
