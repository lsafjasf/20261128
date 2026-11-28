package ltc

import (
	"fmt"
	"strconv"
	"strings"
)

func FramesPerDay(nominalFPS int, drop bool) int {
	n := 24 * 60 * 60 * nominalFPS
	if drop && nominalFPS == 30 {
		n -= 2 * (24*60 - 24*6)
	}
	return n
}

// FrameNumber converts a time address to a zero-based frame count within one
// 24-hour day. It rejects labels omitted by 29.97 drop-frame counting.
func FrameNumber(tc Timecode, nominalFPS int, drop bool) (int, error) {
	if nominalFPS <= 0 {
		return 0, fmt.Errorf("invalid nominal frame rate %d", nominalFPS)
	}
	if tc.Hours < 0 || tc.Hours > 23 || tc.Minutes < 0 || tc.Minutes > 59 ||
		tc.Seconds < 0 || tc.Seconds > 59 || tc.Frames < 0 || tc.Frames >= nominalFPS {
		return 0, fmt.Errorf("time address %s out of range for %d fps", tc, nominalFPS)
	}
	base := (tc.Hours*3600+tc.Minutes*60+tc.Seconds)*nominalFPS + tc.Frames
	if !drop {
		return base, nil
	}
	if nominalFPS != 30 {
		return 0, fmt.Errorf("drop-frame is only defined for nominal 30 fps")
	}
	if tc.Minutes%10 != 0 && tc.Seconds == 0 && tc.Frames < 2 {
		return 0, fmt.Errorf("drop-frame label %s is never transmitted", tc)
	}
	totalMinutes := tc.Hours*60 + tc.Minutes
	return base - 2*(totalMinutes-totalMinutes/10), nil
}

// ParseTimecode accepts HH:MM:SS:FF and HH:MM:SS;FF. The separator only
// signals the caller's intent; the authoritative drop-frame bit is in Frame.
func ParseTimecode(s string) (Timecode, bool, error) {
	s = strings.TrimSpace(s)
	if len(s) != 11 {
		return Timecode{}, false, fmt.Errorf("timecode %q must be HH:MM:SS:FF or HH:MM:SS;FF", s)
	}
	drop := s[8] == ';'
	if s[2] != ':' || s[5] != ':' || (s[8] != ':' && !drop) {
		return Timecode{}, false, fmt.Errorf("invalid timecode separators in %q", s)
	}
	parts := []string{s[0:2], s[3:5], s[6:8], s[9:11]}
	values := make([]int, 4)
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return Timecode{}, false, fmt.Errorf("invalid timecode number %q", p)
		}
		values[i] = v
	}
	return Timecode{Hours: values[0], Minutes: values[1], Seconds: values[2], Frames: values[3]}, drop, nil
}
