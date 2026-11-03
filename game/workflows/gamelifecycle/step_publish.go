package gamelifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/card"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamelifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=gamelifecycle -destination=step_publish_mock_test.go . publishRepo

// publishRepo is the publication steps' own narrow persistence contract.
type publishRepo interface {
	LockGame(ctx context.Context, tx *gorm.DB, gameUUID string) (*internalrepo.OwnedGame, error)
	FindLatestEditionInStatus(ctx context.Context, tx *gorm.DB, gameID uint, status string) (*internalrepo.Edition, error)
	FindReleaseByKey(ctx context.Context, tx *gorm.DB, gameID uint, idempotencyKey string) (*internalrepo.ReleaseRecord, error)
	FindPendingRelease(ctx context.Context, tx *gorm.DB, gameID uint) (*internalrepo.ReleaseRecord, error)
	LockRelease(ctx context.Context, tx *gorm.DB, releaseUUID string) (*internalrepo.ReleaseRecord, error)
	NextReleaseSequence(ctx context.Context, tx *gorm.DB, gameID uint) (int64, error)
	CreateRelease(ctx context.Context, tx *gorm.DB, release internalrepo.NewRelease) error
	CompleteRelease(ctx context.Context, tx *gorm.DB, releaseID uint, languages []byte, at time.Time) (bool, error)
	AbandonRelease(ctx context.Context, tx *gorm.DB, releaseID uint, reason string, at time.Time) (bool, error)
	ChangeEditionStatus(ctx context.Context, tx *gorm.DB, editionID uint, from string, to string, publishedAt *time.Time, at time.Time) (bool, error)
	MarkPublished(ctx context.Context, tx *gorm.DB, game internalrepo.OwnedGame, visibility string, at time.Time) error
}

var (
	publicationOutcomeLabel = metrics.NewLabel("outcome", "started", "pending", "completed", "abandoned", "not_found", "not_editable", "not_ready")

	publicationStartsTotal = metrics.NewCounter(
		"game_publication_starts_total",
		"Requests to start publishing a Game, by outcome; completed and abandoned answer retries of releases that already ended.",
		publicationOutcomeLabel,
	)
	releaseEndOutcomeLabel = metrics.NewLabel("outcome", "completed", "abandoned")

	releasesEndedTotal = metrics.NewCounter(
		"game_releases_ended_total",
		"Releases that ended, completed or abandoned.",
		releaseEndOutcomeLabel,
	)
)

func releaseEndLabel(outcome string) metrics.LabelValue {
	return releaseEndOutcomeLabel.Value(outcome)
}

// publishableVisibilities are the visibilities a Game may be published with.
var publishableVisibilities = map[game.VisibilityType]bool{game.Public: true, game.Private: true}

// BeginPublication records a release of ownerUUID's Game gameUUID's open
// edition at revision with visibility, once the card allows it. Idempotent
// per (game, idempotencyKey); a release still being published is answered as
// pending, to be completed first.
func (m *Manager) BeginPublication(ctx context.Context, ownerUUID string, gameUUID string, revision int64, visibility game.VisibilityType, idempotencyKey string) (game.Publication, error) {
	defer logging.Step(ctx, "gamelifecycle.begin_publication").Close()

	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return game.Publication{Outcome: game.PublicationNotFound}, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return game.Publication{Outcome: game.PublicationNotFound}, nil
	}
	if !publishableVisibilities[visibility] {
		return game.Publication{}, fmt.Errorf("visibility %q cannot be published", visibility)
	}
	if idempotencyKey == "" || revision < 1 {
		return game.Publication{}, fmt.Errorf("an idempotency key and a revision are required")
	}
	started, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (game.Publication, error) {
		return m.beginPublicationInTx(ctx, tx, owner, parsedGame.String(), revision, visibility, idempotencyKey)
	})
	if err != nil {
		return game.Publication{}, err
	}
	publicationStartsTotal.Inc(ctx, publicationOutcomeLabel.Value(string(started.Outcome)))
	logging.LogFields(ctx, logging.Field("publication_outcome", string(started.Outcome)))
	return started, nil
}

