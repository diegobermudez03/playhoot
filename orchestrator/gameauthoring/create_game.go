package gameauthoring

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameauthoring -destination=create_game_mock_test.go . gameCreator,editableVersionFinder,draftWriter

// gameCreator is Game Management's creation of an unpublished game with its
// first edition, idempotent per owner and key.
type gameCreator interface {
	CreateGame(ctx context.Context, ownerUUID string, name string, idempotencyKey string, template *game.TemplateSource) (game.CreatedGame, error)
}

// draftWriter is Play's version being built: its creation, under the uuid
// Game Management chose for the edition, and its changes.
type draftWriter interface {
	CreateDraftVersion(ctx context.Context, gameUUID string, versionUUID string, start sessionpkg.DraftStart) (sessionpkg.DraftVersion, error)
	ApplyChanges(ctx context.Context, versionUUID string, change sessionpkg.DraftChange) (sessionpkg.DraftChangeResult, error)
	RestoreRevision(ctx context.Context, versionUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (sessionpkg.DraftChangeResult, error)
	ApplyTemplate(ctx context.Context, versionUUID string, template sessionpkg.DraftTemplate, baseRevision int64, idempotencyKey string, origin string) (sessionpkg.DraftChangeResult, error)
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

// CreateGame gates the caller on the market matrix (approximateCountry is
// what their address suggests), creates their game in Game Management, then
// its version being built in Play: the starter project, or a copy of
// templateGameUUID. Idempotent per key.
func (m *Manager) CreateGame(ctx context.Context, userUUID string, name string, idempotencyKey string, templateGameUUID string, approximateCountry string) (CreatedGame, error) {
	defer logging.Step(ctx, "orchestrator.create_game").Close()

	if err := m.market.Require(ctx, userUUID, compliance.FunctionCreateGames, approximateCountry); err != nil {
		return CreatedGame{}, err
	}

	var template *game.TemplateSource
	start := sessionpkg.DraftStart{}
	if templateGameUUID != "" {
		source, err := m.template(ctx, templateGameUUID)
		if err != nil {
			return CreatedGame{}, err
		}
		template = &source
		start = sessionpkg.DraftStart{FromVersionUUID: source.VersionUUID, TemplateGameUUID: source.GameUUID}
	}
	created, err := m.games.CreateGame(ctx, userUUID, name, idempotencyKey, template)
	if err != nil {
		if errors.Is(err, game.ErrInvalidCard) || errors.Is(err, game.ErrIdempotencyConflict) {
			return CreatedGame{}, err
		}
		return CreatedGame{}, fmt.Errorf("creating the game: %s", err)
	}
	logging.LogFields(ctx, logging.Field("game_uuid", created.GameUUID), logging.Field("version_uuid", created.EditionVersionUUID))

	draft, err := m.createDraft(ctx, created.GameUUID, created.EditionVersionUUID, start)
	if err != nil {
		return CreatedGame{}, err
	}
	return CreatedGame{GameUUID: created.GameUUID, VersionUUID: created.EditionVersionUUID, Revision: draft.Revision}, nil
}
