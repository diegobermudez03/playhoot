package sessionlifecycle

import (
	"time"

	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

//go:generate mockgen -package=sessionlifecycle -destination=mocks_test.go . createRepoAPI,joinRepoAPI,leaveRepoAPI,startRepoAPI,gameCurrentVersionReader,gamePinnedDefinitionReader

// defaultLobbyTTL is the lobby lifetime applied when no other TTL
// configuration is supplied.
const defaultLobbyTTL = 10 * time.Minute

// Session lifecycle idempotency operation labels
// (`docs/engineering/standards/idempotency.md`).
const (
	operationCreate = "CREATE"
	operationJoin   = "JOIN"
	operationLeave  = "LEAVE"
	operationStart  = "START"
)

// Manager is the Session lifecycle workflow controller, exposing
// Create/Join/Leave/Start as its steps. Each step depends on its own narrow
// persistence contract rather than one shared repository interface, even
// though a single concrete internal/repo.Repo currently satisfies all of
// them, so a step's dependencies stay scoped to what it actually needs
// (`docs/engineering/standards/domain-logic-placement.md`'s Workflow
// Grouping Does Not Imply A Shared Repository Contract). Start's own
// RuntimeTurn execution logic lives inline rather than in a separate shared
// package, since no other current use case calls it independently.
//
// Manager decides transaction scope itself by calling
// utils.RunInDBTransaction directly, rather than holding a separate injected
// `transactor` dependency whose only purpose would be to indirect into that
// already generic helper (`docs/engineering/standards/repositories.md`'s
// Transaction Ownership). Manager satisfies utils.DBServicer itself via
// GetDB so it can be passed directly to that helper.
type Manager struct {
	createRepo createRepoAPI
	joinRepo   joinRepoAPI
	leaveRepo  leaveRepoAPI
	startRepo  startRepoAPI

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
// playable Definition/Version (Create's Game Management dependency,
// GAME-ADR-0001/GAME-ADR-0004); pinnedGameReader loads an already-pinned
// immutable Game Definition by its own Definition/Version UUID, never
// "current version" (Join's dependency, reused unchanged by Start), since
// the pinned Definition must stay immutable for the lifetime of the
// Session.
func New(db *gorm.DB, currentGameReader gameCurrentVersionReader, pinnedGameReader gamePinnedDefinitionReader) *Manager {
	r := internalrepo.New(db)
	return &Manager{
		createRepo:        r,
		joinRepo:          r,
		leaveRepo:         r,
		startRepo:         r,
		db:                db,
		currentGameReader: currentGameReader,
		pinnedGameReader:  pinnedGameReader,
		lobbyTTL:          defaultLobbyTTL,
	}
}
