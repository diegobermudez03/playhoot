package checkplayability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestIsPlayable(t *testing.T) {
	const (
		gameUUID  = "6ba7b810-9dad-41d1-80b4-00c04fd430c8"
		ownerUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		otherUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)
	published := time.Now()
	tests := map[string]struct {
		found    *Playability
		user     string
		expected bool
	}{
		"public_published_for_anyone":   {found: &Playability{OwnerUUID: ownerUUID, Visibility: "public", PublishedAt: &published}, user: otherUUID, expected: true},
		"private_published_for_owner":   {found: &Playability{OwnerUUID: ownerUUID, Visibility: "private", PublishedAt: &published}, user: "0F8FAD5B-D9CB-469F-A165-70867728950E", expected: true},
		"private_published_for_others":  {found: &Playability{OwnerUUID: ownerUUID, Visibility: "private", PublishedAt: &published}, user: otherUUID},
		"unpublished_even_for_owner":    {found: &Playability{OwnerUUID: ownerUUID, Visibility: "public"}, user: ownerUUID},
		"deleted_or_missing":            {user: ownerUUID},
		"unknown_visibility_is_refused": {found: &Playability{OwnerUUID: ownerUUID, Visibility: "draft", PublishedAt: &published}, user: ownerUUID},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockrepoAPI(ctrl)
			repo.EXPECT().FindPlayability(gomock.Any(), gameUUID).Return(tc.found, nil)
			playable, err := (&Service{repo: repo}).IsPlayable(context.Background(), gameUUID, tc.user)
			require.NoError(t, err)
			require.Equal(t, tc.expected, playable)
		})
	}

	t.Run("a malformed game uuid is not playable", func(t *testing.T) {
		playable, err := (&Service{}).IsPlayable(context.Background(), "nope", ownerUUID)
		require.NoError(t, err)
		require.False(t, playable)
	})
	t.Run("a read failure is an error", func(t *testing.T) {
		repo := NewMockrepoAPI(gomock.NewController(t))
		repo.EXPECT().FindPlayability(gomock.Any(), gameUUID).Return(nil, errors.New("database down"))
		_, err := (&Service{repo: repo}).IsPlayable(context.Background(), gameUUID, ownerUUID)
		require.Error(t, err)
	})
}
