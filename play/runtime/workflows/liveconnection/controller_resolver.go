// Package liveconnection is the live Session: one controller per Session this
// instance owns, which holds its state, runs its game and serves its
// connections. The ControllerResolver only finds or creates a Session's
// controller when a connection arrives.
package liveconnection

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// defaultTickInterval is how often a running game's accepted input is
// executed, a platform tuning value games must not rely on.
const defaultTickInterval = 100 * time.Millisecond

// defaultTransportGrace is how long a player who lost their transport may
// reattach before the game is told they disconnected; platform time.
const defaultTransportGrace = 3 * time.Second

// defaultRecoveryWindow is how long, after a running Session is recovered,
// the participants who were playing have to reconnect; platform time.
const defaultRecoveryWindow = 30 * time.Second

// defaultCheckpointInterval is the least time between two periodic recovery
// checkpoints of a running game; what a crash can roll back.
const defaultCheckpointInterval = 5 * time.Second

// defaultLimitsInterval is how often a controller checks its Session's
// deadlines.
const defaultLimitsInterval = time.Second

// defaultContextIdleTimeout is how long the sandbox context of a running game
// without update() stays alive unused before it is evicted; the next
// execution restores it from the mirror.
const defaultContextIdleTimeout = 30 * time.Second

// defaultInactivityTimeout is how long a running game may go without a
// distinct input transition or a COMMAND before it expires; platform time.
const defaultInactivityTimeout = 10 * time.Minute

// DefaultMaxLiveSessions bounds the live Sessions an instance runs when not
// configured; a Session in its lobby holds no sandbox context, so the
// sandbox's own limits do not bound it.
const DefaultMaxLiveSessions = 500

// liveLimits bound what players and the platform may ask of one live
// Session; the platform chooses them, never the game.
type liveLimits struct {
	// sampleTransitions bound one participant's distinct input transitions.
	sampleTransitions []ratelimit.Limit
	// participantCommands and sessionCommands bound COMMANDs per participant
	// and for the whole Session; pendingCommands is how many of one
	// participant's may wait at once.
	participantCommands []ratelimit.Limit
	sessionCommands     []ratelimit.Limit
	pendingCommands     int
	// processedTokens is how many executed COMMAND tokens a participant
	// keeps, the oldest forgotten first: a retry comes within seconds, and
	// every one kept is written in each checkpoint.
	processedTokens int
	// pendingPerParticipant and pendingPerSession bound the players'
	// operations waiting in the timeline; the platform's own never wait on
	// them.
	pendingPerParticipant int
	pendingPerSession     int
	// boundaryOperations is the most timeline operations one boundary
	// executes; the rest wait for the next.
	boundaryOperations int
	// boundaries bound how often a Session runs a boundary; a boundary asked
	// for beyond them waits for the next tick, which always runs.
	boundaries []ratelimit.Limit
	// admins is how many admin connections a Session may have.
	admins int
	// pendingViews is how many views a connection may leave untaken before
	// they collapse into one FULL view.
	pendingViews int
}

// defaultLiveLimits are the platform's limits on every live Session. Ticks
// alone run 10 boundaries a second, which the boundary limits leave room
// above for the boundaries COMMANDs ask for.
func defaultLiveLimits() liveLimits {
	return liveLimits{
		sampleTransitions:     []ratelimit.Limit{{Count: 120, Per: time.Second}, {Count: 3600, Per: time.Minute}},
		participantCommands:   []ratelimit.Limit{{Count: 10, Per: time.Second}, {Count: 120, Per: time.Minute}},
		sessionCommands:       []ratelimit.Limit{{Count: 60, Per: time.Second}, {Count: 600, Per: time.Minute}},
		pendingCommands:       4,
		processedTokens:       1000,
		pendingPerParticipant: 128,
		pendingPerSession:     2048,
		boundaryOperations:    256,
		boundaries:            []ratelimit.Limit{{Count: 25, Per: time.Second}, {Count: 1200, Per: time.Minute}},
		admins:                8,
		pendingViews:          8,
	}
}

