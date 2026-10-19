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

// ControllerResolver finds or creates the controller of each Session this
// instance owns, hands every new connection to it, and stops them all when
// the instance shuts down.
type ControllerResolver struct {
	// deps is what every controller runs its Session with.
	deps *controllerDependencies

	mu          sync.Mutex
	controllers map[string]*sessionController

	// root ends every controller's main goroutine when the instance shuts
	// down; running counts them.
	root    context.Context
	cancel  context.CancelFunc
	running sync.WaitGroup
}

// New constructs a ControllerResolver whose controllers admit through
// adminJoins and playerJoins, read manifests and limits, start, end and
// checkpoint Sessions, grant frontends, run games in sandbox, offer their
// history to replay and release their ownership through owners.
func New(adminJoins adminJoinChecker, playerJoins playerJoinChecker, manifests manifestLoader, limits limitsLoader, starts sessionStarter, ends sessionEnder, checkpoints checkpointStore, frontends frontendSigner, sandbox gameSandbox, replay replayRecorder, owners ownerReleaser) *ControllerResolver {
	root, cancel := context.WithCancel(context.Background())
	return &ControllerResolver{
		deps: &controllerDependencies{
			adminJoins:         adminJoins,
			playerJoins:        playerJoins,
			manifests:          manifests,
			limits:             limits,
			starts:             starts,
			ends:               ends,
			checkpoints:        checkpoints,
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
			checkpointInterval: defaultCheckpointInterval,
			limitsInterval:     defaultLimitsInterval,
			inactivityTimeout:  defaultInactivityTimeout,
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
// A controller from an older epoch is retired and replaced. ok is false when
// epoch is older than the existing one.
func (r *ControllerResolver) controllerFor(ctx context.Context, sessionUUID string, epoch int64) (controller *sessionController, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, found := r.controllers[sessionUUID]
	switch {
	case found && epoch == existing.epoch:
		return existing, true
	case found && epoch < existing.epoch:
		return nil, false
	case found:
		existing.retire()
		controllersRetired.Inc(ctx)
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
	return controller, true
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
