package live

import (
	"context"
	"errors"
	"sync"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/logging"
)

// DefaultMaxConnections bounds an instance's live connections when not
// configured.
const DefaultMaxConnections = 2000

// MaxConnectionsPerCaller bounds one caller's live connections on an
// instance: a few tabs and devices, never an unbounded number.
const MaxConnectionsPerCaller = 10

var (
	// ErrInstanceFull means this instance serves as many live connections as
	// it may.
	ErrInstanceFull = errors.New("instance serves as many live connections as it may")
	// ErrCallerFull means the caller has as many live connections as it may.
	ErrCallerFull = errors.New("caller has as many live connections as it may")
)

// Controller runs the live connections of this instance, bounds how many
// there are, and ends them on Shutdown.
type Controller struct {
	mu       sync.Mutex
	stopping bool
	running  sync.WaitGroup
	// reserved counts the connections reserved, in total and by caller.
	maxConnections int
	reserved       int
	reservedBy     map[string]int

	shutdown       context.Context
	signalShutdown context.CancelFunc
}

// New constructs a Controller serving at most maxConnections live
// connections; 0 takes DefaultMaxConnections.
func New(maxConnections int) *Controller {
	if maxConnections <= 0 {
		maxConnections = DefaultMaxConnections
	}
	shutdown, signalShutdown := context.WithCancel(context.Background())
	return &Controller{maxConnections: maxConnections, reservedBy: make(map[string]int), shutdown: shutdown, signalShutdown: signalShutdown}
}

// Reserve holds room for one more live connection of caller, to take before
// upgrading it; release gives it back and may be called more than once. It
// fails with ErrInstanceFull or ErrCallerFull when there is no room.
func (c *Controller) Reserve(ctx context.Context, caller string) (release func(), err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.reserved >= c.maxConnections:
		connectionsRefused.Inc(ctx, refusedLabel.Value(refusedInstanceFull))
		return nil, ErrInstanceFull
	case c.reservedBy[caller] >= MaxConnectionsPerCaller:
		connectionsRefused.Inc(ctx, refusedLabel.Value(refusedCallerFull))
		return nil, ErrCallerFull
	}
	c.reserved++
	c.reservedBy[caller]++
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.reserved--
			if c.reservedBy[caller]--; c.reservedBy[caller] == 0 {
				delete(c.reservedBy, caller)
			}
		})
	}, nil
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