func (m *Manager) beginPublicationInTx(ctx context.Context, tx *gorm.DB, owner uuid.UUID, gameUUID string, revision int64, visibility game.VisibilityType, idempotencyKey string) (game.Publication, error) {
	owned, err := m.ownedGame(ctx, tx, gameUUID, owner)
	if err != nil || owned == nil {
		return game.Publication{Outcome: game.PublicationNotFound}, err
	}
	existing, err := m.publishRepo.FindReleaseByKey(ctx, tx, owned.ID, idempotencyKey)
	if err != nil {
		return game.Publication{}, err
	}
	if existing != nil {
		if existing.Visibility != string(visibility) {
			return game.Publication{}, fmt.Errorf("publication key: %w", game.ErrIdempotencyConflict)
		}
		release := publicRelease(*existing)
		switch {
		case existing.CompletedAt != nil:
			return game.Publication{Outcome: game.PublicationCompleted, Release: &release}, nil
		case existing.AbandonedAt != nil:
			return game.Publication{Outcome: game.PublicationAbandoned, Release: &release}, nil
		default:
			return game.Publication{Outcome: game.PublicationStarted, Release: &release}, nil
		}
	}
	pending, err := m.publishRepo.FindPendingRelease(ctx, tx, owned.ID)
	if err != nil {
		return game.Publication{}, err
	}
	if pending != nil {
		release := publicRelease(*pending)
		return game.Publication{Outcome: game.PublicationPending, Release: &release}, nil
	}
	open, err := m.publishRepo.FindLatestEditionInStatus(ctx, tx, owned.ID, string(edition.Editing))
	if err != nil {
		return game.Publication{}, err
	}
	if open == nil {
		return game.Publication{Outcome: game.PublicationNotEditable}, nil
	}
	if blockers := card.PublishBlockers(owned.Name); len(blockers) > 0 {
		codes := make([]string, len(blockers))
		for i, blocker := range blockers {
			codes[i] = string(blocker)
		}
		return game.Publication{Outcome: game.PublicationNotReady, Blockers: codes}, nil
	}
	sequence, err := m.publishRepo.NextReleaseSequence(ctx, tx, owned.ID)
	if err != nil {
		return game.Publication{}, err
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	created := internalrepo.NewRelease{
		UUID: m.newUUID(), GameID: owned.ID, EditionID: open.ID, Sequence: sequence, Revision: revision,
		Visibility: string(visibility), OwnerUUID: owned.OwnerUUID, IdempotencyKey: idempotencyKey, CreatedAt: now,
	}
	if err := m.publishRepo.CreateRelease(ctx, tx, created); err != nil {
		return game.Publication{}, err
	}
	release := game.Release{
		UUID: created.UUID, GameUUID: owned.UUID, Sequence: sequence, VersionUUID: open.UUID,
		Revision: revision, Visibility: visibility,
	}
	return game.Publication{Outcome: game.PublicationStarted, Release: &release}, nil
}

// FindPublication answers the release ownerUUID's Game gameUUID recorded
// under idempotencyKey, as a retry of BeginPublication would; nil when the
// key holds none.
func (m *Manager) FindPublication(ctx context.Context, ownerUUID string, gameUUID string, idempotencyKey string) (*game.Publication, error) {
	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return nil, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return nil, nil
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (*game.Publication, error) {
		owned, err := m.ownedGame(ctx, tx, parsedGame.String(), owner)
		if err != nil || owned == nil {
			return nil, err
		}
		existing, err := m.publishRepo.FindReleaseByKey(ctx, tx, owned.ID, idempotencyKey)
		if err != nil || existing == nil {
			return nil, err
		}
		release := publicRelease(*existing)
		found := game.Publication{Outcome: game.PublicationStarted, Release: &release}
		switch {
		case existing.CompletedAt != nil:
			found.Outcome = game.PublicationCompleted
		case existing.AbandonedAt != nil:
			found.Outcome = game.PublicationAbandoned
		}
		return &found, nil
	})
}

// FindPendingPublication returns ownerUUID's Game gameUUID's release still
// being published, or nil.
func (m *Manager) FindPendingPublication(ctx context.Context, ownerUUID string, gameUUID string) (*game.Release, error) {
	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return nil, nil
	}
	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return nil, nil
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (*game.Release, error) {
		owned, err := m.ownedGame(ctx, tx, parsedGame.String(), owner)
		if err != nil || owned == nil {
			return nil, err
		}
		pending, err := m.publishRepo.FindPendingRelease(ctx, tx, owned.ID)
		if err != nil || pending == nil {
			return nil, err
		}
		release := publicRelease(*pending)
		return &release, nil
	})
}

// CompletePublication ends releaseUUID once Play runs its version: the
// edition becomes published, the Game published with the release's
// visibility (its first publication time written once), the release
// completed with the languages the game is written in, as Play's frozen
// version has them. Completing it again answers the same.
func (m *Manager) CompletePublication(ctx context.Context, releaseUUID string, languages []string) (game.Release, error) {
	defer logging.Step(ctx, "gamelifecycle.complete_publication").Close()

	if len(languages) == 0 {
		return game.Release{}, fmt.Errorf("a completed release has the languages its game is written in")
	}
	encoded, err := json.Marshal(languages)
	if err != nil {
		return game.Release{}, fmt.Errorf("encoding the release's languages: %w", err)
	}
	completed, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (lifecycleEnd, error) {
		return m.completeInTx(ctx, tx, releaseUUID, encoded)
	})
	if err != nil {
		return game.Release{}, err
	}
	if completed.ended {
		releasesEndedTotal.Inc(ctx, releaseEndLabel("completed"))
	}
	return completed.release, nil
}

