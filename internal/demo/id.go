package demo

import "fmt"

// idCounter produces deterministic 32-char hex-like ids so snapshot
// filenames stay stable in demo output and tests.
type idCounter struct {
	n int
}

func newIDCounter() *idCounter { return &idCounter{} }

func (c *idCounter) gen() string {
	c.n++
	return fmt.Sprintf("%032d", c.n)
}
