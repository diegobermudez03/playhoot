// Package previewlifecycle is the durable record of previews: a draft's
// owner playing one SUCCEEDED build of a revision with synthetic
// participants, which the live Runtime runs. A Manager whose methods are the
// steps, one file per step.
package previewlifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionlifetime"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/previewlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// maxLifetime is the most a preview lives, whatever its activity: as long as
// a Session may.
const maxLifetime = sessionlifetime.Max

// frontendExpiryMargin keeps a preview's signed frontend working a little
// past the preview's own end.
const frontendExpiryMargin = 5 * time.Minute

// builtRevisions is what previews need of the draft versions: a revision's
// build and its signed frontend (versiondrafting.Manager).
type builtRevisions interface {
	LoadBuiltRevision(ctx context.Context, versionUUID string, revision int64) (sessionpkg.BuiltRevision, error)
	SignBuiltFrontend(ctx context.Context, versionUUID string, revision int64, expiresAt time.Time) (sessionpkg.FrontendAccess, error)
}

// Manager is the preview lifecycle controller. Each step depends on its own
// narrow persistence contract, even though one internal/repo.Repo satisfies
// all of them.
type Manager struct {
	openPreviewRepo openPreviewRepo
	endPreviewRepo  endPreviewRepo
	findPreviewRepo findPreviewRepo

	builds builtRevisions

	dbServicer utils.DBServicer

	now     func() time.Time
	newUUID func() string
}

// New constructs a Manager persisting to db and reading builds from builds.
func New(db *gorm.DB, builds builtRevisions) *Manager {
	repo := internalrepo.New(db)
	return &Manager{
		openPreviewRepo: repo,
		endPreviewRepo:  repo,
		findPreviewRepo: repo,
		builds:          builds,
		dbServicer:      repo,
		now:             time.Now,
		newUUID:         uuid.NewString,
	}
}

// participantIDs are the synthetic participants of a preview of n players.
func participantIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("P%d", i+1)
	}
	return ids
}
