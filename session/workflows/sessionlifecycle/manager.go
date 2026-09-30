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
	"time"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/activity"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/expiration"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

//go:generate mockgen -package=sessionlifecycle -destination=mocks_test.go . createRepoAPI,joinRepoAPI,leaveRepoAPI,startRepoAPI,submitPlayerEventRepoAPI,expireTimerRepoAPI,cancelSessionRepoAPI,getClientStateRepoAPI

// defaultLobbyTTL is the lobby lifetime applied when no other TTL
// configuration is supplied.
const defaultLobbyTTL = 10 * time.Minute

// defaultActivityTTL is the RUNNING-phase inactivity deadline applied when no
// other TTL configuration is supplied.
const defaultActivityTTL = 10 * time.Minute

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

	dbServicer dbServicer

	lobbyExpirer    *expiration.Expirer
	activityExpirer *activity.Expirer

	executor executor.Executor

	lobbyTTL    time.Duration
	activityTTL time.Duration
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
// internal/repo.
func New(db *gorm.DB, exec executor.Executor) *Manager {
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
		dbServicer:            r,
		lobbyExpirer:          expiration.NewExpirer(r),
		activityExpirer:       activity.NewExpirer(r),
		executor:              exec,
		lobbyTTL:              defaultLobbyTTL,
		activityTTL:           defaultActivityTTL,
	}
}
