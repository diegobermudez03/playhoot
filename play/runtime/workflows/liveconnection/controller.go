package liveconnection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/internal/business/commands"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=controller_mock_test.go . manifestLoader

// manifestLoader reads the manifest of the game version a Session is pinned
// to, which its controller keeps for the Session's lifetime.
type manifestLoader interface {
	LoadRuntimeManifest(ctx context.Context, sessionUUID string) (sessionpkg.RuntimeManifest, error)
}

// ErrNotImplemented is returned by connection operations not built yet.
var ErrNotImplemented = errors.New("live operation not implemented yet")

// errStopped means the controller stopped before it could run an operation.
var errStopped = errors.New("session controller stopped")

// Manifest is what a controller knows about its Session's game version.
type Manifest struct {
	MaxPlayers int

	// Inputs are what the game's SAMPLEs carry; empty means it takes none.
	Inputs inputs.Schema

	// Commands are the COMMANDs the game accepts and their payloads.
	Commands commands.Catalog

	// Update means the game implements update(), run every boundary.
	Update bool

	// Rejoin means the game implements playerRejoined(), so a disconnected
	// participant may ask to come back.
	Rejoin bool
}

// Game is the confirmed state of a running game: everything needed to
// restore it in a new execution context.
type Game struct {
	GameVersionUUID string

	// Script is the game version's JavaScript source.
	Script []byte

	ContextRevision int64

	// ExecutedThroughSessionTimeMs is the effective Session time the state
	// was materialized through.
	ExecutedThroughSessionTimeMs int64

	// State is the authoritative game state, as JSON.
	State []byte

	// PRNGState is the opaque state of the script's randomness.
	PRNGState []byte
}

// phase is where a controller's Session is in its live lifecycle; it decides
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
	// lobby is closed.
	phaseRunning phase = "running"

	// phaseEnded is a Session that reached a terminal state, or that another
	// instance took over.
	phaseEnded phase = "ended"
)

// controllerDependencies are what every controller runs its Session with.
type controllerDependencies struct {
	adminJoins  adminJoinChecker
	playerJoins playerJoinChecker
	manifests   manifestLoader

	starts    sessionStarter
	aborts    sessionAborter
	cancels   sessionCanceller
	frontends frontendSigner
	sandbox   gameSandbox
	replay    replayRecorder

	// now is the clock Session time is measured with.
	now func() time.Time
	// seeds returns the seed of a starting game's randomness.
	seeds func() uint64
	// tickInterval is a running game's boundary cadence.
	tickInterval time.Duration
	// transportGrace is how long a participant who lost their transport stays
	// connected before the game is told they disconnected.
	transportGrace time.Duration
	logger         *slog.Logger
}

// sessionController is one live Session on its owner instance. Connections
// call it directly through the operations it returns them. Quick operations
// run on the caller's goroutine; everything that touches the sandbox or the
// database runs on its main goroutine, the Session's single mutation lane.
type sessionController struct {
	sessionUUID string
	epoch       int64
	deps        *controllerDependencies
	// root is done when the instance shuts down.
	root context.Context

	// mu guards state; only lock reaches it.
	mu    sync.Mutex
	state liveState

	// manifestMu serializes reading the manifest, done once by the first
	// admission that needs it, so the state lock never waits for it.
	manifestMu     sync.Mutex
	manifestLoaded bool

	// contextID is the sandbox context running the game, infraFailures how
	// many boundaries in a row failed to execute, and lastUpdate when update()
	// last committed. Only the main goroutine uses them.
	contextID     string
	infraFailures int
	lastUpdate    time.Time

	// operations are run by the main goroutine, in order.
	operations chan func()
	// flush asks the main goroutine for a boundary before the next tick.
	flush chan struct{}
	// ended is closed once the Session ended here, and exited once the main
	// goroutine returned.
	ended   chan struct{}
	endOnce sync.Once
	exited  chan struct{}
}

func newSessionController(root context.Context, sessionUUID string, epoch int64, deps *controllerDependencies) *sessionController {
	return &sessionController{
		sessionUUID: sessionUUID,
		epoch:       epoch,
		deps:        deps,
		root:        root,
		state: liveState{
			now:      deps.now,
			phase:    phaseLobby,
			admins:   make(map[string]*connection),
			players:  make(map[string]*connection),
			inputs:   make(map[string]*participantInputs),
			commands: make(map[string]*participantCommands),
			presence: make(map[string]*participantPresence),
			rejoins:  make(map[string]*pendingRejoin),
			views:    make(map[string][]byte),
		},
		operations: make(chan func()),
		flush:      make(chan struct{}, 1),
		ended:      make(chan struct{}),
		exited:     make(chan struct{}),
	}
}

