package findtemplate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/usecases/findtemplate/internal/repo"
)

func TestFindTemplate(t *testing.T) {
	const gameUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()
	version := "0f8fad5b-d9cb-469f-a165-70867728950e"

	t.Run("a_published_game_allowed_as_a_template", func(t *testing.T) {
		mock := NewMockrepoAPI(gomock.NewController(t))
		mock.EXPECT().FindTemplate(ctx, gameUUID, "PUBLISHED").Return(&repo.Template{GameUUID: gameUUID, VersionUUID: &version, Name: "Quiz", Description: "A quiz", Featured: true}, nil)

		found, err := (&Service{repo: mock}).FindTemplate(ctx, gameUUID)

		require.NoError(t, err)
		require.Equal(t, game.Template{Outcome: game.TemplateFound, GameUUID: gameUUID, VersionUUID: version, Name: "Quiz", Description: "A quiz", Featured: true}, found)
	})

	t.Run("anything_else_is_not_found", func(t *testing.T) {
		mock := NewMockrepoAPI(gomock.NewController(t))
		mock.EXPECT().FindTemplate(ctx, gameUUID, "PUBLISHED").Return(nil, nil)
		service := &Service{repo: mock}

		found, err := service.FindTemplate(ctx, gameUUID)
		require.NoError(t, err)
		require.Equal(t, game.TemplateNotFound, found.Outcome)

		found, err = service.FindTemplate(ctx, "not-a-uuid")
		require.NoError(t, err)
		require.Equal(t, game.TemplateNotFound, found.Outcome)
	})
}
