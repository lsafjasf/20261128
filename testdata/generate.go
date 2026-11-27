//go:build ignore

// Run from the repository root with: go run testdata/generate.go
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

const (
	width    = 16
	height   = 8
	chromaHW = width / 2
	chromaHH = height / 2
)

func main() {
	ref := newFrame()
	decoded := newFrame()

	ref.y[0] = 63
	decoded.y[0] = 63
	decoded.y[1] = 941

	// A legal but out-of-gamut combination: it exercises display clipping
	// without creating a legal-range YUV code violation.
	decoded.y[8] = 940
	decoded.cr[4] = 960

	dir := "testdata"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reference_10bit.yuv"), ref.bytes(), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "decoded_10bit.yuv"), decoded.bytes(), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s and %s\n", "testdata/reference_10bit.yuv", "testdata/decoded_10bit.yuv")
}

type frame struct {
	y, cb, cr []uint16
}

func newFrame() frame {
	f := frame{
		y:  make([]uint16, width*height),
		cb: make([]uint16, chromaHW*chromaHH),
		cr: make([]uint16, chromaHW*chromaHH),
	}
	for i := range f.y {
		f.y[i] = 512
	}
	for y := 0; y < chromaHH; y++ {
		for x := 0; x < chromaHW; x++ {
			i := y*chromaHW + x
			f.cb[i] = uint16(480 + (x*7+y*11)%65)
			f.cr[i] = uint16(544 - (x*5+y*13)%65)
		}
	}
	// The two +/-1 luma violations use the first two chroma blocks. Keeping
	// both neutral demonstrates that nearest display rounding can hide them.
	f.cb[0], f.cr[0] = 512, 512
	f.cb[1], f.cr[1] = 512, 512
	return f
}

func (f frame) bytes() []byte {
	out := make([]byte, 0, (len(f.y)+len(f.cb)+len(f.cr))*2)
	for _, plane := range [][]uint16{f.y, f.cb, f.cr} {
		for _, v := range plane {
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], v)
			out = append(out, b[:]...)
		}
	}
	return out
}
