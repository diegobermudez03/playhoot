// Package versionbuilding is the workflow of a DRAFT revision's build: the
// worker takes queued builds one at a time, builds the revision's sources
// with the game toolchain, checks and stores what it produced, and records
// how it ended.
package versionbuilding

import (
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/buildsignal"
	"github.com/diegobermudez03/playhoot/play/session/internal/gamebuild"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versionbuilding/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// Object storage prefixes of built content, content-addressed below them:
// the same prefixes executable versions' content lives under.
const (
	backendScriptPrefix   = "game-versions/backend-scripts"
	frontendPackagePrefix = "game-versions/frontend-packages"
)

// DefaultWorkers is how many builds an instance runs at once.
const DefaultWorkers = 1

// defaultCheckInterval is how often an idle worker looks for queued builds
// it was not woken for (queued by another instance, or left after a crash).
const defaultCheckInterval = 5 * time.Second

// Manager is the build workflow controller.
type Manager struct {
	runBuildRepo runBuildRepo
	requeueRepo  requeueRepo

	builder   gamebuild.Builder
	artifacts artifactStorage
	signal    *buildsignal.Signal

	// dbServicer is the handle utils.RunInDBTransaction opens transactions
	// on; it is the same Repo, which owns the database handle.
	dbServicer utils.DBServicer

	now           func() time.Time
	workers       int
	checkInterval time.Duration
	logger        *slog.Logger
}

// New constructs a Manager reading builds from db, building with builder,
// storing artifacts in store and woken by signal. workers zero takes
// DefaultWorkers.
func New(db *gorm.DB, store storage.Store, builder gamebuild.Builder, signal *buildsignal.Signal, workers int) *Manager {
	if workers <= 0 {
		workers = DefaultWorkers
	}
	repo := internalrepo.New(db)
	return &Manager{
		runBuildRepo:  repo,
		requeueRepo:   repo,
		builder:       builder,
		artifacts:     store,
		signal:        signal,
		dbServicer:    repo,
		now:           time.Now,
		workers:       workers,
		checkInterval: defaultCheckInterval,
		logger:        slog.Default(),
	}
}
