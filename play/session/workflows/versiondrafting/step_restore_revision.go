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

//go:generate mockgen -package=versiondrafting -destination=step_restore_revision_mock_test.go . restoreRevisionRepo

// restoreRevisionRepo is RestoreRevision's own narrow persistence contract.
type restoreRevisionRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	ClaimChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, kind string, idempotencyKey string, requestSHA256 string, baseRevision int64, origin string) (uint, *internalrepo.ChangeAttempt, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error)
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, source string) error
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
	CompleteChangeAttempt(ctx context.Context, tx *gorm.DB, attemptID uint, outcome string, revision *int64, diagnostics []draftsources.Diagnostic) error
}

// restoreSource is the revision source of a restored revision.
const restoreSource = "restore"

// codeRevisionNotFound refuses a restore of a revision the version lacks.
const codeRevisionNotFound = "revision_not_found"

var (
	restoreOutcomeLabel = metrics.NewLabel("outcome", "applied", "unchanged", "conflict", "invalid", "not_editable", "replayed")

	draftRestoresTotal = metrics.NewCounter(
		"play_draft_restores_total",
		"Completed restores of an earlier DRAFT revision, by outcome; replayed counts retries answered from an earlier attempt.",
		restoreOutcomeLabel,
	)
)

// restoreRequest is what makes two restores the same request.
type restoreRequest struct {
	Kind         string `json:"kind"`
	Revision     int64  `json:"revision"`
	BaseRevision int64  `json:"base_revision"`
}

// RestoreRevision makes the DRAFT versionUUID's next revision hold exactly
// the files of its earlier revision, over baseRevision like any change and
// recorded as an attempt. History is never rewritten.
func (m *Manager) RestoreRevision(ctx context.Context, versionUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (sessionpkg.DraftChangeResult, error) {
	defer logging.Step(ctx, "versiondrafting.restore_revision").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if idempotencyKey == "" {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("idempotency key is required")
	}
	if !changeattempt.ValidOrigin(origin) {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("unknown change origin %q", origin)
	}
	if revision < 1 || baseRevision < 1 {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("revisions start at 1")
	}
	fingerprint, err := changeattempt.Fingerprint(restoreRequest{Kind: "restore", Revision: revision, BaseRevision: baseRevision})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}

	result, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftChangeResult, error) {
		return m.restoreRevisionInTx(ctx, tx, parsed.String(), revision, baseRevision, idempotencyKey, origin, fingerprint)
	})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	outcome := string(result.Outcome)
	if result.Replayed {
		outcome = "replayed"
	}
	draftRestoresTotal.Inc(ctx, restoreOutcomeLabel.Value(outcome))
	if result.Outcome == sessionpkg.DraftChangeApplied {
		if result.Build, err = m.builtRevision(ctx, parsed.String(), result.Revision, result.Replayed); err != nil {
			return sessionpkg.DraftChangeResult{}, err
		}
	}
	logging.LogFields(ctx, logging.Field("restore_outcome", string(result.Outcome)), logging.Field("revision", result.Revision))
	return result, nil
}

// restoreRevisionInTx is RestoreRevision's work inside its transaction.
func (m *Manager) restoreRevisionInTx(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string, fingerprint string) (sessionpkg.DraftChangeResult, error) {
	version, err := m.restoreRevisionRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	if version == nil {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	attemptID, existing, err := m.restoreRevisionRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptRestore), idempotencyKey, fingerprint, baseRevision, origin)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	if existing != nil {
		return changeattempt.Replay(ctx, existing, fingerprint)
	}

	decided, diagnostics, err := m.decideRestore(ctx, tx, version, revision, baseRevision)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	completed := decided.Revision
	if err := m.restoreRevisionRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(decided.Outcome), &completed, diagnostics); err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	return decided, nil
}

// decideRestore decides the restore against the version's current revision
// and, when it applies, writes the new revision.
func (m *Manager) decideRestore(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, revision int64, baseRevision int64) (sessionpkg.DraftChangeResult, []draftsources.Diagnostic, error) {
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeNotEditable, Revision: current}, nil, nil
	}
	if baseRevision != current {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: current}, nil, nil
	}
	restored, found, err := m.restoreRevisionRepo.FindRevision(ctx, tx, version.ID, revision)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if !found {
		diagnostics := []draftsources.Diagnostic{{Code: codeRevisionNotFound, Message: fmt.Sprintf("the version has no revision %d", revision)}}
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeInvalid, Revision: current, Diagnostics: changeattempt.Public(diagnostics)}, diagnostics, nil
	}
	currentRevision, found, err := m.restoreRevisionRepo.FindRevision(ctx, tx, version.ID, current)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing its current revision")
		return sessionpkg.DraftChangeResult{}, nil, fmt.Errorf("version %s has no revision %d", version.UUID, current)
	}
	if draftsources.SameRevision(currentRevision, restored) {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: current}, nil, nil
	}

	advanced, err := m.restoreRevisionRepo.AdvanceRevision(ctx, tx, version.ID, string(gameversion.Draft), current)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if !advanced {
		// Another change committed over the same base since this one read it.
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: current}, nil, nil
	}
	next := current + 1
	// The restored revision's contents and assets are already stored:
	// nothing is new but the revision.
	if err := m.restoreRevisionRepo.CreateRevision(ctx, tx, version.ID, next, restored, restoreSource); err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if err := m.queueBuild(ctx, tx, m.restoreRevisionRepo, version.ID, next); err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: next}, nil, nil
}