// lifecycleEnd is a release a step ended, or found already ended.
type lifecycleEnd struct {
	release game.Release
	ended   bool
}

func (m *Manager) completeInTx(ctx context.Context, tx *gorm.DB, releaseUUID string, languages []byte) (lifecycleEnd, error) {
	release, err := m.publishRepo.LockRelease(ctx, tx, releaseUUID)
	if err != nil {
		return lifecycleEnd{}, err
	}
	if release == nil {
		return lifecycleEnd{}, fmt.Errorf("release %s not found", releaseUUID)
	}
	if release.CompletedAt != nil {
		return lifecycleEnd{release: publicRelease(*release)}, nil
	}
	if release.AbandonedAt != nil {
		return lifecycleEnd{}, fmt.Errorf("release %s was abandoned", releaseUUID)
	}
	owned, err := m.publishRepo.LockGame(ctx, tx, release.GameUUID)
	if err != nil {
		return lifecycleEnd{}, err
	}
	if owned == nil {
		monitoring.Alert(ctx, "game release references a game that does not exist")
		return lifecycleEnd{}, fmt.Errorf("release %s references a missing game", releaseUUID)
	}
	now := m.now().UTC().Truncate(time.Microsecond)
	changed, err := m.publishRepo.ChangeEditionStatus(ctx, tx, release.EditionID, string(edition.Editing), string(edition.Published), &now, now)
	if err != nil {
		return lifecycleEnd{}, err
	}
	if !changed {
		monitoring.Alert(ctx, "game release's edition is not open any more")
		return lifecycleEnd{}, fmt.Errorf("release %s: edition %s is not open", releaseUUID, release.EditionUUID)
	}
	if err := m.publishRepo.MarkPublished(ctx, tx, *owned, release.Visibility, now); err != nil {
		return lifecycleEnd{}, err
	}
	if _, err := m.publishRepo.CompleteRelease(ctx, tx, release.ID, languages, now); err != nil {
		return lifecycleEnd{}, err
	}
	release.CompletedAt = &now
	return lifecycleEnd{release: publicRelease(*release), ended: true}, nil
}

// AbandonPublication ends releaseUUID without publishing it, with why: Play
// refused to run its revision. A release already ended is answered as is.
func (m *Manager) AbandonPublication(ctx context.Context, releaseUUID string, reason string) (game.Release, error) {
	defer logging.Step(ctx, "gamelifecycle.abandon_publication").Close()

	abandoned, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (lifecycleEnd, error) {
		release, err := m.publishRepo.LockRelease(ctx, tx, releaseUUID)
		if err != nil {
			return lifecycleEnd{}, err
		}
		if release == nil {
			return lifecycleEnd{}, fmt.Errorf("release %s not found", releaseUUID)
		}
		if release.CompletedAt != nil || release.AbandonedAt != nil {
			return lifecycleEnd{release: publicRelease(*release)}, nil
		}
		now := m.now().UTC().Truncate(time.Microsecond)
		if _, err := m.publishRepo.AbandonRelease(ctx, tx, release.ID, reason, now); err != nil {
			return lifecycleEnd{}, err
		}
		release.AbandonedAt = &now
		return lifecycleEnd{release: publicRelease(*release), ended: true}, nil
	})
	if err != nil {
		return game.Release{}, err
	}
	if abandoned.ended {
		releasesEndedTotal.Inc(ctx, releaseEndLabel("abandoned"))
	}
	return abandoned.release, nil
}

// ownedGame locks gameUUID and returns it when owner owns it, or nil.
func (m *Manager) ownedGame(ctx context.Context, tx *gorm.DB, gameUUID string, owner uuid.UUID) (*internalrepo.OwnedGame, error) {
	owned, err := m.publishRepo.LockGame(ctx, tx, gameUUID)
	if err != nil || owned == nil {
		return nil, err
	}
	recorded, err := uuid.Parse(owned.OwnerUUID)
	if err != nil || recorded != owner {
		return nil, nil
	}
	return owned, nil
}

func publicRelease(record internalrepo.ReleaseRecord) game.Release {
	return game.Release{
		UUID: record.UUID, GameUUID: record.GameUUID, Sequence: record.Sequence, VersionUUID: record.EditionUUID,
		Revision: record.Revision, Visibility: game.VisibilityType(record.Visibility),
		CompletedAt: record.CompletedAt, AbandonedAt: record.AbandonedAt,
	}
}
