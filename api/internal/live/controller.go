package live

import (
	"context"
	"sync"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/logging"
)

// Controller runs the live connections of this instance and ends them on
// Shutdown.
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

// Serve runs conn's loops in the background until one ends the connection or
// the server shuts down, then calls disconnect. ctx must outlive the request.
// Once Shutdown started it refuses instead: it closes conn as going away,
// calls disconnect, and returns false.
func (c *Controller) Serve(ctx context.Context, conn *websocket.Conn, disconnect func(), loops ...Loop) bool {
	// Counted before starting, while ctx's request log is still open, so the
	// connection's own end is never recorded first.
	connectionsActive.Inc(ctx)
	started := c.start(func(shutdown context.Context) {
		serve(ctx, shutdown, conn, disconnect, loops...)
	})
	if !started {
		connectionsActive.Dec(ctx)
		_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
		disconnect()
		logging.LogFields(ctx, logging.Field("connection", "refused_shutting_down"))
		return false
	}
	logging.LogFields(ctx, logging.Field("connection", "opened"))
	return true
}

func (c *Controller) start(run func(shutdown context.Context)) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopping {
		return false
	}
	c.running.Add(1)
	go func() {
		defer c.running.Done()
		run(c.shutdown)
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
