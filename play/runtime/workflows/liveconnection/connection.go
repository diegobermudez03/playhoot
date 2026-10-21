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
	// displayName is how the other players see a player connection's user.
	displayName string
	// participantID is set when the game starts, for a player's connection.
	participantID string
	// watching is the participant an admin's connection selected to watch.
	watching string

	lobbyUpdates  latestFeed[runtime.LobbyUpdate]
	rosterUpdates latestFeed[runtime.RosterUpdate]
	views         viewFeed
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
	ending runtime.ConnectionEnd
}

func newConnection(id string, player runtime.Player) *connection {
	return &connection{
		id: id, userUUID: player.UserUUID, displayName: player.DisplayName,
		lobbyUpdates: newLatestFeed[runtime.LobbyUpdate](), rosterUpdates: newLatestFeed[runtime.RosterUpdate](), views: newViewFeed(),
		started: make(chan struct{}), ended: make(chan struct{}),
	}
}

func (c *connection) end(reason runtime.EndReason) {
	c.endWith(runtime.ConnectionEnd{Reason: reason})
}

// endWith ends the connection as ending; only the first end counts.
func (c *connection) endWith(ending runtime.ConnectionEnd) {
	c.once.Do(func() {
		c.ending = ending
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
		return c.ending, true
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

// nextLobbyUpdate returns the latest lobby update not yet returned; one
// published before the end is still returned before false.
func (c *connection) nextLobbyUpdate(ctx context.Context) (runtime.LobbyUpdate, bool) {
	return c.lobbyUpdates.next(ctx, c.ended)
}

// nextRosterUpdate returns the latest roster update not yet returned; one
// published before the end is still returned before false.
func (c *connection) nextRosterUpdate(ctx context.Context) (runtime.RosterUpdate, bool) {
	return c.rosterUpdates.next(ctx, c.ended)
}

// latestFeed holds the latest update a connection has not taken yet. Each
// update is whole (the whole lobby, the whole roster), so a newer one replaces
// one not yet taken. Once finished it reports no more updates after the
// pending one.
type latestFeed[T any] struct {
	mu       sync.Mutex
	pending  *T
	finished bool
	signal   chan struct{}
}

func newLatestFeed[T any]() latestFeed[T] {
	return latestFeed[T]{signal: make(chan struct{}, 1)}
}

func (f *latestFeed[T]) publish(update T) {
	f.mu.Lock()
	if f.finished {
		f.mu.Unlock()
		return
	}
	f.pending = &update
	f.mu.Unlock()
	f.wake()
}

// finish ends the feed: nothing more is published to this connection.
func (f *latestFeed[T]) finish() {
	f.mu.Lock()
	f.finished = true
	f.mu.Unlock()
	f.wake()
}

func (f *latestFeed[T]) wake() {
	select {
	case f.signal <- struct{}{}:
	default:
	}
}

// take returns the pending update; more is false once the feed finished
// with nothing pending.
func (f *latestFeed[T]) take() (update T, ok bool, more bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pending == nil {
		return update, false, !f.finished
	}
	update = *f.pending
	f.pending = nil
	return update, true, true
}

func (f *latestFeed[T]) next(ctx context.Context, ended <-chan struct{}) (T, bool) {
	for {
		update, ok, more := f.take()
		if ok || !more {
			return update, ok
		}
		select {
		case <-f.signal:
		case <-ended:
			update, ok, _ := f.take()
			return update, ok
		case <-ctx.Done():
			var none T
			return none, false
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

// publish adds view, whose whole state is whole. A connection that left
// maxPending views untaken is behind: they and view become one FULL view of
// whole, keeping the errors and events they carried for the same participant,
// in order. collapsed reports that it happened.
func (f *viewFeed) publish(view runtime.OwnView, whole []byte, maxPending int) (collapsed bool) {
	f.mu.Lock()
	if maxPending > 0 && len(f.pending) >= maxPending && whole != nil {
		merged := view
		merged.Full, merged.State = true, whole
		merged.Errors, merged.Events = nil, nil
		for _, behind := range append(f.pending, view) {
			if behind.ParticipantID == view.ParticipantID {
				merged.Errors = append(merged.Errors, behind.Errors...)
				merged.Events = append(merged.Events, behind.Events...)
			}
		}
		f.pending = []runtime.OwnView{merged}
		collapsed = true
	} else {
		f.pending = append(f.pending, view)
	}
	f.mu.Unlock()
	select {
	case f.signal <- struct{}{}:
	default:
	}
	return collapsed
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
