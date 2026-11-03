package gameauthoring

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameauthoring -destination=publish_mock_test.go . gamePublisher,versionPublisher,cardNamer

// cardNamer is Game Management's card, which a publication may rename.
type cardNamer interface {
	UpdateCard(ctx context.Context, ownerUUID string, gameUUID string, change game.CardChange) (game.CardEdit, error)
}

// gamePublisher is Game Management's side of publishing: releases, which it
// records and ends, and the editions a published game is edited in again.
type gamePublisher interface {
	BeginPublication(ctx context.Context, ownerUUID string, gameUUID string, revision int64, visibility game.VisibilityType, idempotencyKey string) (game.Publication, error)
	FindPendingPublication(ctx context.Context, ownerUUID string, gameUUID string) (*game.Release, error)
	FindPublication(ctx context.Context, ownerUUID string, gameUUID string, idempotencyKey string) (*game.Publication, error)
	CompletePublication(ctx context.Context, releaseUUID string, languages []string) (game.Release, error)
	AbandonPublication(ctx context.Context, releaseUUID string, reason string) (game.Release, error)
	OpenEdition(ctx context.Context, ownerUUID string, gameUUID string, idempotencyKey string) (game.Edition, error)
}

// versionPublisher is Play's side of publishing: whether a version being
// built may be frozen, freezing it into the game's executable version, and
// cleaning the stored content nothing uses any more.
type versionPublisher interface {
	CheckPublication(ctx context.Context, versionUUID string) (sessionpkg.PublishReadiness, error)
	FreezeVersion(ctx context.Context, gameUUID string, versionUUID string, revision int64) (sessionpkg.FrozenVersion, error)
	CleanUnusedObjects(ctx context.Context) (sessionpkg.CleanedObjects, error)
}

// PublishOutcome is what Publish decided.
type PublishOutcome string

const (
	// Published means the release is complete: its version runs new Sessions.
	Published PublishOutcome = "published"
	// PublishNotReady means something is missing: Blockers say what.
	PublishNotReady PublishOutcome = "not_ready"
	// PublishNotEditable means the game has no edition open to publish.
	PublishNotEditable PublishOutcome = "not_editable"
	// PublishNotFound means the user owns no game with that uuid.
	PublishNotFound PublishOutcome = "not_found"
)

// invalidNameBlocker is a name given with a publication that the card refuses.
const invalidNameBlocker = "invalid_name"

// PublishBlocker is one thing missing to publish: a card rule (Game
// Management) or the version's build and assets (Play).
type PublishBlocker struct {
	Code        string
	BuildStatus string
	Keys        []string
}

// Publication is what Publish decided, with the release when published.
type Publication struct {
	Outcome  PublishOutcome
	Release  *game.Release
	Blockers []PublishBlocker
}

var (
	resumedReleasesTotal = metrics.NewCounter(
		"orchestrator_game_authoring_releases_resumed_total",
		"Releases a later operation found still being published and completed or abandoned.",
	)
	cleanupFailuresTotal = metrics.NewCounter(
		"orchestrator_game_authoring_cleanup_failures_total",
		"Cleanups of unused stored content after a publication that failed; the next publication tries again.",
	)
)

