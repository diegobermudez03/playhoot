// Package gameloop runs the single mutation lane of every running game this
// instance owns: at each boundary, a tick or a flush an operation asks for,
// it executes what the timeline accepted, commits the result to the
// controller's mirror and lets every participant see it.
package gameloop

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameloop -destination=manager_mock_test.go . gameSandbox,sessionAborter,replayRecorder

// gameSandbox executes a running game in its contexts. An error is an
// infrastructure failure, never the script's.
type gameSandbox interface {
	CreateContext(ctx context.Context, spec sandbox.ContextSpec) (string, error)
	Restore(ctx context.Context, contextID string, snapshot sandbox.Snapshot) (sandbox.RestoreResult, error)
	Samples(ctx context.Context, contextID string, input sandbox.SamplesInput) (sandbox.ExecutionResult, error)
	Project(ctx context.Context, contextID string, participantID string) (sandbox.ProjectResult, error)
	DiscardContext(ctx context.Context, contextID string) error
}

// sessionAborter ends a running Session the platform cannot continue.
type sessionAborter interface {
	AbortRunningSession(ctx context.Context, sessionUUID string, cause sessionpkg.AbortCause) error
}

// replayRecorder takes what a game offers to its replay history. Offer must
// never block: recording is best effort.
type replayRecorder interface {
	Offer(ctx context.Context, capture replay.Capture)
}

// maxInfraFailures is how many boundaries in a row may fail to execute before
// the Session is aborted: about five seconds at a 100 ms cadence.
const maxInfraFailures = 50

// Manager runs the lanes of the games this instance owns.
type Manager struct {
	sandbox  gameSandbox
	sessions sessionAborter
	replay   replayRecorder
	logger   *slog.Logger

	// root ends every lane when the instance shuts down.
	root   context.Context
	cancel context.CancelFunc
	lanes  sync.WaitGroup
}

// New constructs a Manager running games in sandbox, aborting their Sessions
// through sessions and offering their history to replay.
func New(sandbox gameSandbox, sessions sessionAborter, replay replayRecorder) *Manager {
	root, cancel := context.WithCancel(context.Background())
	return &Manager{
		sandbox:  sandbox,
		sessions: sessions,
		replay:   replay,
		logger:   slog.Default(),
		root:     root,
		cancel:   cancel,
	}
}

// Run starts the lane of sessionUUID's running game, with a boundary every
// interval, which must be positive; it ends when the game stops running on
// sessionController or the Manager closes.
func (m *Manager) Run(sessionUUID string, sessionController *controller.Controller, interval time.Duration) {
	m.lanes.Add(1)
	lanesActive.Inc(m.root)
	go func() {
		defer m.lanes.Done()
		defer lanesActive.Dec(m.root)
		m.lane(sessionUUID, sessionController, interval)
	}()
}

// Close stops every lane and waits for them: a boundary in flight is put
// back, never failed, so shutting down aborts no Session.
func (m *Manager) Close() {
	m.cancel()
	m.lanes.Wait()
}

func (m *Manager) lane(sessionUUID string, sessionController *controller.Controller, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	state := &laneState{}
	for {
		select {
		case <-m.root.Done():
			return
		case <-sessionController.Done():
			return
		case <-ticker.C:
		case <-sessionController.FlushRequests():
		}
		if !m.boundary(sessionUUID, sessionController, state) {
			return
		}
	}
}

// laneState is what a lane remembers between boundaries.
type laneState struct {
	// infraFailures counts boundaries in a row that could not execute.
	infraFailures int
}
