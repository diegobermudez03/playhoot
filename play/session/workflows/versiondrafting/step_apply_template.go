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

//go:generate mockgen -package=versiondrafting -destination=step_apply_template_mock_test.go . applyTemplateRepo

// applyTemplateRepo is ApplyTemplate's own narrow persistence contract.
type applyTemplateRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	ClaimChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, kind string, idempotencyKey string, requestSHA256 string, baseRevision int64, origin string) (uint, *internalrepo.ChangeAttempt, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error)
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, provenance internalrepo.Provenance) error
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
	CompleteChangeAttempt(ctx context.Context, tx *gorm.DB, attemptID uint, outcome string, revision *int64, diagnostics []draftsources.Diagnostic) error
}

var (
	templateOutcomeLabel = metrics.NewLabel("outcome", "applied", "unchanged", "conflict", "not_editable", "replayed")

	draftTemplatesTotal = metrics.NewCounter(
		"play_draft_templates_applied_total",
		"Completed switches of a DRAFT to a template, by outcome; replayed counts retries answered from an earlier attempt.",
		templateOutcomeLabel,
	)
)

// templateRequest is what makes two switches to a template the same request.
type templateRequest struct {
	Kind         string `json:"kind"`
	GameUUID     string `json:"template_game_uuid"`
	VersionUUID  string `json:"template_version_uuid"`
	BaseRevision int64  `json:"base_revision"`
}

// ApplyTemplate makes the DRAFT versionUUID's next revision start over from
// template's files and assets, keeping the draft's notes.md, brief.json and
// the uploads the template still declares, over baseRevision like any change.
// History and the template are never changed.
func (m *Manager) ApplyTemplate(ctx context.Context, versionUUID string, template sessionpkg.DraftTemplate, baseRevision int64, idempotencyKey string, origin string) (sessionpkg.DraftChangeResult, error) {
	defer logging.Step(ctx, "versiondrafting.apply_template").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	templateGame, gameErr := uuid.Parse(template.GameUUID)
	templateVersion, versionErr := uuid.Parse(template.VersionUUID)
	if gameErr != nil || versionErr != nil {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("template game and version uuids must be valid UUIDs")
	}
	template = sessionpkg.DraftTemplate{GameUUID: templateGame.String(), VersionUUID: templateVersion.String()}
	if idempotencyKey == "" {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("idempotency key is required")
	}
	if !changeattempt.ValidOrigin(origin) {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("unknown change origin %q", origin)
	}
	if baseRevision < 1 {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("base revision must be at least 1")
	}
	fingerprint, err := changeattempt.Fingerprint(templateRequest{Kind: "template", GameUUID: template.GameUUID, VersionUUID: template.VersionUUID, BaseRevision: baseRevision})
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}

	done, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (committed[sessionpkg.DraftChangeResult], error) {
		return m.applyTemplateInTx(ctx, tx, parsed.String(), template, baseRevision, idempotencyKey, origin, fingerprint)
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
	draftTemplatesTotal.Inc(ctx, templateOutcomeLabel.Value(outcome))
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
	logging.LogFields(ctx, logging.Field("template_outcome", string(result.Outcome)), logging.Field("revision", result.Revision), logging.Field("template_game_uuid", template.GameUUID))
	return result, nil
}

// applyTemplateInTx is ApplyTemplate's work inside its transaction.
func (m *Manager) applyTemplateInTx(ctx context.Context, tx *gorm.DB, versionUUID string, template sessionpkg.DraftTemplate, baseRevision int64, idempotencyKey string, origin string, fingerprint string) (committed[sessionpkg.DraftChangeResult], error) {
	version, err := m.applyTemplateRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	if version == nil {
		return committed[sessionpkg.DraftChangeResult]{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	attemptID, existing, err := m.applyTemplateRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptTemplate), idempotencyKey, fingerprint, baseRevision, origin)
	if err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	if existing != nil {
		replayed, err := changeattempt.Replay(ctx, existing, fingerprint)
		return committed[sessionpkg.DraftChangeResult]{result: replayed}, err
	}

	decided, err := m.decideTemplate(ctx, tx, version, template, baseRevision, origin)
	if err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	var superseded int
	if decided.Outcome == sessionpkg.DraftChangeApplied {
		if superseded, err = m.queueBuild(ctx, tx, m.applyTemplateRepo, version.ID, decided.Revision); err != nil {
			return committed[sessionpkg.DraftChangeResult]{}, err
		}
	}
	completed := decided.Revision
	if err := m.applyTemplateRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(decided.Outcome), &completed, nil); err != nil {
		return committed[sessionpkg.DraftChangeResult]{}, err
	}
	return committed[sessionpkg.DraftChangeResult]{result: decided, superseded: superseded}, nil
}

