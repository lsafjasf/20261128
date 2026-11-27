package timecode

import "testing"

func Test23976DoesNotDrop(t *testing.T) {
	r := Rate23976()
	cases := map[int64]string{
		0:     "00:00:00:00",
		23:    "00:00:00:23",
		24:    "00:00:01:00",
		1439:  "00:00:59:23",
		1440:  "00:01:00:00",
		86400: "01:00:00:00",
	}
	for frame, want := range cases {
		got, err := r.At(frame)
		if err != nil {
			t.Fatalf("At(%d): %v", frame, err)
		}
		if got.String() != want {
			t.Errorf("At(%d) = %s, want %s", frame, got, want)
		}
		back, err := r.Frame(want)
		if err != nil || back != frame {
			t.Errorf("Frame(%s) = %d, %v; want %d", want, back, err, frame)
		}
	}
}

func Test2997DropFrameBoundaries(t *testing.T) {
	r := Rate2997DF()
	cases := map[int64]string{
		0:      "00:00:00;00",
		1799:   "00:00:59;29",
		1800:   "00:01:00;02",
		3597:   "00:01:59;29",
		3598:   "00:02:00;02",
		17981:  "00:09:59;29",
		17982:  "00:10:00;00",
		107892: "01:00:00;00",
	}
	for frame, want := range cases {
		got, err := r.At(frame)
		if err != nil {
			t.Fatalf("At(%d): %v", frame, err)
		}
		if got.String() != want {
			t.Errorf("At(%d) = %s, want %s", frame, got, want)
		}
		back, err := r.Frame(want)
		if err != nil || back != frame {
			t.Errorf("Frame(%s) = %d, %v; want %d", want, back, err, frame)
		}
	}
}

func TestDropFrameRejectsMissingLabels(t *testing.T) {
	r := Rate2997DF()
	for _, label := range []string{"00:01:00;00", "00:01:00;01", "01:23:00;01"} {
		if _, err := r.Frame(label); err == nil {
			t.Errorf("Frame(%q) accepted nonexistent label", label)
		}
	}
	for _, label := range []string{"00:10:00;00", "01:00:00;00", "00:01:00;02"} {
		if _, err := r.Frame(label); err != nil {
			t.Errorf("Frame(%q): %v", label, err)
		}
	}
}

func TestLongRunRoundTrip(t *testing.T) {
	for _, r := range []Rate{Rate23976(), Rate2997DF()} {
		for frame := int64(0); frame < 108000; frame += 97 {
			label, err := r.At(frame)
			if err != nil {
				t.Fatal(err)
			}
			back, err := r.Frame(label.String())
			if err != nil || back != frame {
				t.Fatalf("%s frame %d -> %s -> %d, %v", r, frame, label, back, err)
			}
		}
	}
}
