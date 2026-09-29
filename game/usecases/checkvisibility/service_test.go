package checkvisibility

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestServiceIsVisible_Public(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockrepoAPI(ctrl)
	visibility := "public"
	repo.EXPECT().ResolveVisibility(gomock.Any(), "game-1").Return(&visibility, nil)

	svc := &Service{repo: repo}
	visible, found, err := svc.IsVisible(context.Background(), "game-1")
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, visible)
}

func TestServiceIsVisible_Hidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockrepoAPI(ctrl)
	visibility := "hidden"
	repo.EXPECT().ResolveVisibility(gomock.Any(), "game-1").Return(&visibility, nil)

	svc := &Service{repo: repo}
	visible, found, err := svc.IsVisible(context.Background(), "game-1")
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, visible, "Hidden is playable, per businessservice.IsPlayableVisibility")
}

func TestServiceIsVisible_NotPlayable(t *testing.T) {
	for _, visibility := range []string{"draft", "private"} {
		t.Run(visibility, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockrepoAPI(ctrl)
			v := visibility
			repo.EXPECT().ResolveVisibility(gomock.Any(), "game-1").Return(&v, nil)

			svc := &Service{repo: repo}
			visible, found, err := svc.IsVisible(context.Background(), "game-1")
			require.NoError(t, err)
			require.True(t, found)
			require.False(t, visible)
		})
	}
}

func TestServiceIsVisible_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockrepoAPI(ctrl)
	repo.EXPECT().ResolveVisibility(gomock.Any(), "missing").Return(nil, nil)

	svc := &Service{repo: repo}
	visible, found, err := svc.IsVisible(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, found)
	require.False(t, visible)
}

func TestServiceIsVisible_RepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMockrepoAPI(ctrl)
	repoErr := errors.New("db unavailable")
	repo.EXPECT().ResolveVisibility(gomock.Any(), "game-1").Return(nil, repoErr)

	svc := &Service{repo: repo}
	_, _, err := svc.IsVisible(context.Background(), "game-1")
	require.ErrorIs(t, err, repoErr)
}
