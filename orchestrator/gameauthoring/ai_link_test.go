package gameauthoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
)

func TestCreateAILink(t *testing.T) {
	ctx := context.Background()
	link := identity.CreatedResourceLink{Token: "phl_x", CreatedAt: time.Unix(10, 0), ExpiresAt: time.Unix(20, 0)}

	for _, access := range []game.EditableVersion{
		{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID},
		{Outcome: game.EditableVersionNotEditable},
	} {
		t.Run("the owner gets a link: "+string(access.Outcome), func(t *testing.T) {
			m, deps := newTestManager(t)
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(access, nil)
			deps.links.EXPECT().CreateResourceLink(ctx, testUserUUID, game.ResourceKind, testGameUUID).Return(link, nil)

			created, err := m.CreateAILink(ctx, testUserUUID, testGameUUID)
			require.NoError(t, err)
			require.Equal(t, AILinkCreation{Access: access.Outcome, Link: link}, created)
		})
	}

	t.Run("anyone else gets nothing", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		created, err := m.CreateAILink(ctx, testUserUUID, testGameUUID)
		require.NoError(t, err)
		require.Equal(t, AILinkCreation{Access: game.EditableVersionNotFound}, created)
	})

	t.Run("identity failing is an error", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotEditable}, nil)
		deps.links.EXPECT().CreateResourceLink(ctx, testUserUUID, game.ResourceKind, testGameUUID).Return(identity.CreatedResourceLink{}, errors.New("down"))

		_, err := m.CreateAILink(ctx, testUserUUID, testGameUUID)
		require.Error(t, err)
	})
}

func TestRevokeAILink(t *testing.T) {
	ctx := context.Background()

	t.Run("the owner revokes", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}, nil)
		deps.links.EXPECT().RevokeResourceLink(ctx, testUserUUID, game.ResourceKind, testGameUUID).Return(nil)

		access, err := m.RevokeAILink(ctx, testUserUUID, testGameUUID)
		require.NoError(t, err)
		require.Equal(t, game.EditableVersionFound, access)
	})

	t.Run("anyone else revokes nothing", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		access, err := m.RevokeAILink(ctx, testUserUUID, testGameUUID)
		require.NoError(t, err)
		require.Equal(t, game.EditableVersionNotFound, access)
	})
}
