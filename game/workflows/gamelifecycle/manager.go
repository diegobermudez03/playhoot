// Package gamelifecycle is a Game's lifecycle in Game Management: created
// unpublished with an edition open, published, edited again in a new edition.
// A Manager whose methods are the lifecycle steps, one file per step.
package gamelifecycle

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamelifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// Manager is the Game lifecycle workflow controller. Each step depends on its
// own narrow persistence contract, even though one internal/repo.Repo
// satisfies all of them.
type Manager struct {
	createGameRepo createGameRepo

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	now     func() time.Time
	newUUID func() string
}

// New constructs a Manager persisting to db.
func New(db *gorm.DB) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		createGameRepo: repo,
		dbServicer:     repo,
		now:            time.Now,
		newUUID:        uuid.NewString,
	}
}
