package gameauthoring

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=gameauthoring -destination=ai_link_mock_test.go . resourceLinkWriter

// resourceLinkWriter is Identity's secret links to one resource, here a game.
type resourceLinkWriter interface {
	CreateResourceLink(ctx context.Context, userUUID string, kind string, resourceID string) (identity.CreatedResourceLink, error)
	RevokeResourceLink(ctx context.Context, userUUID string, kind string, resourceID string) error
}

// AILinkCreation is a new link for an AI to a game: Access is Game
// Management's decision, and Link is set only when the user owns the game.
type AILinkCreation struct {
	Access game.EditableVersionOutcome
	Link   identity.CreatedResourceLink
}

// CreateAILink creates the link an AI uses to read and change gameUUID for
// userUUID, replacing the previous one. Only the game's owner may, whether or
// not it has an open edition, so its links can be managed after publishing.
func (m *Manager) CreateAILink(ctx context.Context, userUUID string, gameUUID string) (AILinkCreation, error) {
	defer logging.Step(ctx, "orchestrator.create_ai_link").Close()

	access, err := m.ownerAccess(ctx, gameUUID, userUUID)
	if err != nil || access == game.EditableVersionNotFound {
		return AILinkCreation{Access: access}, err
	}
	link, err := m.links.CreateResourceLink(ctx, userUUID, game.ResourceKind, gameUUID)
	if err != nil {
		return AILinkCreation{}, fmt.Errorf("creating the AI link: %s", err)
	}
	return AILinkCreation{Access: access, Link: link}, nil
}

// RevokeAILink revokes the link an AI uses for gameUUID, answering Game
// Management's decision; revoking when there is none does nothing.
func (m *Manager) RevokeAILink(ctx context.Context, userUUID string, gameUUID string) (game.EditableVersionOutcome, error) {
	defer logging.Step(ctx, "orchestrator.revoke_ai_link").Close()

	access, err := m.ownerAccess(ctx, gameUUID, userUUID)
	if err != nil || access == game.EditableVersionNotFound {
		return access, err
	}
	if err := m.links.RevokeResourceLink(ctx, userUUID, game.ResourceKind, gameUUID); err != nil {
		return "", fmt.Errorf("revoking the AI link: %s", err)
	}
	return access, nil
}

// ownerAccess answers whether userUUID owns gameUUID: Game Management finds
// an editable version, or none open, only for the game's owner.
func (m *Manager) ownerAccess(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersionOutcome, error) {
	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil {
		return "", err
	}
	logging.LogFields(ctx, logging.Field("edit_access", string(editable.Outcome)))
	return editable.Outcome, nil
}
