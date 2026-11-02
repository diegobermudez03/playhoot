package findeditableversion

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/usecases/findeditableversion/internal/repo"
)

func TestFindEditableVersion(t *testing.T) {
	type svcMocks struct {
		repo *MockrepoAPI
	}
	type test struct {
		gameUUID  string
		userUUID  string
		expected  game.EditableVersion
		expectErr bool
	}

	const (
		gameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		ownerUUID   = "0f8fad5b-d9cb-469f-a165-70867728950e"
		otherUUID   = "16fd2706-8baf-433b-82eb-8c7fada847da"
		versionUUID = "6ba7b810-9dad-41d1-80b4-00c04fd430c8"
	)
	ctx := context.Background()
	version := versionUUID

	tests := map[string]func(t *testing.T, mocks svcMocks) test{
		"owner_edits_the_open_edition": func(t *testing.T, mocks svcMocks) test {
			mocks.repo.EXPECT().FindGameEditing(ctx, gameUUID, "EDITING").Return(&repo.GameEditing{OwnerUUID: ownerUUID, VersionUUID: &version}, nil)
			return test{expected: game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: versionUUID}}
		},
		"owner_uuid_in_another_textual_form_is_the_owner": func(t *testing.T, mocks svcMocks) test {
			mocks.repo.EXPECT().FindGameEditing(ctx, gameUUID, "EDITING").Return(&repo.GameEditing{OwnerUUID: ownerUUID, VersionUUID: &version}, nil)
			return test{userUUID: "0F8FAD5B-D9CB-469F-A165-70867728950E", expected: game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: versionUUID}}
		},
		"owner_without_an_open_edition": func(t *testing.T, mocks svcMocks) test {
			mocks.repo.EXPECT().FindGameEditing(ctx, gameUUID, "EDITING").Return(&repo.GameEditing{OwnerUUID: ownerUUID}, nil)
			return test{expected: game.EditableVersion{Outcome: game.EditableVersionNotEditable}}
		},
		"someone_elses_game_is_not_found": func(t *testing.T, mocks svcMocks) test {
			mocks.repo.EXPECT().FindGameEditing(ctx, gameUUID, "EDITING").Return(&repo.GameEditing{OwnerUUID: otherUUID, VersionUUID: &version}, nil)
			return test{expected: game.EditableVersion{Outcome: game.EditableVersionNotFound}}
		},
		"unknown_game_is_not_found": func(t *testing.T, mocks svcMocks) test {
			mocks.repo.EXPECT().FindGameEditing(ctx, gameUUID, "EDITING").Return(nil, nil)
			return test{expected: game.EditableVersion{Outcome: game.EditableVersionNotFound}}
		},
		"malformed_game_uuid_is_not_found": func(t *testing.T, mocks svcMocks) test {
			return test{gameUUID: "nope", expected: game.EditableVersion{Outcome: game.EditableVersionNotFound}}
		},
		"malformed_user_uuid_is_not_found": func(t *testing.T, mocks svcMocks) test {
			return test{userUUID: "nope", expected: game.EditableVersion{Outcome: game.EditableVersionNotFound}}
		},
		"reading_fails": func(t *testing.T, mocks svcMocks) test {
			mocks.repo.EXPECT().FindGameEditing(ctx, gameUUID, "EDITING").Return(nil, errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := svcMocks{repo: NewMockrepoAPI(ctrl)}
			service := &Service{repo: mocks.repo}
			tc := setup(t, mocks)
			if tc.gameUUID == "" {
				tc.gameUUID = gameUUID
			}
			if tc.userUUID == "" {
				tc.userUUID = ownerUUID
			}

			answer, err := service.FindEditableVersion(ctx, tc.gameUUID, tc.userUUID)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, answer)
		})
	}
}
