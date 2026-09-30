// Package sessionlifecycle is the Session lifecycle workflow's
// implementation: one Manager exposing
// Create/Join/Leave/Start/AnswerInteraction/ExpireTimer as its steps. The
// Manager decides business/lifecycle policy (transaction scope, admission,
// expiration, idempotency-replay meaning); its narrow `internal/repo`
// persistence layer reports facts and performs the mutations the Manager
// requests.
//
// This package is not the workflow's public contract - session is.
// Every type Manager's methods take or return is defined there, at a
// zero-dependency package a caller may depend on without transitively
// importing anything this package needs for its own implementation (the
// Game Language engine, GORM, and so on). This package refers to those
// types fully qualified (session.SessionUUID, session.StartResult, ...)
// rather than re-declaring them under local names.
package sessionlifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/monitoring"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/activity"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/expiration"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

//go:generate mockgen -package=sessionlifecycle -destination=mocks_test.go . createRepoAPI,joinRepoAPI,leaveRepoAPI,startRepoAPI,submitPlayerEventRepoAPI,expireTimerRepoAPI,cancelSessionRepoAPI,getClientStateRepoAPI,contentAccessRepoAPI

// defaultLobbyTTL is the lobby lifetime applied when no other TTL
// configuration is supplied.
const defaultLobbyTTL = 10 * time.Minute

// defaultActivityTTL is the RUNNING-phase inactivity deadline applied when no
// other TTL configuration is supplied.
const defaultActivityTTL = 10 * time.Minute

// defaultSignedURLTTL is how long a signed content URL stays valid when no
// other TTL is supplied: long enough for a browser to start the download,
// short enough that a leaked URL is worth little.
const defaultSignedURLTTL = 2 * time.Minute

// Session lifecycle idempotency operation labels, scoping each command's
// idempotency identity to (UserUUID, operation, IdempotencyKey).
const (
	operationCreate            = "CREATE"
	operationJoin              = "JOIN"
	operationLeave             = "LEAVE"
	operationStart             = "START"
	operationSubmitPlayerEvent = "SUBMIT_PLAYER_EVENT"
	operationCancelSession     = "CANCEL_SESSION"
)

// Manager is the Session lifecycle workflow controller, exposing
// Create/Join/Leave/Start/SubmitPlayerEvent/CancelSession/ExpireTimer as its
// steps: LOBBY admission and RUNNING-phase execution both stay on this one
// Manager rather than splitting into a separate workflow package. Each step
// depends on its own narrow persistence contract naming only the methods
// that step actually calls, rather than one shared repository interface -
// even though a single concrete internal/repo.Repo currently satisfies all
// of them - so adding a method for one step never forces every other step's
// interface, mock, and test to change with it. Execution itself is entirely
// owned by the injected executor.Executor port - this package never
// implements any part of the sandboxed JavaScript execution model itself,
// and holds no dependency on how/where the Executor actually runs.
//
// Manager decides transaction scope itself by calling
// utils.RunInDBTransaction directly, rather than holding a separate injected
// `transactor` dependency whose only purpose would be to indirect into that
// already generic helper. Manager holds no persistence handle of its own -
// its own internal/repo.Repo already owns it (that is Repo's job, not
// Manager's), so Manager passes its dbServicer field, backed by that same
// Repo value, directly to utils.RunInDBTransaction instead of duplicating
// the handle onto Manager just to shoehorn Manager into the helper's
// required shape.
type Manager struct {
	createRepo            createRepoAPI
	joinRepo              joinRepoAPI
	leaveRepo             leaveRepoAPI
	startRepo             startRepoAPI
	expireTimerRepo       expireTimerRepoAPI
	submitPlayerEventRepo submitPlayerEventRepoAPI
	cancelSessionRepo     cancelSessionRepoAPI
	getClientStateRepo    getClientStateRepoAPI
	contentAccessRepo     contentAccessRepoAPI

	dbServicer dbServicer

	lobbyExpirer    *expiration.Expirer
	activityExpirer *activity.Expirer

	executor executor.Executor

	// objectStore holds every game version's immutable content; scripts
	// loads and hash-verifies a backend script from it, remembering
	// verified bytes so the steady-state path never touches storage.
	objectStore objectstore.Store
	scripts     *objectstore.Loader

	lobbyTTL     time.Duration
	activityTTL  time.Duration
	signedURLTTL time.Duration
}

// dbServicer is the narrow capability Manager needs to open a DB
// transaction via utils.RunInDBTransaction: reaching the *gorm.DB handle
// its own internal/repo.Repo already owns exclusively.
type dbServicer interface {
	GetDB() *gorm.DB
}

// New constructs a Manager. exec is Session Runtime's own caller-side port
// onto the separately deployed JavaScript Executor - every RUNNING-phase
// step calls it, never a local/in-process execution mechanism. Create needs
// no Game Management reader at all: it resolves the Game's current
// pinnable version, and every RUNNING-phase step resolves its pinned
// artifact, entirely from Session Runtime's own tables through
// internal/repo. store is the private object storage holding every game
// version's script and asset bytes - the database records only where they
// are and how to verify them.
func New(db *gorm.DB, exec executor.Executor, store objectstore.Store) *Manager {
	r := internalrepo.New(db)
	return &Manager{
		createRepo:            r,
		joinRepo:              r,
		leaveRepo:             r,
		startRepo:             r,
		expireTimerRepo:       r,
		submitPlayerEventRepo: r,
		cancelSessionRepo:     r,
		getClientStateRepo:    r,
		contentAccessRepo:     r,
		dbServicer:            r,
		lobbyExpirer:          expiration.NewExpirer(r),
		activityExpirer:       activity.NewExpirer(r),
		executor:              exec,
		objectStore:           store,
		scripts:               objectstore.NewLoader(store, objectstore.DefaultLoaderEntries),
		lobbyTTL:              defaultLobbyTTL,
		activityTTL:           defaultActivityTTL,
		signedURLTTL:          defaultSignedURLTTL,
	}
}

// backendScriptSource loads artifact's backend script from object storage,
// verified against the hash its locator recorded, ready to hand to the
// Executor. Verified bytes are remembered, so once a script has been loaded
// on this instance no later call touches storage. A missing or corrupt
// object is an invariant break - a pinned version's content must always be
// present and unchanged - so it alerts as well as failing; a plain
// transport failure only fails.
func (m *Manager) backendScriptSource(ctx context.Context, artifact *internalrepo.GameVersionArtifact) (executor.ResolvedScript, error) {
	source, err := m.scripts.Load(ctx, artifact.BackendScript)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) || errors.Is(err, objectstore.ErrHashMismatch) {
			monitoring.Alert(ctx, "session pinned backend script is missing or corrupt in object storage")
		}
		return executor.ResolvedScript{}, fmt.Errorf("loading backend script: %w", err)
	}
	return executor.ResolvedScript{Source: string(source)}, nil
}

// warmBackendScript loads artifact's backend script in the background so the
// first RUNNING-phase step for a Session on this instance usually finds it
// already verified and in memory, instead of fetching from storage while it
// holds the Session row lock. Best-effort only: it never blocks or fails its
// caller, and a step that finds the script still cold simply loads it itself.
func (m *Manager) warmBackendScript(ctx context.Context, artifact *internalrepo.GameVersionArtifact) {
	if m.scripts == nil {
		return
	}
	warmCtx := context.WithoutCancel(ctx)
	go func() {
		warmCtx, cancel := context.WithTimeout(warmCtx, 10*time.Second)
		defer cancel()
		_, _ = m.scripts.Load(warmCtx, artifact.BackendScript)
	}()
}
