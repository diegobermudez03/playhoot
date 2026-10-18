// Package controller holds the live state of the Sessions this instance owns:
// one Controller per Session, and the registry that finds or creates them.
package controller

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// ErrNotImplemented is returned by connection operations not built yet.
var ErrNotImplemented = errors.New("live operation not implemented yet")

// Manifest is what a Controller knows about its Session's game version.
type Manifest struct {
	MinPlayers int
	MaxPlayers int
}

// phase is where a Controller's Session is in its live lifecycle; it decides
// what admitting, leaving and disconnecting mean.
type phase string

const (
	// phaseLobby is waiting for the game to start; players come and go
	// freely and every change reaches all connections as a lobby update.
	phaseLobby phase = "lobby"

	// phaseStarting is an attempt to start the game: the lobby is frozen,
	// so nobody joins and nobody who disconnects leaves it until the attempt
	// ends.
	phaseStarting phase = "starting"

	// phaseRunning is a started game: its participants are fixed and the
	// lobby is closed. Start moves a Controller here.
	phaseRunning phase = "running"

	// phaseEnded is a Session that reached a terminal state.
	phaseEnded phase = "ended"
)

// ManifestLoader reads the manifest of the Session a new Controller is for.
type ManifestLoader func(ctx context.Context) (Manifest, error)

// Registry finds or creates the Controller of each Session this instance
// owns.
type Registry struct {
	mu          sync.Mutex
	controllers map[string]*Controller
}

// Controller is the live state of one Session on its owner instance.
type Controller struct {
	sessionUUID string
	epoch       int64
	manifest    Manifest

	mu     sync.Mutex
	phase  phase
	admins map[string]*connection
	// players holds each user's current player connection, and lobby the
	// users in the lobby in the order they joined.
	players map[string]*connection
	lobby   []string

	// participants are the lobby frozen by the current Start attempt, kept
	// once the game runs.
	participants []runtime.Participant
	// game is the confirmed state of the running game.
	game Game
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{controllers: make(map[string]*Controller)}
}

// ControllerFor returns sessionUUID's Controller under epoch, creating it with
// load's manifest when there is none. A Controller from an older epoch is
// retired and replaced. ok is false when epoch is older than the existing one.
func (r *Registry) ControllerFor(ctx context.Context, sessionUUID string, epoch int64, load ManifestLoader) (controller *Controller, ok bool, err error) {
	r.mu.Lock()
	controller, decided, ok := r.lookupLocked(sessionUUID, epoch)
	r.mu.Unlock()
	if decided {
		return controller, ok, nil
	}

	// Loaded without holding the registry, so other Sessions never wait for it.
	manifest, err := load(ctx)
	if err != nil {
		return nil, false, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Another request may have created it meanwhile.
	if controller, decided, ok := r.lookupLocked(sessionUUID, epoch); decided {
		return controller, ok, nil
	}
	if stale, found := r.controllers[sessionUUID]; found {
		stale.retire()
		controllersRetired.Inc(ctx)
	}
	controller = &Controller{
		sessionUUID: sessionUUID,
		epoch:       epoch,
		manifest:    manifest,
		phase:       phaseLobby,
		admins:      make(map[string]*connection),
		players:     make(map[string]*connection),
	}
	r.controllers[sessionUUID] = controller
	controllersCreated.Inc(ctx)
	return controller, true, nil
}

// lookupLocked decides from the existing Controller alone: decided is false
// when there is none or it is stale, so one must be created.
func (r *Registry) lookupLocked(sessionUUID string, epoch int64) (controller *Controller, decided bool, ok bool) {
	existing, found := r.controllers[sessionUUID]
	switch {
	case !found || epoch > existing.epoch:
		return nil, false, false
	case epoch == existing.epoch:
		return existing, true, true
	default:
		return nil, true, false
	}
}

// retire ends every connection of a Controller that no longer serves its
// Session under the current ownership.
func (c *Controller) retire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, admin := range c.admins {
		admin.end(runtime.EndOwnerChanged)
		delete(c.admins, id)
	}
	for userUUID, player := range c.players {
		player.end(runtime.EndOwnerChanged)
		delete(c.players, userUUID)
	}
	c.lobby = nil
}

// lobbyLocked is the lobby as its connections see it.
func (c *Controller) lobbyLocked() runtime.LobbyUpdate {
	members := make([]runtime.LobbyMember, len(c.lobby))
	for i, userUUID := range c.lobby {
		members[i] = runtime.LobbyMember{UserUUID: userUUID}
	}
	return runtime.LobbyUpdate{Members: members}
}

// broadcastLobbyLocked tells every connection the lobby changed.
func (c *Controller) broadcastLobbyLocked() {
	update := c.lobbyLocked()
	for _, admin := range c.admins {
		admin.lobbyUpdates.publish(update)
	}
	for _, player := range c.players {
		player.lobbyUpdates.publish(update)
	}
}

// leaveLobbyLocked takes player out of the lobby, unless a newer connection of
// the same user already replaced it. Only valid in phaseLobby.
func (c *Controller) leaveLobbyLocked(player *connection) {
	if c.players[player.userUUID] != player {
		return
	}
	delete(c.players, player.userUUID)
	c.lobby = slices.DeleteFunc(c.lobby, func(userUUID string) bool { return userUUID == player.userUUID })
	c.broadcastLobbyLocked()
}

// connection is one attached admin or player connection.
type connection struct {
	id       string
	userUUID string
	// participantID is set when the game starts, for a player's connection.
	participantID string

	lobbyUpdates lobbyFeed
	views        viewFeed
	// started is closed when the game starts while the connection is open.
	started chan struct{}

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