// ControllerResolver finds or creates the controller of each Session this
// instance owns, up to a maximum, hands every new connection to it, and stops
// them all when the instance shuts down.
type ControllerResolver struct {
	// deps is what every controller runs its Session with.
	deps *controllerDependencies
	// maxLiveSessions is how many controllers may exist at once.
	maxLiveSessions int

	mu          sync.Mutex
	controllers map[string]*sessionController

	// root ends every controller's main goroutine when the instance shuts
	// down; running counts them.
	root    context.Context
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// New constructs a ControllerResolver running at most maxLiveSessions live
// Sessions (0 takes DefaultMaxLiveSessions), whose controllers admit through
// adminJoins and playerJoins, read manifests and limits, start, end,
// checkpoint and heartbeat Sessions, grant frontends, run games in sandbox,
// offer their history to replay and release their ownership through owners.
func New(maxLiveSessions int, adminJoins adminJoinChecker, playerJoins playerJoinChecker, manifests manifestLoader, limits limitsLoader, starts sessionStarter, ends sessionEnder, checkpoints checkpointStore, heartbeats ownerHeartbeater, frontends frontendSigner, sandbox gameSandbox, replay replayRecorder, owners ownerReleaser) *ControllerResolver {
	if maxLiveSessions <= 0 {
		maxLiveSessions = DefaultMaxLiveSessions
	}
	root, cancel := context.WithCancel(context.Background())
	return &ControllerResolver{
		maxLiveSessions: maxLiveSessions,
		deps: &controllerDependencies{
			adminJoins:         adminJoins,
			playerJoins:        playerJoins,
			manifests:          manifests,
			limits:             limits,
			starts:             starts,
			ends:               ends,
			checkpoints:        checkpoints,
			heartbeats:         heartbeats,
			frontends:          frontends,
			sandbox:            sandbox,
			replay:             replay,
			owners:             owners,
			now:                time.Now,
			seeds:              randomSeed,
			tickInterval:       defaultTickInterval,
			transportGrace:     defaultTransportGrace,
			recoveryWindow:     defaultRecoveryWindow,
			contextIdleTimeout: defaultContextIdleTimeout,
			heartbeatInterval:  sessionpkg.OwnerHeartbeatInterval,
			checkpointInterval: defaultCheckpointInterval,
			limitsInterval:     defaultLimitsInterval,
			inactivityTimeout:  defaultInactivityTimeout,
			liveLimits:         defaultLiveLimits(),
			logger:             slog.Default(),
		},
		controllers: make(map[string]*sessionController),
		root:        root,
		cancel:      cancel,
	}
}

// Close stops every controller and waits for them. Work in flight is put
// back, never failed, so shutting down aborts no Session.
func (r *ControllerResolver) Close() {
	r.cancel()
	r.running.Wait()
}

func randomSeed() uint64 {
	var seed [8]byte
	// crypto/rand.Read never fails.
	_, _ = rand.Read(seed[:])
	return binary.LittleEndian.Uint64(seed[:])
}

// controllerFor returns sessionUUID's controller under epoch, creating it
// when there is none, whatever the connection asking for it turns out to be.
// A controller from an older epoch is retired and replaced. Without a
// controller, rejection says why: epoch is older than the existing one's, or
// this instance runs as many live Sessions as it may.
func (r *ControllerResolver) controllerFor(ctx context.Context, sessionUUID string, epoch int64) (controller *sessionController, rejection runtime.RejectReason) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, found := r.controllers[sessionUUID]
	switch {
	case found && epoch == existing.epoch:
		return existing, ""
	case found && epoch < existing.epoch:
		return nil, runtime.RejectOwnerChanged
	case found:
		existing.retire()
		controllersRetired.Inc(ctx)
	case len(r.controllers) >= r.maxLiveSessions:
		limitRejections.Inc(ctx, limitLabel.Value(limitLiveSessions))
		return nil, runtime.RejectServerBusy
	}
	controller = newSessionController(r.root, sessionUUID, epoch, r.deps)
	r.controllers[sessionUUID] = controller
	controllersCreated.Inc(ctx)
	controllersActive.Inc(ctx)
	r.running.Add(1)
	go func() {
		defer r.running.Done()
		controller.run()
		controllersActive.Dec(controller.root)
		r.forget(controller)
	}()
	return controller, ""
}

// forget drops a controller whose main goroutine returned, unless a newer
// one already took its place; a later connection creates a new one.
func (r *ControllerResolver) forget(controller *sessionController) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.controllers[controller.sessionUUID] == controller {
		delete(r.controllers, controller.sessionUUID)
	}
}
