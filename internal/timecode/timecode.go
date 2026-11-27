// Package timecode maps integer output-frame indexes to SMPTE labels.
//
// Frame indexes are zero based. 23.976 uses nominal 24 fps labels without
// drops. 29.97DF uses nominal 30 fps labels and skips ;00 and ;01 at the start
// of every minute except each tenth minute.
package timecode

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrNegativeFrame = errors.New("frame index must not be negative")
	ErrInvalidRate   = errors.New("unsupported frame rate")
	ErrInvalidLabel  = errors.New("invalid timecode label")
	ErrOverflow      = errors.New("timecode exceeds 99 hours")
)

// Rate describes both an exact media rate and the integer rate used for labels.
// Num/Den are the real frame rate; Nominal is the SMPTE label rate.
type Rate struct {
	name       string
	num        int64
	den        int64
	nominal    int64
	dropFrames int64
}

// Rate23976 is 24000/1001 fps with non-drop 24 fps timecode.
func Rate23976() Rate {
	return Rate{name: "23.976", num: 24000, den: 1001, nominal: 24}
}

// Rate2997DF is 30000/1001 fps with 30 fps drop-frame timecode.
func Rate2997DF() Rate {
	return Rate{name: "29.97DF", num: 30000, den: 1001, nominal: 30, dropFrames: 2}
}

// Rate2997ND is 30000/1001 fps with non-drop 30 fps timecode.
func Rate2997ND() Rate {
	return Rate{name: "29.97ND", num: 30000, den: 1001, nominal: 30}
}

// ParseRate accepts the names emitted by String and common aliases.
func ParseRate(s string) (Rate, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "23.976", "23.98", "24000/1001":
		return Rate23976(), nil
	case "29.97df", "29.97-df", "29.97dfps", "30000/1001df":
		return Rate2997DF(), nil
	case "29.97", "29.97nd", "29.97-nd", "30000/1001":
		return Rate2997ND(), nil
	default:
		return Rate{}, fmt.Errorf("%w: %q", ErrInvalidRate, s)
	}
}

func (r Rate) String() string {
	if r.name != "" {
		return r.name
	}
	return fmt.Sprintf("%d/%d", r.num, r.den)
}

func (r Rate) Actual() (num, den int64) {
	return r.num, r.den
}

func (r Rate) NominalFPS() int {
	return int(r.nominal)
}

func (r Rate) DropFrame() bool {
	return r.dropFrames > 0
}

func (r Rate) valid() bool {
	if r.num <= 0 || r.den <= 0 || r.nominal <= 0 {
		return false
	}
	return r.dropFrames == 0 || r.nominal == 30
}

// Timecode is a validated SMPTE wall label.
type Timecode struct {
	Hours   int
	Minutes int
	Seconds int
	Frames  int
	Drop    bool
}

func (tc Timecode) String() string {
	sep := ":"
	if tc.Drop {
		sep = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d",
		tc.Hours, tc.Minutes, tc.Seconds, sep, tc.Frames)
}

// At returns the label for a zero-based output frame index.
func (r Rate) At(frame int64) (Timecode, error) {
	if !r.valid() {
		return Timecode{}, ErrInvalidRate
	}
	if frame < 0 {
		return Timecode{}, ErrNegativeFrame
	}
	labelled, err := r.labelNumber(frame)
	if err != nil {
		return Timecode{}, err
	}
	perSecond := r.nominal
	perMinute := perSecond * 60
	perHour := perMinute * 60
	hours := labelled / perHour
	if hours > 99 {
		return Timecode{}, ErrOverflow
	}
	remainder := labelled % perHour
	return Timecode{
		Hours:   int(hours),
		Minutes: int(remainder / perMinute),
		Seconds: int((remainder % perMinute) / perSecond),
		Frames:  int(remainder % perSecond),
		Drop:    r.dropFrames > 0,
	}, nil
}

// labelNumber converts an output index to a count in the label namespace.
// The result excludes labels omitted by drop-frame numbering.
func (r Rate) labelNumber(frame int64) (int64, error) {
	if r.dropFrames == 0 {
		return frame, nil
	}
	const minutesPerCycle = 10
	framesPerMinute := r.nominal*60 - r.dropFrames
	framesPerCycle := r.nominal*60*minutesPerCycle - r.dropFrames*(minutesPerCycle-1)
	cycles := frame / framesPerCycle
	rest := frame % framesPerCycle
	dropped := cycles * r.dropFrames * (minutesPerCycle - 1)
	if rest >= r.dropFrames {
		dropped += r.dropFrames * ((rest - r.dropFrames) / framesPerMinute)
	}
	if dropped > int64(^uint64(0)>>1)-frame {
		return 0, ErrOverflow
	}
	return frame + dropped, nil
}

// Frame parses a timecode back to a zero-based frame index.
func (r Rate) Frame(s string) (int64, error) {
	if !r.valid() {
		return 0, ErrInvalidRate
	}
	t := strings.TrimSpace(s)
	if len(t) != 11 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLabel, s)
	}
	sep := t[8]
	if (sep != ':' && sep != ';') || t[2] != ':' || t[5] != ':' {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLabel, s)
	}
	parts := []string{t[0:2], t[3:5], t[6:8], t[9:11]}
	values := make([]int64, 4)
	for i, part := range parts {
		v, err := strconv.ParseInt(part, 10, 64)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("%w: %q", ErrInvalidLabel, s)
		}
		values[i] = v
	}
	hh, mm, ss, ff := values[0], values[1], values[2], values[3]
	if hh > 99 || mm > 59 || ss > 59 || ff >= r.nominal {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLabel, s)
	}
	totalMinutes := hh*60 + mm
	if r.dropFrames > 0 && mm%10 != 0 && ss == 0 && ff < r.dropFrames {
		return 0, fmt.Errorf("%w: drop-frame label %q does not exist", ErrInvalidLabel, s)
	}
	frame := (hh*3600+mm*60+ss)*r.nominal + ff
	frame -= r.dropFrames * (totalMinutes - totalMinutes/10)
	if frame < 0 {
		return 0, fmt.Errorf("%w: %q", ErrInvalidLabel, s)
	}
	return frame, nil
}

// Seconds converts an output frame index to exact media seconds.
func (r Rate) Seconds(frame int64) float64 {
	return float64(frame*r.den) / float64(r.num)
}
