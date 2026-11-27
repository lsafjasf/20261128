package quant

import (
	"math"
	"testing"
)

func TestTruncateUsesArithmeticShift(t *testing.T) {
	source := []int32{255, 256, -255, -256}
	got := Truncate(source)
	want := []int32{0, 1, -1, -1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestHeadroomOnlyAttenuates(t *testing.T) {
	source := []int32{1000, -1000}
	got, gain := ApplyHeadroom(source, -1)
	if gain != 1 {
		t.Fatalf("low-level signal gain = %f, want 1", gain)
	}
	if got[0] != source[0] || got[1] != source[1] {
		t.Fatalf("low-level signal changed: %v", got)
	}

	loud := []int32{Int24Max, -Int24Max / 2}
	got, gain = ApplyHeadroom(loud, -1)
	if gain >= 1 {
		t.Fatalf("loud signal gain = %f, want attenuation", gain)
	}
	limit := int64(math.Round(0.891251 * float64(Int24Max)))
	if int64(got[0]) > limit+1 {
		t.Fatalf("headroom limit not applied: %d", got[0])
	}
}

func TestTPDFAndShapingStayWithin16BitRange(t *testing.T) {
	source := make([]int32, 4096)
	for i := range source {
		source[i] = int32(i%512) - 256
	}
	for _, values := range [][]int32{TPDFDither(source, 7), ShapedTPDF(source, 7)} {
		for _, value := range values {
			if value < Int16Min || value > Int16Max {
				t.Fatalf("value %d outside int16 range", value)
			}
		}
	}
}

func TestDitherMethodsHaveNearZeroMeanError(t *testing.T) {
	source := make([]int32, 16384)
	for i := range source {
		source[i] = 128 + int32((i*37)%512)
	}
	reference := make([]float64, len(source))
	for i, value := range source {
		reference[i] = float64(value) / CodeStep
	}
	for _, output := range [][]int32{TPDFDither(source, 11), ShapedTPDF(source, 11)} {
		meanError := 0.0
		for i, value := range output {
			meanError += reference[i] - float64(value)
		}
		meanError /= float64(len(output))
		if math.Abs(meanError) > 0.02 {
			t.Fatalf("mean error %.5f exceeds zero-mean tolerance", meanError)
		}
	}
}
