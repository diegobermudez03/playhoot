package sessionlifecycle

import (
	"context"
	"time"

	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

//go:generate mockgen -package=sessionlifecycle -destination=mocks_test.go . createRepoAPI,joinRepoAPI,leaveRepoAPI,transactor,gameCurrentVersionReader,gamePinnedDefinitionReader

// defaultLobbyTTL is V1 Session Runtime policy (see WORK-0001's Known
// Risks/Questions - no repository-wide configuration surface exists yet to
// reuse for this).
const defaultLobbyTTL = 10 * time.Minute

// transactor lets the Manager decide transaction scope while delegating the
// mechanical BEGIN/COMMIT/ROLLBACK to infrastructure
// (`docs/engineering/standards/repositories.md`'s Transaction Ownership).
//
// Returning a non-nil error from fn rolls the transaction back - reserved
// for genuine infrastructure failures. A step that must still commit an
// already-decided durable outcome (lazy expiration materialization, an
// idempotency completion) while surfacing a business rejection to its own
// caller returns nil from fn and carries the business error out through its
// own result closure instead (see expiration.go and each step_*.go file) -
// so a business rejection never discards a durable outcome it must keep.
type transactor interface {
	RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error
}

// dbTransactor is transactor's real implementation, reusing
// utils.RunInDBTransaction - the one generic transaction helper already used
// elsewhere in the repository - rather than inventing a second one.
type dbTransactor struct {
	db *gorm.DB
}

func (t *dbTransactor) GetDB() *gorm.DB { return t.db }

func (t *dbTransactor) RunInTransaction(ctx context.Context, fn func(ctx context.Context, tx *gorm.DB) error) error {
	_, err := utils.RunInDBTransaction(ctx, t, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		return struct{}{}, fn(ctx, tx)
	})
	return err
}

// Manager is the Session lifecycle workflow controller, exposing
// Create/Join/Leave (see step_create.go/step_join.go/step_leave.go) as its
// steps. Each step depends on its own narrow persistence contract
// (createRepoAPI/joinRepoAPI/leaveRepoAPI) even though, today, one concrete
// internal/repo.Repo satisfies all three
// (`docs/engineering/standards/domain-logic-placement.md`'s Workflow
// Grouping Does Not Imply A Shared Repository Contract).
type Manager struct {
	createRepo createRepoAPI
	joinRepo   joinRepoAPI
	leaveRepo  leaveRepoAPI

	tx transactor

	currentGameReader gameCurrentVersionReader
	pinnedGameReader  gamePinnedDefinitionReader

	lobbyTTL time.Duration
	now      func() time.Time
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
		tx:                &dbTransactor{db: db},
		currentGameReader: currentGameReader,
		pinnedGameReader:  pinnedGameReader,
		lobbyTTL:          defaultLobbyTTL,
		now:               func() time.Time { return time.Now().UTC() },
	}
}
