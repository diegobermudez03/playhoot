// Package versiondrafting is the workflow of a game version while it is a
// DRAFT: created with the starter sources, changed in place one revision at a
// time over the revision its author last read, given the assets it declares,
// restored, and read back.
package versiondrafting

import (
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/buildsignal"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// maxRecentAttempts is how many of a version's latest change attempts a read
// returns.
const maxRecentAttempts = 10

// assetObjectPrefix is where checked asset content is stored,
// content-addressed: the same prefix versions' assets live under.
const assetObjectPrefix = "game-versions/assets"

// assetLinkTTL is how long a read's preview links work.
const assetLinkTTL = 15 * time.Minute

// Manager is the version drafting workflow controller. Each step depends on
// its own narrow persistence contract, all satisfied by one internal Repo,
// and on the narrow part of object storage it uses.
type Manager struct {
	createDraftVersionRepo createDraftVersionRepo
	readDraftRepo          readDraftRepo
	applyChangesRepo       applyChangesRepo
	restoreRevisionRepo    restoreRevisionRepo
	listRevisionsRepo      listRevisionsRepo
	uploadAssetRepo        uploadAssetRepo
	removeAssetRepo        removeAssetRepo
	readBuildRepo          readBuildRepo

	assetLinks   assetLinkSigner
	assetStorage assetStorage

	// buildSignal wakes the build worker when a revision's build is queued.
	buildSignal *buildsignal.Signal

	// uploadSlots bounds the asset uploads held in memory at once.
	uploadSlots chan struct{}

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	now     func() time.Time
	newUUID func() string
}

// New constructs a Manager persisting to db, keeping asset content in store
// and waking the build worker through signal.
func New(db *gorm.DB, store storage.Store, signal *buildsignal.Signal) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		createDraftVersionRepo: repo,
		readDraftRepo:          repo,
		applyChangesRepo:       repo,
		restoreRevisionRepo:    repo,
		listRevisionsRepo:      repo,
		uploadAssetRepo:        repo,
		removeAssetRepo:        repo,
		readBuildRepo:          repo,
		buildSignal:            signal,
		assetLinks:             store,
		assetStorage:           store,
		uploadSlots:            make(chan struct{}, maxConcurrentUploads),
		dbServicer:             repo,
		now:                    time.Now,
		newUUID:                newUUIDString,
	}
}
