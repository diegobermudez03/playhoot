package gameauthoring

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameauthoring -destination=create_game_mock_test.go . gameCreator,editableVersionFinder,draftWriter

// gameCreator is Game Management's creation of an unpublished game with its
// first edition, idempotent per owner and key.
type gameCreator interface {
	CreateGame(ctx context.Context, ownerUUID string, name string, idempotencyKey string) (game.CreatedGame, error)
}

// draftWriter is Play's version being built: its creation, under the uuid
// Game Management chose for the edition, and its changes.
type draftWriter interface {
	CreateDraftVersion(ctx context.Context, gameUUID string, versionUUID string, fromVersionUUID string) (sessionpkg.DraftVersion, error)
	ApplyChanges(ctx context.Context, versionUUID string, change sessionpkg.DraftChange) (sessionpkg.DraftChangeResult, error)
	RestoreRevision(ctx context.Context, versionUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (sessionpkg.DraftChangeResult, error)
	UploadAsset(ctx context.Context, versionUUID string, key string, content io.Reader, idempotencyKey string, origin string) (sessionpkg.DraftAssetResult, error)
	RemoveAsset(ctx context.Context, versionUUID string, key string, idempotencyKey string, origin string) (sessionpkg.DraftAssetResult, error)
}

// CreatedGame is a new game with its version being built, at its first
// revision.
type CreatedGame struct {
	GameUUID    string
	VersionUUID string
	Revision    int64
}

// CreateGame creates a game for userUUID in Game Management, then the version
// being built for it in Play. Both steps are idempotent: a retry with the same
// key completes the same game, and any later edit or read of the game creates
// the missing version (EnsureDraft).
func (m *Manager) CreateGame(ctx context.Context, userUUID string, name string, idempotencyKey string) (CreatedGame, error) {
	defer logging.Step(ctx, "orchestrator.create_game").Close()

	created, err := m.games.CreateGame(ctx, userUUID, name, idempotencyKey)
	if err != nil {
		if errors.Is(err, game.ErrInvalidCard) || errors.Is(err, game.ErrIdempotencyConflict) {
			return CreatedGame{}, err
		}
		return CreatedGame{}, fmt.Errorf("creating the game: %s", err)
	}
	logging.LogFields(ctx, logging.Field("game_uuid", created.GameUUID), logging.Field("version_uuid", created.EditionVersionUUID))

	draft, err := m.createDraft(ctx, created.GameUUID, created.EditionVersionUUID, "")
	if err != nil {
		return CreatedGame{}, err
	}
	return CreatedGame{GameUUID: created.GameUUID, VersionUUID: created.EditionVersionUUID, Revision: draft.Revision}, nil
}
