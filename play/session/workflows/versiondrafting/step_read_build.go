package versiondrafting

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

//go:generate mockgen -package=versiondrafting -destination=step_read_build_mock_test.go . readBuildRepo

// readBuildRepo is the narrow persistence contract of reading builds.
type readBuildRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindBuild(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64, running string) (*internalrepo.Build, error)
	FindLastBuildInStatus(ctx context.Context, tx *gorm.DB, versionUUID string, status string, running string) (*internalrepo.Build, error)
}

// buildReader reads a version's builds.
type buildReader interface {
	FindBuild(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64, running string) (*internalrepo.Build, error)
	FindLastBuildInStatus(ctx context.Context, tx *gorm.DB, versionUUID string, status string, running string) (*internalrepo.Build, error)
}

// buildQueuer queues a revision's build inside the transaction that made the
// revision.
type buildQueuer interface {
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
}

// MaxBuildWait is the longest ReadBuild waits for a build to end.
const MaxBuildWait = 20 * time.Second

// buildPollInterval is how often a waiting ReadBuild looks at the build again.
const buildPollInterval = 250 * time.Millisecond

var draftBuildsSupersededTotal = metrics.NewCounter(
	"play_draft_builds_superseded_total",
	"Queued DRAFT builds a newer revision's build replaced before they ran.",
)

// committed is what a transaction that may queue a build decided, with how
// many waiting builds the queued one superseded, counted once it commits.
type committed[T any] struct {
	result     T
	superseded int
}

// queueBuild queues the build of revision, superseding the version's builds
// still waiting, and returns how many it superseded: only the newest
// revision is worth building.
func (m *Manager) queueBuild(ctx context.Context, tx *gorm.DB, queuer buildQueuer, versionID uint, revision int64) (int, error) {
	return queuer.QueueBuild(ctx, tx, versionID, revision, m.newUUID(), string(build.Queued), string(build.Superseded))
}

// countSuperseded counts the builds a committed transaction superseded.
func countSuperseded(ctx context.Context, superseded int) {
	if superseded > 0 {
		draftBuildsSupersededTotal.Add(ctx, float64(superseded))
	}
}

// builtRevision wakes the build worker and adds the build of revision to an
// answer that made, or replays, that revision.
func (m *Manager) builtRevision(ctx context.Context, versionUUID string, revision int64, replayed bool) (*sessionpkg.DraftBuild, error) {
	if !replayed {
		m.buildSignal.Notify()
	}
	return m.buildOf(ctx, versionUUID, revision)
}

// buildOf is the build of revision of versionUUID, or nil.
func (m *Manager) buildOf(ctx context.Context, versionUUID string, revision int64) (*sessionpkg.DraftBuild, error) {
	found, err := m.readBuildRepo.FindBuild(ctx, m.dbServicer.GetDB(), versionUUID, revision, string(build.Running))
	if err != nil || found == nil {
		return nil, err
	}
	return publicBuild(found)
}

// ReadBuild returns the build of revision of versionUUID, its current
// revision when revision is 0. With wait, it answers once the build ended
// or wait (at most MaxBuildWait) passed, whichever comes first. A revision
// without a build is sessionpkg.ErrDraftBuildNotFound.
func (m *Manager) ReadBuild(ctx context.Context, versionUUID string, revision int64, wait time.Duration) (sessionpkg.DraftBuild, error) {
	defer logging.Step(ctx, "versiondrafting.read_build").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftBuild{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if revision < 0 {
		return sessionpkg.DraftBuild{}, fmt.Errorf("revision must not be negative")
	}
	wait = min(max(wait, 0), MaxBuildWait)
	db := m.dbServicer.GetDB()
	version, err := m.readBuildRepo.FindVersion(ctx, db, parsed.String())
	if err != nil {
		return sessionpkg.DraftBuild{}, err
	}
	if version == nil {
		return sessionpkg.DraftBuild{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	if revision == 0 && version.CurrentRevision != nil {
		revision = *version.CurrentRevision
	}

	deadline := m.now().Add(wait)
	for {
		found, err := m.readBuildRepo.FindBuild(ctx, db, version.UUID, revision, string(build.Running))
		if err != nil {
			return sessionpkg.DraftBuild{}, err
		}
		if found == nil {
			return sessionpkg.DraftBuild{}, fmt.Errorf("version %s revision %d: %w", versionUUID, revision, sessionpkg.ErrDraftBuildNotFound)
		}
		if build.Status(found.Status).Terminal() || !m.now().Before(deadline) {
			public, err := publicBuild(found)
			if err != nil {
				return sessionpkg.DraftBuild{}, err
			}
			logging.LogFields(ctx, logging.Field("build_status", found.Status), logging.Field("revision", revision))
			return *public, nil
		}
		select {
		case <-ctx.Done():
			return sessionpkg.DraftBuild{}, ctx.Err()
		case <-time.After(min(buildPollInterval, deadline.Sub(m.now()))):
		}
	}
}

// readBuildsInTx adds to a snapshot the build of its revision and the
// version's newest succeeded build.
func (m *Manager) readBuildsInTx(ctx context.Context, tx *gorm.DB, reader buildReader, snapshot *sessionpkg.DraftSnapshot) error {
	if snapshot.Revision > 0 {
		found, err := reader.FindBuild(ctx, tx, snapshot.VersionUUID, snapshot.Revision, string(build.Running))
		if err != nil {
			return err
		}
		if found != nil {
			if snapshot.Build, err = publicBuild(found); err != nil {
				return err
			}
		}
	}
	last, err := reader.FindLastBuildInStatus(ctx, tx, snapshot.VersionUUID, string(build.Succeeded), string(build.Running))
	if err != nil {
		return err
	}
	if last != nil {
		snapshot.LastSuccessfulBuild, err = publicBuild(last)
	}
	return err
}

// publicBuild is a stored build as the contract shows it.
func publicBuild(found *internalrepo.Build) (*sessionpkg.DraftBuild, error) {
	public := &sessionpkg.DraftBuild{
		Revision:   found.Revision,
		Status:     sessionpkg.DraftBuildStatus(found.Status),
		Manifest:   found.Manifest,
		Reused:     found.ReusedBuildID != nil,
		QueuedAt:   found.CreatedAt,
		StartedAt:  found.StartedAt,
		FinishedAt: found.FinishedAt,
	}
	if found.AspectRatio != nil {
		public.AspectRatio = *found.AspectRatio
	}
	if found.ServerSHA256 != nil && found.ServerSize != nil {
		public.Server = &sessionpkg.DraftBuildArtifact{SHA256: *found.ServerSHA256, Size: *found.ServerSize}
	}
	if found.ClientSHA256 != nil && found.ClientSize != nil {
		public.Client = &sessionpkg.DraftBuildArtifact{SHA256: *found.ClientSHA256, Size: *found.ClientSize}
	}
	if len(found.Diagnostics) > 0 {
		var diagnostics []build.Diagnostic
		if err := json.Unmarshal(found.Diagnostics, &diagnostics); err != nil {
			return nil, fmt.Errorf("decoding build diagnostics: %w", err)
		}
		for _, d := range diagnostics {
			public.Diagnostics = append(public.Diagnostics, sessionpkg.DraftBuildDiagnostic{
				Stage: string(d.Stage), Code: d.Code, File: d.File, Line: d.Line, Column: d.Column, Message: d.Message,
			})
		}
	}
	return public, nil
}
