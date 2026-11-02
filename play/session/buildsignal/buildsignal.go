// Package buildsignal wakes this instance's build worker when a build is
// queued, so it does not wait for its next periodic check.
package buildsignal

// Signal is a wake-up that never blocks its sender: wake-ups sent while the
// worker is busy collapse into one.
type Signal struct {
	c chan struct{}
}

// New returns a Signal.
func New() *Signal {
	return &Signal{c: make(chan struct{}, 1)}
}

// Notify wakes the worker. A nil Signal does nothing.
func (s *Signal) Notify() {
	if s == nil {
		return
	}
	select {
	case s.c <- struct{}{}:
	default:
	}
}

// C receives one value after one or more Notify calls.
func (s *Signal) C() <-chan struct{} {
	return s.c
}
