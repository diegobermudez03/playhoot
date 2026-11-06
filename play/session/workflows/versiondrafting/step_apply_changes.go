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
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	StoreSourceContents(ctx context.Context, tx *gorm.DB, contents map[string]string) error
	AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error)
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, provenance internalrepo.Provenance) error
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
// were made over and what they write, edit and delete, in order. Edits are
// left out when none, so a change without them keeps its fingerprint.
type changeRequest struct {
	Kind         string                        `json:"kind"`
	BaseRevision int64                         `json:"base_revision"`
	Files        []sessionpkg.DraftFileContent `json:"files"`
	Edits        []draftsources.Edit           `json:"edits,omitempty"`
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
	fingerprint, err := changeattempt.Fingerprint(changeRequest{Kind: "change", BaseRevision: change.BaseRevision, Files: change.Files, Edits: edits(change.Edits), Delete: change.Delete})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}

	done, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (committed[sessionpkg.DraftChangeResult], error) {
		return m.applyChangesInTx(ctx, tx, parsed.String(), change, fingerprint)
	})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	countSuperseded(ctx, done.superseded)
	result := done.result
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
	if result.Outcome == sessionpkg.DraftChangeApplied || result.Outcome == sessionpkg.DraftChangeUnchanged {
		made := result.Outcome == sessionpkg.DraftChangeApplied
		if result.Changes, err = m.writtenChanges(ctx, parsed.String(), result.Revision, made); err != nil {
			return sessionpkg.DraftChangeResult{}, err
		}
	}
	logging.LogFields(ctx, logging.Field("change_outcome", string(result.Outcome)), logging.Field("revision", result.Revision))
	return result, nil
}

// applyChangesInTx is ApplyChanges' work inside its transaction.
func (m *Manager) applyChangesInTx(ctx context.Context, tx *gorm.DB, versionUUID string, change sessionpkg.DraftChange, fingerprint string) (committed[sessionpkg.DraftChangeResult], error) {
	version, err := m.applyChangesRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	if version == nil {
		return committed[sessionpkg.DraftChangeResult]{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	attemptID, existing, err := m.applyChangesRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptChange), change.IdempotencyKey, fingerprint, change.BaseRevision, change.Origin)
	if err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	if existing != nil {
		replayed, err := changeattempt.Replay(ctx, existing, fingerprint)
		return committed[sessionpkg.DraftChangeResult]{result: replayed}, err
	}

	decided, diagnostics, err := m.decideChange(ctx, tx, version, change)
	if err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	var superseded int
	if decided.Outcome == sessionpkg.DraftChangeApplied {
		if superseded, err = m.queueBuild(ctx, tx, m.applyChangesRepo, version.ID, decided.Revision); err != nil {
			return committed[sessionpkg.DraftChangeResult]{}, err
		}
	}
	revision := decided.Revision
	if err := m.applyChangesRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(decided.Outcome), &revision, diagnostics); err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	return committed[sessionpkg.DraftChangeResult]{result: decided, superseded: superseded}, nil
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

	invalid := func(diagnostics []draftsources.Diagnostic) (sessionpkg.DraftChangeResult, []draftsources.Diagnostic, error) {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeInvalid, Revision: current, Diagnostics: changeattempt.Public(diagnostics)}, diagnostics, nil
	}
	edited, before, diagnostics, err := m.resolveEdits(ctx, tx, revision.Files, edits(change.Edits))
	if err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	if len(diagnostics) > 0 {
		return invalid(diagnostics)
	}
	writes := make([]draftsources.Write, 0, len(change.Files)+len(edited))
	for _, file := range change.Files {
		writes = append(writes, draftsources.Write{Path: file.Path, Content: file.Content})
	}
	// An edited path also written or deleted is a duplicate_path for Apply.
	writes = append(writes, edited...)
	applied := draftsources.Apply(revision.Files, writes, change.Delete)
	if len(applied.Diagnostics) > 0 {
		return invalid(applied.Diagnostics)
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
	if err := m.applyChangesRepo.CreateRevision(ctx, tx, version.ID, next, nextRevision, written(version, sessionpkg.DraftRevisionChange, current, change.Origin)); err != nil {
		return sessionpkg.DraftChangeResult{}, nil, err
	}
	return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: next, Diffs: editDiffs(edited, before)}, nil, nil
}

// resolveEdits turns edits into whole-file writes over the files of the base
// revision, answering as well the edited files' contents before them, by
// path.
func (m *Manager) resolveEdits(ctx context.Context, tx *gorm.DB, files map[string]draftsources.File, fileEdits []draftsources.Edit) ([]draftsources.Write, map[string]string, []draftsources.Diagnostic, error) {
	if len(fileEdits) == 0 {
		return nil, nil, nil, nil
	}
	contents, err := m.applyChangesRepo.FindSourceContents(ctx, tx, draftsources.EditedHashes(files, fileEdits))
	if err != nil {
		return nil, nil, nil, err
	}
	writes, diagnostics, err := draftsources.ResolveEdits(files, contents, fileEdits)
	if err != nil {
		monitoring.Alert(ctx, "play draft revision references a source content that is not stored")
		return nil, nil, nil, err
	}
	before := make(map[string]string, len(writes))
	for _, write := range writes {
		before[write.Path] = contents[files[write.Path].SHA256]
	}
	return writes, before, diagnostics, nil
}

// editDiffs are the diffs of the edited files, in the order they were
// edited.
func editDiffs(edited []draftsources.Write, before map[string]string) []sessionpkg.DraftFileDiff {
	if len(edited) == 0 {
		return nil
	}
	diffs := make([]sessionpkg.DraftFileDiff, len(edited))
	for i, write := range edited {
		diff := draftsources.Diff(write.Path, before[write.Path], write.Content)
		diffs[i] = sessionpkg.DraftFileDiff{Path: diff.Path, Diff: diff.Diff, Truncated: diff.Truncated}
	}
	return diffs
}

// edits are a change's edits in the sources' terms.
func edits(fileEdits []sessionpkg.DraftFileEdit) []draftsources.Edit {
	if len(fileEdits) == 0 {
		return nil
	}
	converted := make([]draftsources.Edit, len(fileEdits))
	for i, edit := range fileEdits {
		converted[i] = draftsources.Edit{Path: edit.Path, Find: edit.Find, Replace: edit.Replace, All: edit.All}
	}
	return converted
}
