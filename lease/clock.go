package lease

import (
	"sync"
	"time"
)

// Clock abstracts the source of time so tests can control expiry
// without sleeping.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// ManualClock is a Clock driven by the caller. It never advances on its
// own; Advance moves it forward. It is safe for concurrent use.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewManualClock returns a ManualClock set to start.
func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

// Now returns the current manual time.
func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
