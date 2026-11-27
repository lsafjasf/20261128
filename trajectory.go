package ghostdiag

import (
	"encoding/json"
	"fmt"
	"os"
)

// Box is a frame-local motion region. Velocity is expressed in pixels per
// frame and points in the direction of travel.
type Box struct {
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"w"`
	Height int     `json:"h"`
	VX     float64 `json:"vx,omitempty"`
	VY     float64 `json:"vy,omitempty"`
}

// TrajectoryPoint associates one box with a frame index.
type TrajectoryPoint struct {
	Frame int `json:"frame"`
	Box
}

// Trajectory is an optional, explicit motion track. When it is omitted the
// analyzer estimates motion from adjacent original frames.
type Trajectory struct {
	Frames map[int]Box
}

type trajectoryFile struct {
	Points []TrajectoryPoint `json:"points"`
}

// LoadTrajectory reads either a JSON array or {"points": [...]}.
func LoadTrajectory(path string) (Trajectory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Trajectory{}, fmt.Errorf("read trajectory: %w", err)
	}
	var points []TrajectoryPoint
	if err := json.Unmarshal(data, &points); err != nil {
		var file trajectoryFile
		if err2 := json.Unmarshal(data, &file); err2 != nil {
			return Trajectory{}, fmt.Errorf("parse trajectory: %w", err)
		}
		points = file.Points
	}
	t := Trajectory{Frames: make(map[int]Box, len(points))}
	for _, point := range points {
		if point.Frame < 0 || point.Width <= 0 || point.Height <= 0 {
			return Trajectory{}, fmt.Errorf("invalid trajectory entry for frame %d", point.Frame)
		}
		if _, exists := t.Frames[point.Frame]; exists {
			return Trajectory{}, fmt.Errorf("duplicate trajectory frame %d", point.Frame)
		}
		t.Frames[point.Frame] = point.Box
	}
	return t, nil
}

func (t Trajectory) Box(frame int) (Box, bool) {
	if t.Frames == nil {
		return Box{}, false
	}
	box, ok := t.Frames[frame]
	return box, ok
}
