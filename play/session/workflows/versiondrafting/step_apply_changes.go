package versiondrafting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/changeattempt"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_apply_changes_mock_test.go . applyChangesRepo

// applyChangesRepo is ApplyChanges' own narrow persistence contract.
type applyChangesRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	ClaimChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, kind string, idempotencyKey string, requestSHA256 string, baseRevision int64, origin string) (uint, *internalrepo.ChangeAttempt, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	StoreSourceContents(ctx context.Context, tx *gorm.DB, contents map[string]string) error
	AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error)
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, source string) error
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
	CompleteChangeAttempt(ctx context.Context, tx *gorm.DB, attemptID uint, outcome string, revision *int64, diagnostics []draftsources.Diagnostic) error
}

var (
	changeOutcomeLabel = metrics.NewLabel("outcome", "applied", "unchanged", "conflict", "invalid", "not_editable", "replayed")

	draftChangesTotal = metrics.NewCounter(
		"play_draft_changes_total",
		"Completed changes to DRAFT sources, by outcome; replayed counts retries answered from an earlier attempt.",
		changeOutcomeLabel,
	)
)

// changeRequest is what makes two changes the same request: the base they
// were made over and what they write and delete, in order.
type changeRequest struct {
	Kind         string                        `json:"kind"`
	BaseRevision int64                         `json:"base_revision"`
	Files        []sessionpkg.DraftFileContent `json:"files"`
	Delete       []string                      `json:"delete"`
}

// ApplyChanges applies change to the DRAFT versionUUID as its next revision,
// whole or not at all, and records the attempt whatever it decides. The same
// idempotency key with the same change answers the first decision again;
// with another change it is sessionpkg.ErrIdempotencyConflict.
func (m *Manager) ApplyChanges(ctx context.Context, versionUUID string, change sessionpkg.DraftChange) (sessionpkg.DraftChangeResult, error) {
	defer logging.Step(ctx, "versiondrafting.apply_changes").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if change.IdempotencyKey == "" {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("idempotency key is required")
	}
	if !changeattempt.ValidOrigin(change.Origin) {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("unknown change origin %q", change.Origin)
	}
	if change.BaseRevision < 1 {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("base revision must be at least 1")
	}
	fingerprint, err := changeattempt.Fingerprint(changeRequest{Kind: "change", BaseRevision: change.BaseRevision, Files: change.Files, Delete: change.Delete})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}

	result, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftChangeResult, error) {
		return m.applyChangesInTx(ctx, tx, parsed.String(), change, fingerprint)
	})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	outcome := string(result.Outcome)
	if result.Replayed {
		outcome = "replayed"
	}
	draftChangesTotal.Inc(ctx, changeOutcomeLabel.Value(outcome))
	if result.Outcome == sessionpkg.DraftChangeApplied {
		if result.Build, err = m.builtRevision(ctx, parsed.String(), result.Revision, result.Replayed); err != nil {
			return sessionpkg.DraftChangeResult{}, err
		}
	}
	logging.LogFields(ctx, logging.Field("change_outcome", string(result.Outcome)), logging.Field("revision", result.Revision))
	return result, nil
}

// applyChangesInTx is ApplyChanges' work inside its transaction.
func (m *Manager) applyChangesInTx(ctx context.Context, tx *gorm.DB, versionUUID string, change sessionpkg.DraftChange, fingerprint string) (sessionpkg.DraftChangeResult, error) {
	version, err := m.applyChangesRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	if version == nil {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	attemptID, existing, err := m.applyChangesRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptChange), change.IdempotencyKey, fingerprint, change.BaseRevision, change.Origin)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	if existing != nil {
		return changeattempt.Replay(ctx, existing, fingerprint)
	}

	decided, diagnostics, err := m.decideChange(ctx, tx, version, change)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	revision := decided.Revision
	if err := m.applyChangesRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(decided.Outcome), &revision, diagnostics); err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	return decided, nil
}

// decideChange decides the change against the version's current revision
// and, when it applies, writes the new revision.
func (m *Manager) decideChange(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, change sessionpkg.DraftChange) (sessionpkg.DraftChangeResult, []draftsources.Diagnostic, error) {
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeNotEditable, Revision: current}, nil, nil
	}
	if change.BaseRevision != current {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: current}, nil, nil
	}
	revision, found, err := m.applyChangesRepo.FindRevision(ctx, tx, version.ID, current)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing its current revision")
		return sessionpkg.DraftChangeResult{}, nil, fmt.Errorf("version %s has no revision %d", version.UUID, current)
	}

	writes := make([]draftsources.Write, len(change.Files))
	for i, file := range change.Files {
		writes[i] = draftsources.Write{Path: file.Path, Content: file.Content}
	}
	applied := draftsources.Apply(revision.Files, writes, change.Delete)
	if len(applied.Diagnostics) > 0 {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeInvalid, Revision: current, Diagnostics: changeattempt.Public(applied.Diagnostics)}, applied.Diagnostics, nil
	}
	if draftsources.SameFiles(revision.Files, applied.Files) {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: current}, nil, nil
	}

	advanced, err := m.applyChangesRepo.AdvanceRevision(ctx, tx, version.ID, string(gameversion.Draft), current)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if !advanced {
		// Another change committed over the same base since this one read it.
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: current}, nil, nil
	}
	if err := m.applyChangesRepo.StoreSourceContents(ctx, tx, applied.Contents); err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	next := current + 1
	// Asset bindings are kept: a change of the sources never touches them.
	nextRevision := draftsources.Revision{Files: applied.Files, Assets: revision.Assets}
	if err := m.applyChangesRepo.CreateRevision(ctx, tx, version.ID, next, nextRevision, change.Origin); err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if err := m.queueBuild(ctx, tx, m.applyChangesRepo, version.ID, next); err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: next}, nil, nil
}
