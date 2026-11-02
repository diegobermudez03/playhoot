// Package versiondrafting is the workflow of a game version while it is a
// DRAFT: created with the starter sources, changed in place one revision at a
// time over the revision its author last read, restored, and read back.
package versiondrafting

import (
	"gorm.io/gorm"

	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// maxRecentAttempts is how many of a version's latest change attempts a read
// returns.
const maxRecentAttempts = 10

// Manager is the version drafting workflow controller. Each step depends on
// its own narrow persistence contract, all satisfied by one internal Repo.
type Manager struct {
	createDraftVersionRepo createDraftVersionRepo
	readDraftRepo          readDraftRepo
	applyChangesRepo       applyChangesRepo
	restoreRevisionRepo    restoreRevisionRepo
	listRevisionsRepo      listRevisionsRepo

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer
}

// New constructs a Manager persisting to db.
func New(db *gorm.DB) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		createDraftVersionRepo: repo,
		readDraftRepo:          repo,
		applyChangesRepo:       repo,
		restoreRevisionRepo:    repo,
		listRevisionsRepo:      repo,
		dbServicer:             repo,
	}
}
