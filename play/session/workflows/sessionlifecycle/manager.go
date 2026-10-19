// Package sessionlifecycle is the durable Session lifecycle workflow: a
// Manager whose methods are the lifecycle steps, one file per step.
package sessionlifecycle

import (
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// defaultLobbyTTL is how long a Session stays joinable in LOBBY, and how long
// its join code stays valid, before it expires.
const defaultLobbyTTL = 30 * time.Minute

// defaultMaxLifetime is the most time a Session may live from its creation,
// whatever its activity; a platform tuning value.
const defaultMaxLifetime = 4 * time.Hour

// Manager is the Session lifecycle workflow controller. Each step depends on
// its own narrow persistence contract, even though one internal/repo.Repo
// satisfies all of them, so adding a method for one step never changes
// another step's interface or mocks.
type Manager struct {
	createSessionRepo      createSessionRepo
	checkStartRepo         checkStartRepo
	commitStartRepo        commitStartRepo
	loadRunningSessionRepo loadRunningSessionRepo
	endSessionRepo         endSessionRepo

	// scripts reads a game version's backend script from object storage.
	scripts scriptLoader

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	now         func() time.Time
	newJoinCode func() (string, error)
	lobbyTTL    time.Duration
	maxLifetime time.Duration
}

// New constructs a Manager persisting to db and reading game version content
// from store.
func New(db *gorm.DB, store storage.Store) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		createSessionRepo:      repo,
		checkStartRepo:         repo,
		commitStartRepo:        repo,
		loadRunningSessionRepo: repo,
		endSessionRepo:         repo,
		scripts:                objectstore.NewLoader(store, objectstore.DefaultLoaderEntries),
		dbServicer:             repo,
		now:                    time.Now,
		newJoinCode:            newJoinCode,
		lobbyTTL:               defaultLobbyTTL,
		maxLifetime:            defaultMaxLifetime,
	}
}