// Publish publishes the current revision of userUUID's game gameUUID's open
// edition with visibility: Game Management records a release, Play freezes
// the version and points the game at it, Game Management completes the
// release, then unused stored content is cleaned, best effort. Each step is
// idempotent; a release left half done is completed by the next call or edit.
func (m *Manager) Publish(ctx context.Context, userUUID string, gameUUID string, visibility game.VisibilityType, name *string, idempotencyKey string) (Publication, error) {
	defer logging.Step(ctx, "orchestrator.publish").Close()

	if err := m.resumePublication(ctx, userUUID, gameUUID); err != nil {
		return Publication{}, err
	}
	// A retry of a publication that already ended answers what it decided,
	// though its edition is no longer open.
	if earlier, err := m.publications.FindPublication(ctx, userUUID, gameUUID, idempotencyKey); err != nil {
		return Publication{}, fmt.Errorf("finding the publication: %s", err)
	} else if earlier != nil && earlier.Release != nil {
		switch earlier.Outcome {
		case game.PublicationCompleted:
			return Publication{Outcome: Published, Release: earlier.Release}, nil
		case game.PublicationAbandoned:
			return Publication{Outcome: PublishNotReady, Release: earlier.Release, Blockers: []PublishBlocker{{Code: string(sessionpkg.PublishRevisionChanged)}}}, nil
		}
	}
	if name != nil {
		// A name given with the publication replaces the card's first.
		renamed, err := m.cards.UpdateCard(ctx, userUUID, gameUUID, game.CardChange{Name: name})
		if err != nil {
			return Publication{}, fmt.Errorf("renaming the card: %s", err)
		}
		switch renamed.Outcome {
		case game.CardDone, game.CardUnchanged:
		case game.CardNotFound:
			return Publication{Outcome: PublishNotFound}, nil
		case game.CardInvalid:
			return Publication{Outcome: PublishNotReady, Blockers: []PublishBlocker{{Code: invalidNameBlocker}}}, nil
		default:
			monitoring.Alert(ctx, "game management answered an unknown card outcome")
			return Publication{}, fmt.Errorf("unknown card outcome %q", renamed.Outcome)
		}
	}
	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil {
		return Publication{}, err
	}
	switch editable.Outcome {
	case game.EditableVersionNotFound:
		return Publication{Outcome: PublishNotFound}, nil
	case game.EditableVersionNotEditable:
		return Publication{Outcome: PublishNotEditable}, nil
	}
	readiness, err := m.versions.CheckPublication(ctx, editable.VersionUUID)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		if err := m.repairDraft(ctx, gameUUID, editable); err != nil {
			return Publication{}, err
		}
		readiness, err = m.versions.CheckPublication(ctx, editable.VersionUUID)
	}
	if err != nil {
		return Publication{}, fmt.Errorf("checking the version being built: %s", err)
	}
	switch readiness.Outcome {
	case sessionpkg.PublishReady:
	case sessionpkg.PublishNotReady:
		return Publication{Outcome: PublishNotReady, Blockers: playBlockers(readiness.Blockers)}, nil
	case sessionpkg.PublishNotEditable:
		return Publication{Outcome: PublishNotEditable}, nil
	default:
		monitoring.Alert(ctx, "play answered an unknown publication readiness")
		return Publication{}, fmt.Errorf("unknown publication readiness %q", readiness.Outcome)
	}

	started, err := m.publications.BeginPublication(ctx, userUUID, gameUUID, readiness.Revision, visibility, idempotencyKey)
	if err != nil {
		if errors.Is(err, game.ErrIdempotencyConflict) {
			return Publication{}, err
		}
		return Publication{}, fmt.Errorf("starting the publication: %s", err)
	}
	logging.LogFields(ctx, logging.Field("publication_outcome", string(started.Outcome)))
	switch started.Outcome {
	case game.PublicationNotFound:
		return Publication{Outcome: PublishNotFound}, nil
	case game.PublicationNotEditable:
		return Publication{Outcome: PublishNotEditable}, nil
	case game.PublicationNotReady:
		blockers := make([]PublishBlocker, len(started.Blockers))
		for i, code := range started.Blockers {
			blockers[i] = PublishBlocker{Code: code}
		}
		return Publication{Outcome: PublishNotReady, Blockers: blockers}, nil
	case game.PublicationCompleted:
		return Publication{Outcome: Published, Release: started.Release}, nil
	case game.PublicationAbandoned:
		return Publication{Outcome: PublishNotReady, Release: started.Release, Blockers: []PublishBlocker{{Code: string(sessionpkg.PublishRevisionChanged)}}}, nil
	case game.PublicationStarted, game.PublicationPending:
		if started.Release == nil {
			monitoring.Alert(ctx, "game management started a publication without its release")
			return Publication{}, fmt.Errorf("publication %s has no release", started.Outcome)
		}
		finished, err := m.finishRelease(ctx, *started.Release)
		if err != nil || started.Outcome == game.PublicationStarted {
			return finished, err
		}
		// An earlier release was completed first: this request publishes
		// nothing more, its edition is no longer open.
		return Publication{Outcome: PublishNotEditable}, nil
	default:
		monitoring.Alert(ctx, "game management answered an unknown publication outcome")
		return Publication{}, fmt.Errorf("unknown publication outcome %q", started.Outcome)
	}
}