// decideTemplate decides the switch against the version's current revision
// and, when it applies, writes the new revision.
func (m *Manager) decideTemplate(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, template sessionpkg.DraftTemplate, baseRevision int64, origin string) (sessionpkg.DraftChangeResult, error) {
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeNotEditable, Revision: current}, nil
	}
	if baseRevision != current {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: current}, nil
	}
	draft, found, err := m.applyTemplateRepo.FindRevision(ctx, tx, version.ID, current)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing its current revision")
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("version %s has no revision %d", version.UUID, current)
	}
	source, sourceRevision, err := m.templateRevision(ctx, tx, template)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	declared, err := m.declaredAssets(ctx, tx, source)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	next := draftsources.FromTemplate(draft, source, declared)
	if draftsources.SameRevision(draft, next) {
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: current}, nil
	}

	advanced, err := m.applyTemplateRepo.AdvanceRevision(ctx, tx, version.ID, string(gameversion.Draft), current)
	if err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	if !advanced {
		// Another change committed over the same base since this one read it.
		return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: current}, nil
	}
	provenance := written(version, sessionpkg.DraftRevisionTemplate, current, origin)
	provenance.FromGameUUID, provenance.FromVersionUUID, provenance.FromRevision = template.GameUUID, template.VersionUUID, &sourceRevision
	// Every content the new revision holds is already stored: the
	// template's and the draft's.
	if err := m.applyTemplateRepo.CreateRevision(ctx, tx, version.ID, current+1, next, provenance); err != nil {
		return sessionpkg.DraftChangeResult{}, err
	}
	return sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: current + 1}, nil
}

// templateRevision is the revision template's published version was frozen
// at, and its number.
func (m *Manager) templateRevision(ctx context.Context, tx *gorm.DB, template sessionpkg.DraftTemplate) (draftsources.Revision, int64, error) {
	source, err := m.applyTemplateRepo.FindVersion(ctx, tx, template.VersionUUID)
	if err != nil {
		return draftsources.Revision{}, 0, err
	}
	if source == nil || source.GameUUID != template.GameUUID || gameversion.Status(source.Status) != gameversion.Executable || source.CurrentRevision == nil {
		return draftsources.Revision{}, 0, fmt.Errorf("version %s of template %s: %w", template.VersionUUID, template.GameUUID, sessionpkg.ErrDraftTemplateNotFound)
	}
	revision, found, err := m.applyTemplateRepo.FindRevision(ctx, tx, source.ID, *source.CurrentRevision)
	if err != nil {
		return draftsources.Revision{}, 0, err
	}
	if !found {
		monitoring.Alert(ctx, "play executable version is missing the revision it was frozen at")
		return draftsources.Revision{}, 0, fmt.Errorf("version %s has no revision %d", source.UUID, *source.CurrentRevision)
	}
	return revision, *source.CurrentRevision, nil
}

// declaredAssets are the assets revision's game.project.json declares; none
// when it cannot be read (then no upload of the draft is kept).
func (m *Manager) declaredAssets(ctx context.Context, tx *gorm.DB, revision draftsources.Revision) (map[string]draftsources.AssetDeclaration, error) {
	project, found := revision.Files[draftsources.ProjectFile]
	if !found {
		return nil, nil
	}
	contents, err := m.applyTemplateRepo.FindSourceContents(ctx, tx, []string{project.SHA256})
	if err != nil {
		return nil, err
	}
	declared, diagnostics := draftsources.DeclaredAssets(contents[project.SHA256])
	if len(diagnostics) > 0 {
		return nil, nil
	}
	return declared, nil
}