// loadManifest reads the Session's manifest the first time an admitted
// connection needs it; a failed read is tried again by the next one.
func (c *sessionController) loadManifest(ctx context.Context) error {
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	if c.manifestLoaded {
		return nil
	}
	loaded, err := c.deps.manifests.LoadRuntimeManifest(ctx, c.sessionUUID)
	if err != nil {
		return fmt.Errorf("loading runtime manifest: %w", err)
	}
	declarations := inputDeclarations(loaded.Inputs)
	schema, err := inputs.NewSchema(declarations)
	if err != nil {
		return fmt.Errorf("reading the manifest's inputs: %w", err)
	}
	declaredCommands := make([]commands.Declaration, len(loaded.Commands))
	for i, command := range loaded.Commands {
		declaredCommands[i] = commands.Declaration{Name: command.Name, Payload: inputDeclarations(command.Payload)}
	}
	catalog, err := commands.NewCatalog(declaredCommands)
	if err != nil {
		return fmt.Errorf("reading the manifest's commands: %w", err)
	}
	s := c.lock()
	s.manifest = Manifest{MaxPlayers: loaded.MaxPlayers, Inputs: schema, Commands: catalog, Update: loaded.Update, Rejoin: loaded.Rejoin}
	c.unlock()
	c.manifestLoaded = true
	return nil
}

func inputDeclarations(declared []sessionpkg.RuntimeInput) []inputs.Declaration {
	declarations := make([]inputs.Declaration, len(declared))
	for i, input := range declared {
		declarations[i] = inputs.Declaration{
			Name: input.Name, Type: inputs.Type(input.Type), Nullable: input.Nullable,
			Min: input.Min, Max: input.Max, Values: input.Values, Default: input.Default,
		}
	}
	return declarations
}

// lock takes the controller's state until unlock.
func (c *sessionController) lock() *liveState {
	c.mu.Lock()
	return &c.state
}

func (c *sessionController) unlock() {
	c.mu.Unlock()
}

// run is the controller's main goroutine, from its creation until its
// Session ends here or the instance shuts down. Once the game runs, it also
// runs a boundary every tick.
func (c *sessionController) run() {
	defer close(c.exited)
	var ticker *time.Ticker
	defer func() {
		if ticker != nil {
			ticker.Stop()
			lanesActive.Dec(c.root)
		}
	}()
	var ticks <-chan time.Time
	for {
		select {
		case <-c.root.Done():
			// The Session is left for its next owner, context included.
			return
		case <-c.ended:
			c.release()
			return
		case operation := <-c.operations:
			operation()
		case <-ticks:
			c.runBoundary()
		case <-c.flush:
			c.runBoundary()
		}
		if ticker == nil && c.isRunning() {
			ticker = time.NewTicker(c.deps.tickInterval)
			ticks = ticker.C
			lanesActive.Inc(c.root)
		}
	}
}

// do runs operation on the main goroutine and waits for it; false means the
// controller stopped first, or ctx was done before it began.
func (c *sessionController) do(ctx context.Context, operation func()) bool {
	finished := make(chan struct{})
	select {
	case c.operations <- func() {
		defer close(finished)
		operation()
	}:
	case <-c.exited:
		return false
	case <-ctx.Done():
		return false
	}
	// Once begun it always finishes, so its result is never lost.
	<-finished
	return true
}

// requestFlush asks for a boundary now; requests before it runs coalesce.
func (c *sessionController) requestFlush() {
	select {
	case c.flush <- struct{}{}:
	default:
	}
}

// release frees what the ended Session held in the sandbox.
func (c *sessionController) release() {
	if c.contextID == "" {
		return
	}
	ctx := logging.Start(context.WithoutCancel(c.root))
	c.discardContext(ctx)
	logging.LogFields(ctx, logging.Field("session_uuid", c.sessionUUID))
	logging.FinishRequestLog(ctx, c.deps.logger, "session controller released")
}

func (c *sessionController) isRunning() bool {
	s := c.lock()
	defer c.unlock()
	return s.phase == phaseRunning
}

