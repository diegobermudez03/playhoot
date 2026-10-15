// Package sessionlifecycle is the durable Session lifecycle workflow: one
// Manager exposing the lifecycle steps (today, creating a Session) as its
// methods, each step in its own file. The Manager decides business/lifecycle
// policy (transaction scope, idempotency meaning, join code allocation); its
// internal/repo reports facts and performs the mutations the Manager
// requests.
package sessionlifecycle

import (
	"time"

	"gorm.io/gorm"

	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// defaultLobbyTTL is how long a Session stays joinable in LOBBY, and how long
// its join code stays valid, before it expires.
const defaultLobbyTTL = 30 * time.Minute

// Manager is the Session lifecycle workflow controller. Each step depends on
// its own narrow persistence contract, even though one internal/repo.Repo
// satisfies all of them, so adding a method for one step never changes
// another step's interface or mocks.
type Manager struct {
	createSessionRepo createSessionRepo

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	now         func() time.Time
	newJoinCode func() (string, error)
	lobbyTTL    time.Duration
}

// New constructs a Manager persisting to db.
func New(db *gorm.DB) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		createSessionRepo: repo,
		dbServicer:        repo,
		now:               time.Now,
		newJoinCode:       newJoinCode,
		lobbyTTL:          defaultLobbyTTL,
	}
}
