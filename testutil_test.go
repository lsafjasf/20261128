package yuv

func makeFrame(width, height, bitDepth int, y, cb, cr uint16) *Frame {
	cw, ch := (width+1)/2, (height+1)/2
	f := &Frame{
		Width: width, Height: height, BitDepth: bitDepth,
		ChromaWidth: cw, ChromaHeight: ch, SampleFormat: YUV420P10LE,
		Y:  Plane{Name: "Y", Width: width, Height: height, Stride: width},
		Cb: Plane{Name: "Cb", Width: cw, Height: ch, Stride: cw},
		Cr: Plane{Name: "Cr", Width: cw, Height: ch, Stride: cw},
	}
	f.Y.Pix = fillPlane(width*height, y)
	f.Cb.Pix = fillPlane(cw*ch, cb)
	f.Cr.Pix = fillPlane(cw*ch, cr)
	return f
}

func fillPlane(n int, value uint16) []uint16 {
	p := make([]uint16, n)
	for i := range p {
		p[i] = value
	}
	return p
}
