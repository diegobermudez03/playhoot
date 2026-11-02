package versiondrafting

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestCreateDraftVersionInTx(t *testing.T) {
	type dependencies struct {
		repo *MockcreateDraftVersionRepo
	}
	type test struct {
		expected        sessionpkg.DraftVersion
		expectErr       bool
		expectedErrorIs error
	}

	const (
		gameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		otherGame   = "16fd2706-8baf-433b-82eb-8c7fada847da"
		versionUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		openUUID    = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
		gameID      = uint(7)
		versionID   = uint(21)
	)
	ctx := context.Background()
	tx := &gorm.DB{}
	revision := func(n int64) *int64 { return &n }

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"created_with_the_starter_template": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(gameID, nil),
				deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(nil, nil),
				deps.repo.EXPECT().FindVersionOfGameInStatus(ctx, tx, gameID, "DRAFT").Return(nil, nil),
				deps.repo.EXPECT().CreateVersion(ctx, tx, gameID, versionUUID, "DRAFT", "sdk-v1", int64(1)).Return(versionID, nil),
				deps.repo.EXPECT().StoreSourceContents(ctx, tx, gomock.Len(5)).Return(nil),
				deps.repo.EXPECT().CreateRevision(ctx, tx, versionID, int64(1), revisionWithFiles(5), "template").Return(nil),
			)
			return test{expected: sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}}
		},
		"same_draft_again_is_already_created": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(gameID, nil),
				deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(&internalrepo.Version{ID: versionID, UUID: versionUUID, GameUUID: gameUUID, Status: "DRAFT", CurrentRevision: revision(4)}, nil),
			)
			return test{expected: sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 4}}
		},
		"version_of_another_game_is_a_conflict": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(gameID, nil),
				deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(&internalrepo.Version{ID: versionID, UUID: versionUUID, GameUUID: otherGame, Status: "DRAFT", CurrentRevision: revision(1)}, nil),
			)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrDraftVersionConflict}
		},
		"version_no_longer_a_draft_is_a_conflict": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(gameID, nil),
				deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(&internalrepo.Version{ID: versionID, UUID: versionUUID, GameUUID: gameUUID, Status: "EXECUTABLE", CurrentRevision: revision(3)}, nil),
			)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrDraftVersionConflict}
		},
		"another_draft_already_open": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(gameID, nil),
				deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(nil, nil),
				deps.repo.EXPECT().FindVersionOfGameInStatus(ctx, tx, gameID, "DRAFT").Return(&internalrepo.Version{ID: 9, UUID: openUUID, GameUUID: gameUUID, Status: "DRAFT", CurrentRevision: revision(2)}, nil),
			)
			return test{expected: sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, GameUUID: gameUUID, VersionUUID: openUUID, Revision: 2}}
		},
		"draft_without_revision_is_broken_data": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(gameID, nil),
				deps.repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(&internalrepo.Version{ID: versionID, UUID: versionUUID, GameUUID: gameUUID, Status: "DRAFT"}, nil),
			)
			return test{expectErr: true}
		},
		"locking_the_game_fails": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(uint(0), errors.New("database down"))
			return test{expectErr: true}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcreateDraftVersionRepo(ctrl)}
			tc := setup(t, deps)
			m := &Manager{createDraftVersionRepo: deps.repo}

			got, err := m.createDraftVersionInTx(ctx, tx, gameUUID, versionUUID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}
