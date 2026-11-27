package ass

import (
	"math"
	"sort"
	"strings"
	"time"
)

// DefaultSampleInterval is the step used to approximate a continuous ASS
// transform with static WebVTT cues.
const DefaultSampleInterval = 40 * time.Millisecond

// Options controls static segmentation.
type Options struct {
	// SampleInterval is the maximum duration of a static cue while at least one
	// transform is changing. Zero selects DefaultSampleInterval.
	SampleInterval time.Duration
}

// Cue is a static interval produced from one ASS event.
type Cue struct {
	Start time.Duration
	End   time.Duration
	Text  string
	State Transform
}

type track struct {
	prop            prop
	start           time.Duration
	end             time.Duration
	accel           float64
	order           int
	from            float64
	to              float64
	hasExplicitFrom bool
}

func (line *timeline) tracks(duration time.Duration) []track {
	var tracks []track
	order := 0
	for _, anim := range line.animations {
		start := anim.start
		end := anim.end
		if end < 0 {
			end = duration
		}
		if start < 0 {
			start = 0
		}
		if end > duration {
			end = duration
		}
		if start > duration || end < start {
			continue
		}
		for key, value := range anim.changes {
			current := track{
				prop:  key,
				start: start,
				end:   end,
				accel: anim.accel,
				order: order,
				to:    value,
			}
			if initial, ok := anim.initial[key]; ok {
				current.from = initial
				current.hasExplicitFrom = true
			}
			tracks = append(tracks, current)
		}
		order++
	}
	sort.SliceStable(tracks, func(i, j int) bool {
		if tracks[i].start != tracks[j].start {
			return tracks[i].start < tracks[j].start
		}
		return tracks[i].order < tracks[j].order
	})

	for i := range tracks {
		if !tracks[i].hasExplicitFrom {
			tracks[i].from = line.propertyBefore(&tracks[i], tracks[:i])
		}
	}
	return tracks
}

func (line *timeline) propertyBefore(current *track, earlier []track) float64 {
	value, ok := baseProperty(line.base, current.prop)
	for i := range earlier {
		candidate := &earlier[i]
		if candidate.prop != current.prop {
			continue
		}
		if candidate.start > current.start ||
			candidate.start == current.start && candidate.order >= current.order {
			continue
		}
		value = candidate.valueAt(current.start)
		ok = true
	}
	if ok {
		return value
	}
	return 0
}

func baseProperty(state Transform, key prop) (float64, bool) {
	switch key {
	case propScaleX:
		return state.ScaleX, true
	case propScaleY:
		return state.ScaleY, true
	case propRotateX:
		return state.RotateX, true
	case propRotateY:
		return state.RotateY, true
	case propRotateZ:
		return state.RotateZ, true
	case propPosX:
		if state.Position == nil {
			return 0, false
		}
		return state.Position.X, true
	case propPosY:
		if state.Position == nil {
			return 0, false
		}
		return state.Position.Y, true
	case propOriginX:
		if state.Origin == nil {
			return 0, false
		}
		return state.Origin.X, true
	case propOriginY:
		if state.Origin == nil {
			return 0, false
		}
		return state.Origin.Y, true
	case propAlignment:
		return float64(state.Alignment), true
	}
	return 0, false
}

func (track *track) valueAt(at time.Duration) float64 {
	if at <= track.start {
		return track.from
	}
	if at >= track.end || track.end <= track.start {
		return track.to
	}
	progress := float64(at-track.start) / float64(track.end-track.start)
	progress = ease(progress, track.accel)
	return track.from + (track.to-track.from)*progress
}

func ease(progress, acceleration float64) float64 {
	if acceleration == 0 || acceleration == 1 || math.IsNaN(acceleration) {
		return progress
	}
	if acceleration <= 0 {
		acceleration = 1
	}
	// ASS treats values below 1 as accelerating and values above 1 as
	// decelerating. Clamping keeps malformed values monotonic and bounded.
	progress = math.Pow(progress, acceleration)
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}

func stateAt(base Transform, tracks []track, at time.Duration) Transform {
	state := base.Clone()
	selected := make(map[prop]*track, len(tracks))
	for i := range tracks {
		candidate := &tracks[i]
		if candidate.start > at {
			continue
		}
		current := selected[candidate.prop]
		if current == nil ||
			candidate.start > current.start ||
			candidate.start == current.start && candidate.order >= current.order {
			selected[candidate.prop] = candidate
		}
	}
	for key, current := range selected {
		applyProperty(&state, key, current.valueAt(at))
	}
	return state
}

func segmentPoints(duration time.Duration, tracks []track, sample time.Duration) []time.Duration {
	points := []time.Duration{0}
	if duration > 0 {
		points = append(points, duration)
	}
	for i := range tracks {
		track := &tracks[i]
		points = append(points, clampDuration(track.start, duration), clampDuration(track.end, duration))
	}
	if sample <= 0 {
		sample = DefaultSampleInterval
	}
	for i := range tracks {
		track := &tracks[i]
		for at := track.start; at < track.end; at += sample {
			points = append(points, clampDuration(at, duration))
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	unique := points[:0]
	for _, point := range points {
		if len(unique) == 0 || unique[len(unique)-1] != point {
			unique = append(unique, point)
		}
	}
	return unique
}

func clampDuration(value, maximum time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	if value > maximum {
		return maximum
	}
	return value
}

// Segments converts every dialogue event into static intervals. Transform
// boundaries are always present; continuous transitions are additionally
// sampled so the final state is never applied to the beginning of the cue.
func (script *Script) Segments(options Options) ([]Cue, error) {
	var cues []Cue
	for _, event := range script.Events {
		if event.End <= event.Start {
			continue
		}
		line, err := parseLine(event.Text)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(line.plain) == "" {
			continue
		}
		eventDuration := event.End - event.Start
		tracks := line.timeline.tracks(eventDuration)
		points := segmentPoints(eventDuration, tracks, options.SampleInterval)
		firstEventCue := len(cues)
		for i := 0; i+1 < len(points); i++ {
			start, end := points[i], points[i+1]
			if end <= start {
				continue
			}
			cue := Cue{
				Start: event.Start + start,
				End:   event.Start + end,
				Text:  line.plain,
				State: stateAt(line.timeline.base, tracks, start),
			}
			if len(cues) > firstEventCue {
				last := &cues[len(cues)-1]
				if last.End == cue.Start && last.Text == cue.Text && last.State.Equal(cue.State) {
					last.End = cue.End
					continue
				}
			}
			cues = append(cues, cue)
		}
	}
	sort.SliceStable(cues, func(i, j int) bool {
		if cues[i].Start != cues[j].Start {
			return cues[i].Start < cues[j].Start
		}
		return cues[i].End < cues[j].End
	})
	return cues, nil
}
