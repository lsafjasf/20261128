package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"

	"yuvdiag"
)

// writePPM writes the already clipped, matrix-corrected RGB frame. Values are
// big-endian when the selected RGB depth is greater than 8 bits.
func writePPM(path string, frame *yuv.RGBFrame) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := bufio.NewWriter(file)
	max := (1 << frame.BitDepth) - 1
	if _, err := fmt.Fprintf(w, "P6\n%d %d\n%d\n", frame.Width, frame.Height, max); err != nil {
		return err
	}
	var word [2]byte
	for _, p := range frame.Pix {
		if frame.BitDepth <= 8 {
			if err := w.WriteByte(byte(p.R)); err != nil {
				return err
			}
			if err := w.WriteByte(byte(p.G)); err != nil {
				return err
			}
			if err := w.WriteByte(byte(p.B)); err != nil {
				return err
			}
			continue
		}
		for _, v := range []uint16{p.R, p.G, p.B} {
			binary.BigEndian.PutUint16(word[:], v)
			if _, err := w.Write(word[:]); err != nil {
				return err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return nil
}
