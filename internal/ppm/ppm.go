// Package ppm reads and writes binary P6 Portable Pixmap images.
package ppm

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	ErrInvalidImage = errors.New("invalid PPM image")
	ErrInvalidSize  = errors.New("invalid image dimensions")
)

type RGB struct {
	R uint8
	G uint8
	B uint8
}

var (
	Black = RGB{0, 0, 0}
	White = RGB{255, 255, 255}
)

type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r Rect) Empty() bool {
	return r.Width <= 0 || r.Height <= 0
}

func (r Rect) Intersect(other Rect) Rect {
	x0 := max(r.X, other.X)
	y0 := max(r.Y, other.Y)
	x1 := min(r.X+r.Width, other.X+other.Width)
	y1 := min(r.Y+r.Height, other.Y+other.Height)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

// Image stores tightly packed RGB88 pixels in row-major order.
type Image struct {
	Width  int
	Height int
	Pix    []byte
}

func New(width, height int, background RGB) (*Image, error) {
	if width <= 0 || height <= 0 || int64(width)*int64(height) > int64(^uint(0)>>1)/3 {
		return nil, ErrInvalidSize
	}
	im := &Image{Width: width, Height: height, Pix: make([]byte, width*height*3)}
	im.Fill(background)
	return im, nil
}

func (im *Image) Bounds() Rect {
	return Rect{Width: im.Width, Height: im.Height}
}

func (im *Image) Contains(x, y int) bool {
	return x >= 0 && y >= 0 && x < im.Width && y < im.Height
}

func (im *Image) At(x, y int) RGB {
	if !im.Contains(x, y) {
		return RGB{}
	}
	i := (y*im.Width + x) * 3
	return RGB{R: im.Pix[i], G: im.Pix[i+1], B: im.Pix[i+2]}
}

func (im *Image) Set(x, y int, c RGB) {
	if !im.Contains(x, y) {
		return
	}
	i := (y*im.Width + x) * 3
	im.Pix[i] = c.R
	im.Pix[i+1] = c.G
	im.Pix[i+2] = c.B
}

func (im *Image) Fill(c RGB) {
	for i := 0; i < len(im.Pix); i += 3 {
		im.Pix[i] = c.R
		im.Pix[i+1] = c.G
		im.Pix[i+2] = c.B
	}
}

func (im *Image) FillRect(r Rect, c RGB) {
	r = r.Intersect(im.Bounds())
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			im.Set(x, y, c)
		}
	}
}

// Clone returns an independent copy, used for frozen/paused content.
func (im *Image) Clone() *Image {
	clone := &Image{Width: im.Width, Height: im.Height, Pix: make([]byte, len(im.Pix))}
	copy(clone.Pix, im.Pix)
	return clone
}

func (im *Image) Write(w io.Writer) error {
	if im.Width <= 0 || im.Height <= 0 || len(im.Pix) != im.Width*im.Height*3 {
		return ErrInvalidImage
	}
	if _, err := fmt.Fprintf(w, "P6\n%d %d\n255\n", im.Width, im.Height); err != nil {
		return err
	}
	_, err := w.Write(im.Pix)
	return err
}

func (im *Image) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := im.Write(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func Read(r io.Reader) (*Image, error) {
	br := bufio.NewReader(r)
	magic, err := readToken(br)
	if err != nil {
		return nil, err
	}
	if magic != "P6" {
		return nil, fmt.Errorf("%w: magic %q", ErrInvalidImage, magic)
	}
	width, err := readInt(br)
	if err != nil {
		return nil, err
	}
	height, err := readInt(br)
	if err != nil {
		return nil, err
	}
	maxval, err := readInt(br)
	if err != nil {
		return nil, err
	}
	if maxval != 255 {
		return nil, fmt.Errorf("%w: maxval %d", ErrInvalidImage, maxval)
	}
	if width <= 0 || height <= 0 || int64(width)*int64(height) > int64(^uint(0)>>1)/3 {
		return nil, ErrInvalidSize
	}
	im := &Image{Width: width, Height: height, Pix: make([]byte, width*height*3)}
	if _, err := io.ReadFull(br, im.Pix); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImage, err)
	}
	return im, nil
}

func Load(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

func readToken(r *bufio.Reader) (string, error) {
	var token []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '#' {
			for b != '\n' {
				b, err = r.ReadByte()
				if err != nil {
					return "", err
				}
			}
			continue
		}
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			if len(token) > 0 {
				break
			}
			continue
		}
		token = append(token, b)
	}
	return string(token), nil
}

func readInt(r *bufio.Reader) (int, error) {
	token, err := readToken(r)
	if err != nil {
		return 0, err
	}
	var value int
	if _, err := fmt.Sscanf(token, "%d", &value); err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalidImage, token)
	}
	return value, nil
}
