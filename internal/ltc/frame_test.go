package ltc

import "testing"

func TestDecode25FPSDateAndUserBits(t *testing.T) {
	groups := [8]uint8{}
	setDateDDMMYY(&groups, 27, 11, 26, 0x18)
	f := testFrame(t, Timecode{Hours: 12, Minutes: 34, Seconds: 56, Frames: 24},
		25, false, groups, false, false, true)

	if got := f.Timecode.String(); got != "12:34:56:24" {
		t.Fatalf("timecode = %s", got)
	}
	if !f.SyncValid || f.InfoBitsOnly {
		t.Fatalf("sync/info flags = %v/%v", f.SyncValid, f.InfoBitsOnly)
	}
	if f.UserBits != "72116281" {
		t.Fatalf("user bits = %s", f.UserBits)
	}
	if f.Date.Status != DateCenturyAmbiguous {
		t.Fatalf("date status = %s (%s)", f.Date.Status, f.Date.Reason)
	}
	if f.Date.Raw != "27-11-26" || f.Date.Timezone != "+08:00" {
		t.Fatalf("date = %+v", f.Date)
	}
}

func TestDecodeMJDDate(t *testing.T) {
	groups := [8]uint8{}
	setDateMJD(&groups, 49718)
	f := testFrame(t, Timecode{Hours: 23, Minutes: 59, Seconds: 59, Frames: 29},
		30, false, groups, false, false, true)

	if f.Date.Status != DateDetermined || f.Date.Date != "1995-01-01" {
		t.Fatalf("date = %+v", f.Date)
	}
	if !f.Date.MJD {
		t.Fatal("MJD flag was not decoded")
	}
}

func TestDropFrameCounting(t *testing.T) {
	tests := []struct {
		tc      Timecode
		want    int
		wantErr bool
	}{
		{Timecode{0, 0, 59, 29}, 1799, false},
		{Timecode{0, 1, 0, 2}, 1800, false},
		{Timecode{0, 1, 0, 0}, 0, true},
		{Timecode{0, 1, 0, 1}, 0, true},
		{Timecode{0, 10, 0, 0}, 17982, false},
		{Timecode{23, 59, 59, 29}, 2589407, false},
	}
	for _, tt := range tests {
		got, err := FrameNumber(tt.tc, 30, true)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr=%v", tt.tc, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("%s: frame number = %d, want %d", tt.tc, got, tt.want)
		}
	}
}

func TestDropFrameFlagAndAddress(t *testing.T) {
	f := testFrame(t, Timecode{Hours: 0, Minutes: 1, Seconds: 0, Frames: 2},
		30, true, [8]uint8{}, false, false, false)
	if !f.DropFrame {
		t.Fatal("drop-frame flag was not parsed")
	}
	if got := f.Timecode.DropFrameString(f.DropFrame); got != "00:01:00;02" {
		t.Fatalf("display address = %s", got)
	}
	n, err := FrameNumber(f.Timecode, 30, f.DropFrame)
	if err != nil || n != 1800 {
		t.Fatalf("frame number = %d, err = %v", n, err)
	}
}

func TestHexFrameMSBAndLSBAuto(t *testing.T) {
	groups := [8]uint8{}
	setDateDDMMYY(&groups, 28, 11, 26, 0x18)
	f := testFrame(t, Timecode{Hours: 0, Minutes: 0, Seconds: 0, Frames: 0},
		30, false, groups, false, false, true)

	for _, reverse := range []bool{false, true} {
		decoded, err := DecodeHexFrame(frameHex(f, reverse), 30, BitOrderAuto)
		if err != nil {
			t.Fatal(err)
		}
		if !decoded.SyncValid {
			t.Fatalf("reverse=%v: sync invalid: %v", reverse, decoded.Warnings)
		}
		if decoded.Timecode != f.Timecode || decoded.UserBits != f.UserBits {
			t.Fatalf("reverse=%v: decoded %+v, want %+v", reverse, decoded, f)
		}
		if decoded.Date.Status != DateCenturyAmbiguous {
			t.Fatalf("reverse=%v: date status %s", reverse, decoded.Date.Status)
		}
	}
}

func TestParseInfoBitsOnly(t *testing.T) {
	groups := [8]uint8{1, 2, 3, 4, 5, 6, 7, 8}
	f := testFrame(t, Timecode{Hours: 1, Minutes: 2, Seconds: 3, Frames: 4},
		30, false, groups, false, false, false)
	got, err := ParseBitString(f.Bits[:64], 30)
	if err != nil {
		t.Fatal(err)
	}
	if !got.InfoBitsOnly || got.SyncValid {
		t.Fatalf("infoOnly=%v sync=%v", got.InfoBitsOnly, got.SyncValid)
	}
	if got.Timecode.String() != "01:02:03:04" {
		t.Fatalf("timecode = %s", got.Timecode)
	}
}
