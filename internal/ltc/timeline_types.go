package ltc

type FrameObservation struct {
	Frame
	Index               int    `json:"index"`
	TimeAddress         string `json:"timeAddress"`
	FrameNumber         *int   `json:"frameNumber,omitempty"`
	RelativeDay         int    `json:"relativeDay"`
	Chronology          string `json:"chronology"`
	ChronologyAmbiguous bool   `json:"chronologyAmbiguous"`
	SegmentIndex        int    `json:"segmentIndex"`
}

type Segment struct {
	Index            int        `json:"index"`
	StartFrameIndex  int        `json:"startFrameIndex"`
	EndFrameIndex    int        `json:"endFrameIndex"`
	StartTimecode    string     `json:"startTimecode"`
	EndTimecode      string     `json:"endTimecode"`
	StartDate        string     `json:"startDate,omitempty"`
	EndDate          string     `json:"endDate,omitempty"`
	DateStatus       DateStatus `json:"dateStatus"`
	RelativeDayStart int        `json:"relativeDayStart"`
	RelativeDayEnd   int        `json:"relativeDayEnd"`
	CrossesMidnight  bool       `json:"crossesMidnight"`
	FrameCount       int        `json:"frameCount"`
	Chronology       string     `json:"chronology"`
	Ambiguous        bool       `json:"ambiguous"`
	AmbiguityReasons []string   `json:"ambiguityReasons,omitempty"`
	StartUTC         string     `json:"startUtc,omitempty"`
	EndUTC           string     `json:"endUtc,omitempty"`
}

type Event struct {
	Type             string `json:"type"`
	FrameIndex       int    `json:"frameIndex"`
	Timecode         string `json:"timecode"`
	PreviousIndex    *int   `json:"previousFrameIndex,omitempty"`
	PreviousTimecode string `json:"previousTimecode,omitempty"`
	DeltaFrames      *int   `json:"deltaFrames,omitempty"`
	Ambiguous        bool   `json:"ambiguous"`
	Message          string `json:"message"`
}

type Timeline struct {
	Label        string             `json:"label"`
	NominalFPS   int                `json:"nominalFps"`
	InputFormat  string             `json:"inputFormat,omitempty"`
	BitOrder     string             `json:"bitOrder,omitempty"`
	FPSInferred  bool               `json:"fpsInferred"`
	FrameCount   int                `json:"frameCount"`
	RelativeDays int                `json:"relativeDayCount"`
	Frames       []FrameObservation `json:"frames"`
	Segments     []Segment          `json:"segments"`
	Events       []Event            `json:"events"`
	Ambiguities  []string           `json:"ambiguities,omitempty"`
}

type MergeItem struct {
	Source        string     `json:"source"`
	Segment       int        `json:"segment"`
	StartTimecode string     `json:"startTimecode"`
	EndTimecode   string     `json:"endTimecode"`
	StartDate     string     `json:"startDate,omitempty"`
	StartUTC      string     `json:"startUtc,omitempty"`
	DateStatus    DateStatus `json:"dateStatus"`
	Chronology    string     `json:"chronology"`
}

type Merge struct {
	Ordering string      `json:"ordering"`
	Reason   string      `json:"reason"`
	Items    []MergeItem `json:"items"`
}

type Analysis struct {
	SchemaVersion int        `json:"schemaVersion"`
	Assumptions   []string   `json:"assumptions"`
	Sources       []Timeline `json:"sources"`
	CrossSource   Merge      `json:"crossSource"`
}