// finishRelease freezes release's version in Play and ends the release in
// Game Management: completed when Play runs it, abandoned when Play refuses
// it. After completing, it cleans unused stored content, best effort.
func (m *Manager) finishRelease(ctx context.Context, release game.Release) (Publication, error) {
	frozen, err := m.versions.FreezeVersion(ctx, release.GameUUID, release.VersionUUID, release.Revision)
	if err != nil {
		return Publication{}, fmt.Errorf("freezing the version: %s", err)
	}
	switch frozen.Outcome {
	case sessionpkg.FreezeFrozen, sessionpkg.FreezeAlreadyFrozen:
	case sessionpkg.FreezeNotReady:
		blockers := playBlockers(frozen.Blockers)
		codes := make([]string, len(blockers))
		for i, blocker := range blockers {
			codes[i] = blocker.Code
		}
		abandoned, err := m.publications.AbandonPublication(ctx, release.UUID, strings.Join(codes, ","))
		if err != nil {
			return Publication{}, fmt.Errorf("abandoning the release: %s", err)
		}
		return Publication{Outcome: PublishNotReady, Release: &abandoned, Blockers: blockers}, nil
	default:
		monitoring.Alert(ctx, "play answered an unknown freeze outcome")
		return Publication{}, fmt.Errorf("unknown freeze outcome %q", frozen.Outcome)
	}
	completed, err := m.publications.CompletePublication(ctx, release.UUID, frozen.Languages)
	if err != nil {
		return Publication{}, fmt.Errorf("completing the release: %s", err)
	}
	if _, err := m.versions.CleanUnusedObjects(ctx); err != nil {
		cleanupFailuresTotal.Inc(ctx)
		logging.LogError(ctx, err)
	}
	return Publication{Outcome: Published, Release: &completed}, nil
}

// resumePublication finishes userUUID's game's release a crash left being
// published, so its edition is not edited after Play froze its version.
func (m *Manager) resumePublication(ctx context.Context, userUUID string, gameUUID string) error {
	pending, err := m.publications.FindPendingPublication(ctx, userUUID, gameUUID)
	if err != nil {
		return fmt.Errorf("finding a release being published: %s", err)
	}
	if pending == nil {
		return nil
	}
	resumedReleasesTotal.Inc(ctx)
	logging.LogFields(ctx, logging.Field("resumed_release", pending.UUID))
	_, err = m.finishRelease(ctx, *pending)
	return err
}

func playBlockers(blockers []sessionpkg.PublishBlocker) []PublishBlocker {
	answered := make([]PublishBlocker, len(blockers))
	for i, blocker := range blockers {
		answered[i] = PublishBlocker{Code: string(blocker.Code), BuildStatus: string(blocker.BuildStatus), Keys: blocker.Keys}
	}
	return answered
}

// EditionOpening is what OpenEdition decided, with the new version being
// built when opened.
type EditionOpening struct {
	Outcome     game.EditionOutcome
	VersionUUID string
	Revision    int64
}

// OpenEdition opens a new edition of userUUID's published game gameUUID:
// Game Management opens it, Play creates its version being built from the
// published version's sources and assets. The published version keeps
// running Sessions until the new edition is published. A retry with the same
// key resumes where it stopped.
func (m *Manager) OpenEdition(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (EditionOpening, error) {
	defer logging.Step(ctx, "orchestrator.open_edition").Close()

	if err := m.resumePublication(ctx, userUUID, gameUUID); err != nil {
		return EditionOpening{}, err
	}
	opened, err := m.publications.OpenEdition(ctx, userUUID, gameUUID, idempotencyKey)
	if err != nil {
		return EditionOpening{}, fmt.Errorf("opening the edition: %s", err)
	}
	logging.LogFields(ctx, logging.Field("edition_outcome", string(opened.Outcome)))
	switch opened.Outcome {
	case game.EditionOpened:
	case game.EditionNotFound, game.EditionNotPublished, game.EditionAlreadyOpen, game.EditionPublishing:
		return EditionOpening{Outcome: opened.Outcome, VersionUUID: opened.VersionUUID}, nil
	default:
		monitoring.Alert(ctx, "game management answered an unknown edition outcome")
		return EditionOpening{}, fmt.Errorf("unknown edition outcome %q", opened.Outcome)
	}
	if opened.VersionUUID == "" || opened.BaseVersionUUID == "" {
		monitoring.Alert(ctx, "game management opened an edition without its versions")
		return EditionOpening{}, fmt.Errorf("edition of game %s opened without its versions", gameUUID)
	}
	draft, err := m.createDraft(ctx, gameUUID, opened.VersionUUID, opened.BaseVersionUUID)
	if err != nil {
		return EditionOpening{}, err
	}
	return EditionOpening{Outcome: game.EditionOpened, VersionUUID: opened.VersionUUID, Revision: draft.Revision}, nil
}