// end moves the Session to its end with every connection ending for reason.
// The caller holds the lock.
func (c *sessionController) end(s *liveState, reason runtime.EndReason) {
	s.phase = phaseEnded
	s.lobby = nil
	s.timeline = nil
	for _, tokens := range s.commands {
		tokens.abandonPending()
	}
	for participantID, request := range s.rejoins {
		request.resolve(rejoinNotRunning)
		delete(s.rejoins, participantID)
	}
	for id, admin := range s.admins {
		admin.end(reason)
		delete(s.admins, id)
	}
	for userUUID, player := range s.players {
		player.end(reason)
		delete(s.players, userUUID)
	}
	c.endOnce.Do(func() { close(c.ended) })
}

// retire ends a controller that no longer serves its Session under the
// current ownership.
func (c *sessionController) retire() {
	s := c.lock()
	defer c.unlock()
	c.end(s, runtime.EndOwnerChanged)
}

// AdminCount returns how many admin connections are attached.
func (c *sessionController) AdminCount() int {
	s := c.lock()
	defer c.unlock()
	return len(s.admins)
}

// PlayerCount returns how many players are in the lobby.
func (c *sessionController) PlayerCount() int {
	s := c.lock()
	defer c.unlock()
	return len(s.lobby)
}

// liveState is a controller's state shared between its goroutines and its
// connections' calls. Its methods expect the controller's lock to be held.
type liveState struct {
	manifest Manifest
	now      func() time.Time

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
	// start is what every connection is told when the game starts.
	start runtime.SessionStart

	// runningSince and baseSessionTimeMs give the effective Session time: it
	// continues from the confirmed state's time when the game began running
	// here.
	runningSince      time.Time
	baseSessionTimeMs int64
	// inputs is each participant's SAMPLE ingress, by ParticipantID.
	inputs map[string]*participantInputs
	// commands is each participant's COMMAND tokens, by ParticipantID.
	commands map[string]*participantCommands
	// presence is whether each participant is still playing, by
	// ParticipantID.
	presence map[string]*participantPresence
	// rejoins are the disconnected participants' requests to come back
	// waiting for the game, by ParticipantID.
	rejoins map[string]*pendingRejoin
	// views is each participant's current projection, by ParticipantID, as
	// the last committed state gave it.
	views map[string][]byte
	// timeline holds the operations accepted and not yet executed, in order.
	timeline  []Operation
	nextOrder uint64
}

// Operation is one entry of the Session's single timeline, at the effective
// Session time it was accepted; Order breaks ties between operations accepted
// at the same time. It is a participant's new input state, their COMMAND when
// Command is set, their disconnection when Disconnected is, or their request
// to come back when Rejoin is.
type Operation struct {
	ParticipantID string
	SessionTimeMs int64
	Order         uint64

	// Inputs is the full input snapshot of an input transition.
	Inputs inputs.Snapshot

	Command      *pendingCommand
	Disconnected bool
	Rejoin       *pendingRejoin
}

// runsAlone reports whether the operation is its own call, at its own time,
// rather than a transition sharing a samples() call.
func (o Operation) runsAlone() bool {
	return o.Command != nil || o.Disconnected || o.Rejoin != nil
}

// enqueue appends operation to the timeline at the current Session time.
func (s *liveState) enqueue(operation Operation) {
	s.nextOrder++
	operation.SessionTimeMs = s.sessionTime()
	operation.Order = s.nextOrder
	s.timeline = append(s.timeline, operation)
}

// lobbyUpdate is the lobby as its connections see it.
func (s *liveState) lobbyUpdate() runtime.LobbyUpdate {
	members := make([]runtime.LobbyMember, len(s.lobby))
	for i, userUUID := range s.lobby {
		members[i] = runtime.LobbyMember{UserUUID: userUUID}
	}
	return runtime.LobbyUpdate{Members: members}
}

// broadcastLobby tells every connection the lobby changed.
func (s *liveState) broadcastLobby() {
	update := s.lobbyUpdate()
	for _, admin := range s.admins {
		admin.lobbyUpdates.publish(update)
	}
	for _, player := range s.players {
		player.lobbyUpdates.publish(update)
	}
}

// leaveLobby takes player out of the lobby, unless a newer connection of the
// same user already replaced it. Only valid in phaseLobby.
func (s *liveState) leaveLobby(player *connection) {
	if s.players[player.userUUID] != player {
		return
	}
	delete(s.players, player.userUUID)
	s.lobby = slices.DeleteFunc(s.lobby, func(userUUID string) bool { return userUUID == player.userUUID })
	s.broadcastLobby()
}
