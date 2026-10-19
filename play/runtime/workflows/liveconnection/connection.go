package liveconnection

import (
	"context"
	"sync"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// connection is one attached admin or player connection.
type connection struct {
	id       string
	userUUID string
	// participantID is set when the game starts, for a player's connection.
	participantID string

	lobbyUpdates lobbyFeed
	views        viewFeed
	// lastView, lastAck and revision are the view, acknowledgement and view
	// revision this connection last received; the next view is a patch
	// against them.
	lastView []byte
	lastAck  int64
	revision int64
	// started is closed when the game starts while the connection is open,
	// after start is set.
	started chan struct{}
	start   runtime.SessionStart

	once   sync.Once
	ended  chan struct{}
	reason runtime.EndReason
}

func newConnection(id string, userUUID string) *connection {
	return &connection{id: id, userUUID: userUUID, lobbyUpdates: newLobbyFeed(), views: newViewFeed(), started: make(chan struct{}), ended: make(chan struct{})}
}

func (c *connection) end(reason runtime.EndReason) {
	c.once.Do(func() {
		c.reason = reason
		close(c.ended)
	})
}

func (c *connection) isEnded() bool {
	select {
	case <-c.ended:
		return true
	default:
		return false
	}
}

func (c *connection) wait(ctx context.Context) (runtime.ConnectionEnd, bool) {
	select {
	case <-c.ended:
		return runtime.ConnectionEnd{Reason: c.reason}, true
	case <-ctx.Done():
		return runtime.ConnectionEnd{}, false
	}
}

// sessionStart blocks until the game starts for the connection and returns
// who plays; false means it ended, or ctx was done, first.
func (c *connection) sessionStart(ctx context.Context) (runtime.SessionStart, bool) {
	select {
	case <-c.started:
	case <-c.ended:
		select {
		case <-c.started:
		default:
			return runtime.SessionStart{}, false
		}
	case <-ctx.Done():
		return runtime.SessionStart{}, false
	}
	// Written before started was closed and never again.
	return c.start, true
}

// markStarted tells the connection the game started with start. The caller
// holds the controller's lock.
func (c *connection) markStarted(start runtime.SessionStart) {
	c.start = start
	c.lobbyUpdates.finish()
	close(c.started)
}

// waitWithoutResult blocks until the connection ends or ctx is done; it backs
// the streams that never produce anything yet.
func (c *connection) waitWithoutResult(ctx context.Context) {
	select {
	case <-c.ended:
	case <-ctx.Done():
	}
}

// nextLobbyUpdate returns the latest lobby update not yet returned; one
// published before the end is still returned before false.
func (c *connection) nextLobbyUpdate(ctx context.Context) (runtime.LobbyUpdate, bool) {
	return c.lobbyUpdates.next(ctx, c.ended)
}

// lobbyFeed holds the latest lobby update a connection has not taken yet.
// Each update is the whole lobby, so a newer one replaces one not yet taken.
// Once finished it reports no more updates after the pending one.
type lobbyFeed struct {
	mu       sync.Mutex
	pending  *runtime.LobbyUpdate
	finished bool
	signal   chan struct{}
}

func newLobbyFeed() lobbyFeed {
	return lobbyFeed{signal: make(chan struct{}, 1)}
}

func (f *lobbyFeed) publish(update runtime.LobbyUpdate) {
	f.mu.Lock()
	if f.finished {
		f.mu.Unlock()
		return
	}
	f.pending = &update
	f.mu.Unlock()
	f.wake()
}

// finish ends the feed: the lobby is over for this connection.
func (f *lobbyFeed) finish() {
	f.mu.Lock()
	f.finished = true
	f.mu.Unlock()
	f.wake()
}

func (f *lobbyFeed) wake() {
	select {
	case f.signal <- struct{}{}:
	default:
	}
}

// take returns the pending update; more is false once the feed finished
// with nothing pending.
func (f *lobbyFeed) take() (update runtime.LobbyUpdate, ok bool, more bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pending == nil {
		return runtime.LobbyUpdate{}, false, !f.finished
	}
	update = *f.pending
	f.pending = nil
	return update, true, true
}

func (f *lobbyFeed) next(ctx context.Context, ended <-chan struct{}) (runtime.LobbyUpdate, bool) {
	for {
		update, ok, more := f.take()
		if ok {
			return update, true
		}
		if !more {
			return runtime.LobbyUpdate{}, false
		}
		select {
		case <-f.signal:
		case <-ended:
			update, ok, _ := f.take()
			return update, ok
		case <-ctx.Done():
			return runtime.LobbyUpdate{}, false
		}
	}
}

// viewFeed holds a connection's views in the order they were produced, until
// taken.
type viewFeed struct {
	mu      sync.Mutex
	pending []runtime.OwnView
	signal  chan struct{}
}

func newViewFeed() viewFeed {
	return viewFeed{signal: make(chan struct{}, 1)}
}

func (f *viewFeed) publish(view runtime.OwnView) {
	f.mu.Lock()
	f.pending = append(f.pending, view)
	f.mu.Unlock()
	select {
	case f.signal <- struct{}{}:
	default:
	}
}

func (f *viewFeed) take() (runtime.OwnView, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pending) == 0 {
		return runtime.OwnView{}, false
	}
	view := f.pending[0]
	f.pending = f.pending[1:]
	return view, true
}

// next returns the oldest view not yet taken; every view published before the
// end is still returned before false.
func (f *viewFeed) next(ctx context.Context, ended <-chan struct{}) (runtime.OwnView, bool) {
	for {
		if view, ok := f.take(); ok {
			return view, true
		}
		select {
		case <-f.signal:
		case <-ended:
			return f.take()
		case <-ctx.Done():
			return runtime.OwnView{}, false
		}
	}
}
