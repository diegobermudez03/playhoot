// Package gameauthoring groups the cross-domain write operations that create a
// game and change the version being built for it: Game Management owns the
// game and decides who may edit it, Play owns the version being built.
package gameauthoring

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/monitoring"
)

// Manager coordinates the game-authoring operations. Each operation lives in
// its own file and declares its own narrow interfaces for the capabilities it
// calls. It owns no business entity.
type Manager struct {
	games        gameCreator
	editions     editableVersionFinder
	drafts       draftWriter
	links        resourceLinkWriter
	previews     previewStarter
	publications gamePublisher
	versions     versionPublisher
	cards        cardNamer
}

// New constructs a Manager from Game Management's game creation, edit
// authorization, releases and card; Play's version being built, previews and
// freezing; and Identity's links an AI uses for a game.
func New(games gameCreator, editions editableVersionFinder, drafts draftWriter, links resourceLinkWriter, previews previewStarter, publications gamePublisher, versions versionPublisher, cards cardNamer) *Manager {
	return &Manager{games: games, editions: editions, drafts: drafts, links: links, previews: previews, publications: publications, versions: versions, cards: cards}
}

// editableVersionFinder is Game Management's decision of whether a user may
// edit a game, answering the uuid of the version being built when they may.
type editableVersionFinder interface {
	FindEditableVersion(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersion, error)
}

// editableVersion asks Game Management for the version userUUID may edit. A
// found outcome without a version, or an unknown outcome, is broken data.
func (m *Manager) editableVersion(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersion, error) {
	editable, err := m.editions.FindEditableVersion(ctx, gameUUID, userUUID)
	if err != nil {
		return game.EditableVersion{}, fmt.Errorf("finding the editable version: %s", err)
	}
	switch editable.Outcome {
	case game.EditableVersionFound:
		if editable.VersionUUID == "" {
			monitoring.Alert(ctx, "game management found an editable version without its uuid")
			return game.EditableVersion{}, fmt.Errorf("game %s has an editable version without uuid", gameUUID)
		}
	case game.EditableVersionNotFound, game.EditableVersionNotEditable:
	default:
		monitoring.Alert(ctx, "game management answered an unknown editable version outcome")
		return game.EditableVersion{}, fmt.Errorf("unknown editable version outcome %q", editable.Outcome)
	}
	return editable, nil
}
