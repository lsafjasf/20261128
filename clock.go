package st2110

import (
	"sync"
	"time"
)

// Clock abstracts a PTP-disciplined clock (ST 2059). Now must only be
// trusted while Locked reports true; there is deliberately no unlocked
// fallback in this interface.
type Clock interface {
	Locked() bool
	// Now returns the disciplined time and ok=true only while locked.
	Now() (t time.Time, ok bool)
}

// PTPClock is a Clock whose lock state is driven externally (e.g. from
// ptp4l/port status). The underlying source is never exposed while unlocked,
// so callers cannot silently drift onto a local clock.
type PTPClock struct {
	mu     sync.RWMutex
	locked bool
	source func() time.Time
}

func NewPTPClock(source func() time.Time) *PTPClock {
	if source == nil {
		source = time.Now
	}
	return &PTPClock{source: source}
}

func (c *PTPClock) SetLocked(locked bool) {
	c.mu.Lock()
	c.locked = locked
	c.mu.Unlock()
}

func (c *PTPClock) Locked() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.locked
}

func (c *PTPClock) Now() (time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.locked {
		return time.Time{}, false
	}
	return c.source(), true
}
