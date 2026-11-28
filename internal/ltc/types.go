package ltc

import "fmt"

// Timecode is a raw LTC time address. It deliberately contains no calendar
// date: the 64 information bits only define a 24-hour clock.
type Timecode struct {
	Hours   int `json:"hours"`
	Minutes int `json:"minutes"`
	Seconds int `json:"seconds"`
	Frames  int `json:"frames"`
}

func (tc Timecode) String() string {
	return fmt.Sprintf("%02d:%02d:%02d:%02d", tc.Hours, tc.Minutes, tc.Seconds, tc.Frames)
}

// DropFrameString uses the SMPTE ; frame separator for drop-frame addresses.
func (tc Timecode) DropFrameString(drop bool) string {
	sep := ":"
	if drop {
		sep = ";"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%02d", tc.Hours, tc.Minutes, tc.Seconds, sep, tc.Frames)
}

type BinaryGroupFlags struct {
	BGF0 bool `json:"bgf0"`
	BGF1 bool `json:"bgf1"`
	BGF2 bool `json:"bgf2"`
}

type Frame struct {
	Bits         string           `json:"bits"`
	Timecode     Timecode         `json:"timecode"`
	DropFrame    bool             `json:"dropFrame"`
	ColorFrame   bool             `json:"colorFrame"`
	Polarity     bool             `json:"polarity"`
	BGF          BinaryGroupFlags `json:"binaryGroupFlags"`
	UserGroups   [8]uint8         `json:"userGroups"`
	UserBits     string           `json:"userBits"`
	SyncValid    bool             `json:"syncValid"`
	InfoBitsOnly bool             `json:"infoBitsOnly"`
	Warnings     []string         `json:"warnings,omitempty"`
	Date         DateInfo         `json:"date"`
}

type DateStatus string

const (
	DateAbsent           DateStatus = "absent"
	DateInvalid          DateStatus = "invalid"
	DateCenturyAmbiguous DateStatus = "century-ambiguous"
	DateDetermined       DateStatus = "determined"
	DateMixed            DateStatus = "mixed"
)

type DateInfo struct {
	Status       DateStatus `json:"status"`
	Encoding     string     `json:"encoding,omitempty"`
	Raw          string     `json:"raw,omitempty"`
	Date         string     `json:"date,omitempty"`
	Candidates   []string   `json:"candidates,omitempty"`
	TimezoneCode *int       `json:"timezoneCode,omitempty"`
	Timezone     string     `json:"timezone,omitempty"`
	DST          bool       `json:"dst,omitempty"`
	MJD          bool       `json:"mjd,omitempty"`
	Reason       string     `json:"reason,omitempty"`
}
