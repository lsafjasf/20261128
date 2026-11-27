package subburn

import (
	"fmt"
	"time"
)

type Cue struct {
	ID    string
	Start time.Duration
	End   time.Duration
	Text  string
}

func (c Cue) Validate() error {
	if c.Start < 0 || c.End < 0 {
		return fmt.Errorf("cue %q has negative time", c.ID)
	}
	if c.End <= c.Start {
		return fmt.Errorf("cue %q has empty or reversed range [%s, %s)", c.ID, c.Start, c.End)
	}
	return nil
}
