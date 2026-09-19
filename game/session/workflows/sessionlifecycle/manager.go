package sessionlifecycle

import (
	"time"

	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

//go:generate mockgen -package=sessionlifecycle -destination=mocks_test.go . createRepoAPI,joinRepoAPI,leaveRepoAPI,gameCurrentVersionReader,gamePinnedDefinitionReader

// defaultLobbyTTL is V1 Session Runtime policy (see WORK-0001's Known
// Risks/Questions - no repository-wide configuration surface exists yet to
// reuse for this).
const defaultLobbyTTL = 10 * time.Minute

// Session lifecycle idempotency operation labels
// (`docs/engineering/standards/idempotency.md`).
const (
	operationCreate = "CREATE"
	operationJoin   = "JOIN"
	operationLeave  = "LEAVE"
)

// Manager is the Session lifecycle workflow controller, exposing
// Create/Join/Leave (see step_create.go/step_join.go/step_leave.go) as its
// steps. Each step depends on its own narrow persistence contract
// (createRepoAPI/joinRepoAPI/leaveRepoAPI) even though, today, one concrete
// internal/repo.Repo satisfies all three
// (`docs/engineering/standards/domain-logic-placement.md`'s Workflow
// Grouping Does Not Imply A Shared Repository Contract).
//
// Manager decides transaction scope itself by calling
// utils.RunInDBTransaction directly (see step_create.go/step_join.go/
// step_leave.go), rather than holding a separate injected `transactor`
// dependency whose only purpose would be to indirect into that already
// generic helper (`docs/engineering/standards/repositories.md`'s
// Transaction Ownership). Manager satisfies utils.DBServicer itself via
// GetDB so it can be passed directly to that helper.
type Manager struct {
	createRepo createRepoAPI
	joinRepo   joinRepoAPI
	leaveRepo  leaveRepoAPI

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
// "current version" (Join's dependency - see WORK-0001's Pinned Game
// Definition Is Immutable For The Session).
func New(db *gorm.DB, currentGameReader gameCurrentVersionReader, pinnedGameReader gamePinnedDefinitionReader) *Manager {
	r := internalrepo.New(db)
	return &Manager{
		createRepo:        r,
		joinRepo:          r,
		leaveRepo:         r,
		db:                db,
		currentGameReader: currentGameReader,
		pinnedGameReader:  pinnedGameReader,
		lobbyTTL:          defaultLobbyTTL,
	}
}
