package mxfindex

import "fmt"

// FrameToTimecode converts an absolute frame count to SMPTE 12M notation.
// Drop-frame rates use a semicolon before the frame field.
func FrameToTimecode(frame int64, rate int, drop bool) string {
	if rate <= 0 {
		return ""
	}
	negative := frame < 0
	if negative {
		frame = -frame
	}
	label := frame
	if drop && (rate == 30 || rate == 60) {
		label = dropFrameLabel(frame, int64(rate))
	}
	framesPerSecond := int64(rate)
	framesPerMinute := framesPerSecond * 60
	framesPerHour := framesPerMinute * 60
	hours := label / framesPerHour
	label %= framesPerHour
	minutes := label / framesPerMinute
	label %= framesPerMinute
	seconds := label / framesPerSecond
	frames := label % framesPerSecond
	sep := byte(':')
	if drop && (rate == 30 || rate == 60) {
		sep = ';'
	}
	text := fmt.Sprintf("%02d:%02d:%02d%c%02d", hours, minutes, seconds, sep, frames)
	if negative {
		return "-" + text
	}
	return text
}

func dropFrameLabel(frame, nominal int64) int64 {
	dropPerMinute := nominal / 15
	framesPerMinute := nominal*60 - dropPerMinute
	framesPerTenMinutes := nominal*60*10 - dropPerMinute*9
	tenMinuteBlocks := frame / framesPerTenMinutes
	remainder := frame % framesPerTenMinutes
	label := frame + dropPerMinute*9*tenMinuteBlocks
	if remainder > dropPerMinute {
		label += dropPerMinute * ((remainder - dropPerMinute) / framesPerMinute)
	}
	return label
}

func timecodeAt(base TimecodeComponent, editRate Rational, editUnit uint64) string {
	if base.Rate <= 0 {
		return ""
	}
	frame := base.StartFrame
	roundedRate := editRate
	if roundedRate.Valid() {
		editFPS := (roundedRate.Num + roundedRate.Den/2) / roundedRate.Den
		if editFPS == int64(base.Rate) {
			frame += int64(editUnit)
			return FrameToTimecode(frame, base.Rate, base.DropFrame)
		}
		scale := roundedRate.Den * int64(base.Rate)
		if scale > 0 && roundedRate.Num > 0 {
			frame += int64(editUnit) * scale / roundedRate.Num
		}
	} else {
		frame += int64(editUnit)
	}
	return FrameToTimecode(frame, base.Rate, base.DropFrame)
}
