package ltc

import (
	"fmt"
	"time"
)

// DecodeDate decodes SMPTE ST 309 date/time-zone data from the eight binary
// groups. Other user-bit conventions are intentionally not guessed.
func DecodeDate(f Frame) DateInfo {
	info := DateInfo{Status: DateAbsent}
	if !f.BGF.BGF2 || f.BGF.BGF1 {
		info.Reason = fmt.Sprintf("BGF2=%d BGF1=%d does not declare an ST 309 date",
			boolInt(f.BGF.BGF2), boolInt(f.BGF.BGF1))
		return info
	}

	g := f.UserGroups
	for i, v := range g[:6] {
		if v > 9 {
			info.Status = DateInvalid
			info.Reason = fmt.Sprintf("binary group %d is not a BCD digit", i+1)
			return info
		}
	}

	code := int(g[6]&0x0F) | int(g[7]&0x03)<<4
	info.TimezoneCode = &code
	info.Timezone, info.Reason = timezoneName(code)
	info.DST = g[7]&0x04 != 0
	info.MJD = g[7]&0x08 != 0
	if info.MJD {
		info.Encoding = "smpte-st309-mjd"
		mjd := int(g[0]) + int(g[1])*10 + int(g[2])*100 +
			int(g[3])*1000 + int(g[4])*10000 + int(g[5])*100000
		info.Raw = fmt.Sprintf("%06d", mjd)
		date := time.Date(1858, time.November, 17, 0, 0, 0, 0, time.UTC).AddDate(0, 0, mjd)
		info.Status = DateDetermined
		info.Date = date.Format("2006-01-02")
		return info
	}

	info.Encoding = "smpte-st309-ddmmyy"
	day := int(g[0]) + int(g[1])*10
	month := int(g[2]) + int(g[3])*10
	year := int(g[4]) + int(g[5])*10
	info.Raw = fmt.Sprintf("%02d-%02d-%02d", day, month, year)
	if day < 1 || day > 31 || month < 1 || month > 12 ||
		!validMonthDay(month, day) {
		info.Status = DateInvalid
		info.Reason = "invalid DDMMYY date"
		return info
	}
	info.Status = DateCenturyAmbiguous
	info.Reason = "DDMMYY carries a two-digit year; the calendar century is not encoded by LTC"
	return info
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func validMonthDay(month, day int) bool {
	// Year 2000 preserves the 29 February possibility; the century is unknown.
	t := time.Date(2000, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return t.Month() == time.Month(month) && t.Day() == day
}

var timezoneOffsets = map[int]string{
	0x00: "+00:00", 0x01: "-01:00", 0x02: "-02:00", 0x03: "-03:00",
	0x04: "-04:00", 0x05: "-05:00", 0x06: "-06:00", 0x07: "-07:00",
	0x08: "-08:00", 0x09: "-09:00", 0x10: "-10:00", 0x11: "-11:00",
	0x12: "-12:00", 0x13: "+13:00", 0x14: "+12:00", 0x15: "+11:00",
	0x16: "+10:00", 0x17: "+09:00", 0x18: "+08:00", 0x19: "+07:00",
	0x20: "+06:00", 0x21: "+05:00", 0x22: "+04:00", 0x23: "+03:00",
	0x24: "+02:00", 0x25: "+01:00", 0x0A: "-00:30", 0x0B: "-01:30",
	0x0C: "-02:30", 0x0D: "-03:30", 0x0E: "-04:30", 0x0F: "-05:30",
	0x1A: "-06:30", 0x1B: "-07:30", 0x1C: "-08:30", 0x1D: "-09:30",
	0x1E: "-10:30", 0x1F: "-11:30", 0x2A: "+11:30", 0x2B: "+10:30",
	0x2C: "+09:30", 0x2D: "+08:30", 0x2E: "+07:30", 0x2F: "+06:30",
	0x3A: "+05:30", 0x3B: "+04:30", 0x3C: "+03:30", 0x3D: "+02:30",
	0x3E: "+01:30", 0x3F: "+00:30", 0x32: "+12:45", 0x38: "user-defined",
}

func timezoneName(code int) (string, string) {
	if name, ok := timezoneOffsets[code]; ok {
		return name, ""
	}
	if code >= 0x26 && code <= 0x31 {
		return "", "timezone code carries deprecated time-precision information"
	}
	return "", fmt.Sprintf("unknown timezone code 0x%02X", code)
}
