package live

import (
	"context"
	"sync"
)

// Controller runs live connections and ends them on Shutdown.
type Controller struct {
	mu       sync.Mutex
	stopping bool
	running  sync.WaitGroup

	shutdown       context.Context
	signalShutdown context.CancelFunc
}

// New constructs a Controller.
func New() *Controller {
	shutdown, signalShutdown := context.WithCancel(context.Background())
	return &Controller{shutdown: shutdown, signalShutdown: signalShutdown}
}

// Go runs serve in its own goroutine. serve's shutdown context is cancelled
// when Shutdown starts, and it must then end its connection promptly. Go
// returns false, without running serve, once Shutdown has started.
func (c *Controller) Go(serve func(shutdown context.Context)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping {
		return false
	}
	c.running.Add(1)
	go func() {
		defer c.running.Done()
		serve(c.shutdown)
	}()
	return true
}

// Shutdown tells every running connection to end and waits for them until ctx
// is done, returning ctx's error if some did not end in time.
func (c *Controller) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	c.stopping = true
	c.mu.Unlock()
	c.signalShutdown()

	ended := make(chan struct{})
	go func() {
		c.running.Wait()
		close(ended)
	}()
	select {
	case <-ended:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
