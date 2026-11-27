package mediaorient

import (
	"fmt"
	"sync"
)

// SessionConfig fixes the two output canvases and their shared overlay rules.
// CaptionInsets are program pixels; Snapshot scales them to preview pixels so
// the normalized caption-safe area remains identical on both buses.
type SessionConfig struct {
	PreviewCanvas        Size    `json:"preview_canvas"`
	ProgramCanvas        Size    `json:"program_canvas"`
	Fit                  FitMode `json:"fit"`
	CaptionInsets        Insets  `json:"caption_insets"`
	CaptionsWithinActive bool    `json:"captions_within_active"`
}

// SourceView is the immutable resolved state of one source in both outputs.
type SourceView struct {
	Source  Source        `json:"source"`
	State   JointState    `json:"state"`
	Preview FrameGeometry `json:"preview"`
	Program FrameGeometry `json:"program"`
}

// SessionSnapshot is a coherent view of every source and the active buses.
// A source's preview and program always come from the same JointState.
type SessionSnapshot struct {
	PreviewID string                `json:"preview_id"`
	ProgramID string                `json:"program_id"`
	Sources   map[string]SourceView `json:"sources"`
}

type sourceEntry struct {
	source Source
	state  JointState
}

// Session owns all inputs and the active preview/program selection. Switching
// is committed under one lock only after the target source has resolved.
type Session struct {
	mu      sync.RWMutex
	config  SessionConfig
	sources map[string]sourceEntry
	preview string
	program string
}

// NewSession validates output configuration and returns an empty session.
func NewSession(config SessionConfig) (*Session, error) {
	if !config.PreviewCanvas.Valid() || !config.ProgramCanvas.Valid() {
		return nil, fmt.Errorf("%w: invalid preview/program canvas", ErrInvalidSession)
	}
	if config.Fit > FitStretch {
		return nil, fmt.Errorf("%w: unsupported fit mode %d", ErrInvalidSession, config.Fit)
	}
	if err := validateInsets(config.CaptionInsets); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSession, err)
	}
	return &Session{
		config:  config,
		sources: make(map[string]sourceEntry),
	}, nil
}

// AddSource resolves and registers a source. The first source becomes both
// preview and program without an intermediate unrotated frame.
func (s *Session) AddSource(source Source) error {
	if source.ID == "" {
		return fmt.Errorf("%w: source ID is empty", ErrInvalidSession)
	}
	state, err := source.Resolve()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sources[source.ID]; exists {
		return fmt.Errorf("%w: duplicate source %q", ErrInvalidSession, source.ID)
	}
	s.sources[source.ID] = sourceEntry{source: source, state: state}
	if s.program == "" {
		s.program = source.ID
	}
	if s.preview == "" {
		s.preview = source.ID
	}
	return nil
}

// SwitchProgram atomically changes the program bus.
func (s *Session) SwitchProgram(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sources[id]; !ok {
		return fmt.Errorf("%w: %q", ErrSourceNotFound, id)
	}
	s.program = id
	return nil
}

// SwitchPreview atomically changes the preview bus.
func (s *Session) SwitchPreview(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sources[id]; !ok {
		return fmt.Errorf("%w: %q", ErrSourceNotFound, id)
	}
	s.preview = id
	return nil
}

// Cut switches preview and program together. This is the safe path for a
// director cut: readers never observe one bus rotated and the other not.
func (s *Session) Cut(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sources[id]; !ok {
		return fmt.Errorf("%w: %q", ErrSourceNotFound, id)
	}
	s.preview = id
	s.program = id
	return nil
}

// Switch is an alias for Cut.
func (s *Session) Switch(id string) error { return s.Cut(id) }

// Snapshot resolves every source through the shared layout engine.
func (s *Session) Snapshot() (SessionSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.sources) == 0 {
		return SessionSnapshot{}, fmt.Errorf("%w: no sources", ErrInvalidSession)
	}

	snapshot := SessionSnapshot{
		PreviewID: s.preview,
		ProgramID: s.program,
		Sources:   make(map[string]SourceView, len(s.sources)),
	}
	previewInsets := scaleInsets(
		s.config.CaptionInsets, s.config.ProgramCanvas, s.config.PreviewCanvas,
	)
	for id, entry := range s.sources {
		preview, err := LayoutState(
			entry.state, OutputPreview, s.config.PreviewCanvas,
			s.config.Fit, previewInsets, s.config.CaptionsWithinActive,
		)
		if err != nil {
			return SessionSnapshot{}, fmt.Errorf("layout preview source %q: %w", id, err)
		}
		program, err := LayoutState(
			entry.state, OutputProgram, s.config.ProgramCanvas,
			s.config.Fit, s.config.CaptionInsets, s.config.CaptionsWithinActive,
		)
		if err != nil {
			return SessionSnapshot{}, fmt.Errorf("layout program source %q: %w", id, err)
		}
		snapshot.Sources[id] = SourceView{
			Source:  entry.source,
			State:   entry.state,
			Preview: preview,
			Program: program,
		}
	}
	return snapshot, nil
}

func scaleInsets(insets Insets, from, to Size) Insets {
	sx := float64(to.W) / float64(from.W)
	sy := float64(to.H) / float64(from.H)
	return Insets{
		Left:   insets.Left * sx,
		Top:    insets.Top * sy,
		Right:  insets.Right * sx,
		Bottom: insets.Bottom * sy,
	}
}
