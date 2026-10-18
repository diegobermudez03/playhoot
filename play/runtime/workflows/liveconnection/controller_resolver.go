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

// New constructs a ControllerResolver whose controllers decide who may join
// through adminJoins and playerJoins, read their game's manifest through
// manifests, start games through starts, abort them through aborts, grant
// their frontends through frontends, run them in sandbox and offer their
// history to replay.
func New(adminJoins adminJoinChecker, playerJoins playerJoinChecker, manifests manifestLoader, starts sessionStarter, aborts sessionAborter, frontends frontendSigner, sandbox gameSandbox, replay replayRecorder) *ControllerResolver {
	root, cancel := context.WithCancel(context.Background())
	return &ControllerResolver{
		deps: &controllerDependencies{
			adminJoins:   adminJoins,
			playerJoins:  playerJoins,
			manifests:    manifests,
			starts:       starts,
			aborts:       aborts,
			frontends:    frontends,
			sandbox:      sandbox,
			replay:       replay,
			now:          time.Now,
			seeds:        randomSeed,
			tickInterval: defaultTickInterval,
			logger:       slog.Default(),
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
	r.running.Add(1)
	go func() {
		defer r.running.Done()
		controller.run()
	}()
	return controller, true
}
