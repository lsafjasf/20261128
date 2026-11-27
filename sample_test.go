package yuv

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReadYUV420P10LE(t *testing.T) {
	layout := NewLayout(4, 4, 10, YUV420P10LE)
	size, err := layout.FrameSize()
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 0, size)
	for i := 0; i < int(size)/2; i++ {
		v := uint16(512)
		switch i {
		case 0:
			v = 64
		case 1:
			v = 940
		case 16:
			v = 960
		}
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)
		buf = append(buf, b[:]...)
	}
	frame, err := ReadFrameAt(bytes.NewReader(buf), 0, layout)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Y.Sample(0, 0) != 64 || frame.Y.Sample(1, 0) != 940 ||
		frame.Cb.Sample(0, 0) != 960 {
		t.Fatalf("unexpected samples: Y=%d,%d Cb=%d",
			frame.Y.Sample(0, 0), frame.Y.Sample(1, 0), frame.Cb.Sample(0, 0))
	}
}

func TestReadFrameRejectsTruncatedInput(t *testing.T) {
	layout := NewLayout(4, 4, 10, YUV420P10LE)
	frame, err := ReadFrameAt(bytes.NewReader(make([]byte, 8)), 0, layout)
	if err == nil || frame != nil {
		t.Fatalf("expected truncated-frame error")
	}
}

func TestReadFramePreservesOutOfBitDepthCode(t *testing.T) {
	layout := NewLayout(2, 2, 10, YUV420P10LE)
	buf := make([]byte, 0, 12*2)
	for _, v := range []uint16{1024, 512, 512, 64, 512, 512, 64, 512, 512, 64, 512, 512} {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)
		buf = append(buf, b[:]...)
	}
	frame, err := ReadFrameAt(bytes.NewReader(buf), 0, layout)
	if err != nil {
		t.Fatal(err)
	}
	if got := frame.Y.Sample(0, 0); got != 1024 {
		t.Fatalf("Y sample = %d, want unmodified 1024", got)
	}
	if frame.BitDepthAnomaly != 1 {
		t.Fatalf("bit-depth anomalies = %d, want 1", frame.BitDepthAnomaly)
	}
}

func TestReadP010(t *testing.T) {
	layout := NewLayout(2, 2, 10, P010LE)
	buf := make([]byte, 0, 12*2)
	for _, code := range []uint16{64, 940, 512, 512, 512, 512, 512, 512, 512, 512, 512, 512} {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], code<<6|1)
		buf = append(buf, b[:]...)
	}
	frame, err := ReadFrameAt(bytes.NewReader(buf), 0, layout)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Y.Sample(0, 0) != 64 || frame.Y.Sample(1, 0) != 940 {
		t.Fatalf("P010 normalized samples = %d,%d", frame.Y.Sample(0, 0), frame.Y.Sample(1, 0))
	}
	if frame.PaddingAnomaly != 6 {
		t.Fatalf("padding anomalies = %d, want 6", frame.PaddingAnomaly)
	}
}
