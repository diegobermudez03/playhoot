// Package sessionlifecycle is the Session lifecycle workflow's
// implementation: one Manager exposing
// Create/Join/Leave/Start/AnswerInteraction/ExpireTimer as its steps. The
// Manager decides business/lifecycle policy (transaction scope, admission,
// expiration, idempotency-replay meaning); its narrow `internal/repo`
// persistence layer reports facts and performs the mutations the Manager
// requests.
//
// This package is not the workflow's public contract - game/session is.
// Every type Manager's methods take or return is defined there, at a
// zero-dependency package a caller may depend on without transitively
// importing anything this package needs for its own implementation (the
// Game Language engine, GORM, and so on). This package refers to those
// types fully qualified (session.SessionUUID, session.StartResult, ...)
// rather than re-declaring them under local names.
package sessionlifecycle

import (
	"time"

	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

//go:generate mockgen -package=sessionlifecycle -destination=mocks_test.go . createRepoAPI,joinRepoAPI,leaveRepoAPI,startRepoAPI,answerInteractionRepoAPI,expireTimerRepoAPI,outputsRepoAPI,gameCurrentVersionReader,gamePinnedDefinitionReader

// defaultLobbyTTL is the lobby lifetime applied when no other TTL
// configuration is supplied.
const defaultLobbyTTL = 10 * time.Minute

// Session lifecycle idempotency operation labels, scoping each command's
// idempotency identity to (UserUUID, operation, IdempotencyKey).
const (
	operationCreate = "CREATE"
	operationJoin   = "JOIN"
	operationLeave  = "LEAVE"
	operationStart  = "START"
)

// Manager is the Session lifecycle workflow controller, exposing
// Create/Join/Leave/Start/AnswerInteraction as its steps: LOBBY admission
// and RUNNING-phase execution both stay on this one Manager rather than
// splitting into a separate workflow package. Each step depends on its own
// narrow persistence contract naming only the methods that step actually
// calls, rather than one shared repository interface - even though a single
// concrete internal/repo.Repo currently satisfies all of them - so adding a
// method for one step never forces every other step's interface, mock, and
// test to change with it. RuntimeTurn draining/bound execution is entirely
// owned by `engineservice.StartTurn`/`AdvanceTurn` - this package never
// implements any part of the engine's own execution model itself.
//
// Manager decides transaction scope itself by calling
// utils.RunInDBTransaction directly, rather than holding a separate injected
// `transactor` dependency whose only purpose would be to indirect into that
// already generic helper. Manager satisfies utils.DBServicer itself via
// GetDB so it can be passed directly to that helper.
type Manager struct {
	createRepo            createRepoAPI
	joinRepo              joinRepoAPI
	leaveRepo             leaveRepoAPI
	startRepo             startRepoAPI
	answerInteractionRepo answerInteractionRepoAPI
	expireTimerRepo       expireTimerRepoAPI
	outputsRepo           outputsRepoAPI

	db *gorm.DB

	currentGameReader gameCurrentVersionReader
	pinnedGameReader  gamePinnedDefinitionReader

	lobbyTTL time.Duration
}

// GetDB satisfies utils.DBServicer, letting Manager itself be passed
// directly to utils.RunInDBTransaction.
func (m *Manager) GetDB() *gorm.DB {
	return m.db
}

// New constructs a Manager. currentGameReader resolves a Game's current
// playable Definition/Version; pinnedGameReader loads an already-pinned
// immutable Game Definition by its own Definition/Version UUID, never
// "current version", since the pinned Definition must stay immutable for
// the lifetime of the Session.
func New(db *gorm.DB, currentGameReader gameCurrentVersionReader, pinnedGameReader gamePinnedDefinitionReader) *Manager {
	r := internalrepo.New(db)
	return &Manager{
		createRepo:            r,
		joinRepo:              r,
		leaveRepo:             r,
		startRepo:             r,
		answerInteractionRepo: r,
		expireTimerRepo:       r,
		outputsRepo:           r,
		db:                    db,
		currentGameReader:     currentGameReader,
		pinnedGameReader:      pinnedGameReader,
		lobbyTTL:              defaultLobbyTTL,
	}
}
