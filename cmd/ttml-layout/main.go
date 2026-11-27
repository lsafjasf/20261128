package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"ttmlruby/fontmetrics"
	"ttmlruby/layout"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			*s = append(*s, item)
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ttml-layout:", err)
		os.Exit(2)
	}
}

func run() error {
	var fonts stringList
	input := flag.String("in", "-", "TTML input file, or - for stdin")
	output := flag.String("out", "-", "JSON output file, or - for stdout")
	flag.Var(&fonts, "font", "font file; repeat or use commas for fallback order")
	width := flag.Float64("width", 0, "viewport width override in pixels")
	height := flag.Float64("height", 0, "viewport height override in pixels")
	regionID := flag.String("region", "", "region id (defaults to the paragraph region)")
	paragraph := flag.Int("paragraph", 0, "zero-based paragraph index")
	left := flag.Float64("padding-left", 0, "extra left inset in pixels")
	right := flag.Float64("padding-right", 0, "extra right inset in pixels")
	top := flag.Float64("padding-top", 0, "extra top inset in pixels")
	bottom := flag.Float64("padding-bottom", 0, "extra bottom inset in pixels")
	rubyGap := flag.Float64("ruby-gap", 0, "gap between base and nearest ruby layer")
	rubyScale := flag.Float64("ruby-scale", 0.5, "ruby font size relative to base")
	fallback := flag.String("fallback", "\uFFFD", "replacement rune for missing glyphs")
	compact := flag.Bool("compact", false, "emit compact instead of indented JSON")
	flag.Parse()

	data, err := readInput(*input)
	if err != nil {
		return err
	}
	fallbackRune, _ := utf8.DecodeRuneInString(*fallback)
	if fallbackRune == utf8.RuneError && *fallback != "\uFFFD" {
		return errors.New("-fallback must contain a rune")
	}
	face, err := loadFaceChain(fonts)
	if err != nil {
		return err
	}
	result, err := layout.LayoutTTML(data, layout.Options{
		ViewportWidth:  *width,
		ViewportHeight: *height,
		RegionID:       *regionID,
		Paragraph:      *paragraph,
		Face:           face,
		FallbackRune:   fallbackRune,
		LeftPadding:    *left,
		RightPadding:   *right,
		TopPadding:     *top,
		BottomPadding:  *bottom,
		RubyGap:        *rubyGap,
		RubyScale:      *rubyScale,
	})
	if err != nil {
		return err
	}

	var encoded []byte
	if *compact {
		encoded, err = json.Marshal(result)
	} else {
		encoded, err = json.MarshalIndent(result, "", "  ")
	}
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if *output == "-" {
		_, err = os.Stdout.Write(encoded)
		return err
	}
	return os.WriteFile(*output, encoded, 0o644)
}

func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func loadFaceChain(fonts []string) (layout.FaceChain, error) {
	paths := fonts
	explicit := len(paths) > 0
	if !explicit {
		paths = discoverFonts()
	}
	var chain layout.FaceChain
	for _, path := range paths {
		face, err := fontmetrics.Load(path)
		if err != nil {
			if explicit {
				return nil, err
			}
			continue
		}
		chain = append(chain, face)
	}
	if len(chain) == 0 {
		if explicit {
			return nil, fmt.Errorf("no usable font found (searched %s)", strings.Join(paths, ", "))
		}
		return layout.FaceChain{layout.SyntheticFace{}}, nil
	}
	if !explicit {
		chain = addCJKFallback(chain)
	}
	return chain, nil
}

func addCJKFallback(chain layout.FaceChain) layout.FaceChain {
	if _, ok := chain.Advance('漢'); ok {
		return chain
	}
	_ = filepath.WalkDir("/usr/share/fonts", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
			return nil
		}
		face, err := fontmetrics.Load(path)
		if err != nil {
			return nil
		}
		if _, ok := face.Advance('漢'); !ok {
			return nil
		}
		chain = append(chain, face)
		return fs.SkipAll
	})
	return chain
}

func discoverFonts() []string {
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansJP-Regular.ttf",
		"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
	}
	var found []string
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			found = append(found, path)
		}
	}
	if len(found) == 0 {
		_ = filepath.WalkDir("/usr/share/fonts", func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".ttf" || ext == ".otf" || ext == ".ttc" {
				found = append(found, path)
				return fs.SkipAll
			}
			return nil
		})
	}
	return found
}
