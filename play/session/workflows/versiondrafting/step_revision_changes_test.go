package versiondrafting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestRevisionChanges(t *testing.T) {
	ctx := context.Background()
	const versionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	current := int64(3)
	version := &internalrepo.Version{ID: 7, UUID: versionUUID, Status: "DRAFT", CurrentRevision: &current}
	revision1 := draftsources.Revision{Files: map[string]draftsources.File{
		"game.project.json": {SHA256: "p", Size: 10},
		"server/game.ts":    {SHA256: "s1", Size: 20},
		"shared/old.ts":     {SHA256: "o", Size: 5},
	}}
	revision3 := draftsources.Revision{Files: map[string]draftsources.File{
		"game.project.json": {SHA256: "p", Size: 10},
		"server/game.ts":    {SHA256: "s2", Size: 21},
		"shared/new.ts":     {SHA256: "n", Size: 7},
	}}
	files3 := []sessionpkg.DraftFile{
		{Path: "game.project.json", SHA256: "p", Size: 10},
		{Path: "server/game.ts", SHA256: "s2", Size: 21},
		{Path: "shared/new.ts", SHA256: "n", Size: 7},
	}

	testCases := []struct {
		name            string
		since           int64
		upTo            int64
		setup           func(repo *MockrevisionChangesRepo)
		expected        sessionpkg.DraftRevisionChanges
		expectedErrorIs error
	}{
		{
			name: "between two revisions",
			upTo: 3, since: 1,
			setup: func(repo *MockrevisionChangesRepo) {
				repo.EXPECT().FindRevision(ctx, gomock.Any(), uint(7), int64(3)).Return(revision3, true, nil)
				repo.EXPECT().FindRevision(ctx, gomock.Any(), uint(7), int64(1)).Return(revision1, true, nil)
			},
			expected: sessionpkg.DraftRevisionChanges{
				Since: 1, Revision: 3, Files: files3,
				Added: []string{"shared/new.ts"}, Modified: []string{"server/game.ts"}, Deleted: []string{"shared/old.ts"},
			},
		},
		{
			name: "up to the current revision when 0, everything added since 0",
			setup: func(repo *MockrevisionChangesRepo) {
				repo.EXPECT().FindRevision(ctx, gomock.Any(), uint(7), int64(3)).Return(revision3, true, nil)
			},
			expected: sessionpkg.DraftRevisionChanges{
				Since: 0, Revision: 3, Files: files3,
				Added: []string{"game.project.json", "server/game.ts", "shared/new.ts"}, Modified: []string{}, Deleted: []string{},
			},
		},
		{
			name: "the same revision changes nothing",
			upTo: 3, since: 3,
			setup: func(repo *MockrevisionChangesRepo) {
				repo.EXPECT().FindRevision(ctx, gomock.Any(), uint(7), int64(3)).Return(revision3, true, nil).Times(2)
			},
			expected: sessionpkg.DraftRevisionChanges{Since: 3, Revision: 3, Files: files3, Added: []string{}, Modified: []string{}, Deleted: []string{}},
		},
		{name: "a revision past the current one", upTo: 4, setup: func(*MockrevisionChangesRepo) {}, expectedErrorIs: sessionpkg.ErrDraftRevisionNotFound},
		{name: "since past up to", upTo: 2, since: 3, setup: func(*MockrevisionChangesRepo) {}, expectedErrorIs: sessionpkg.ErrDraftRevisionNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockrevisionChangesRepo(ctrl)
			repo.EXPECT().FindVersion(ctx, gomock.Any(), versionUUID).Return(version, nil)
			tc.setup(repo)
			m := &Manager{revisionChangesRepo: repo, dbServicer: nilDB{}}

			got, err := m.revisionChangesInTx(ctx, nil, versionUUID, tc.since, tc.upTo)

			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestRevisionChangesOfAMissingVersion(t *testing.T) {
	ctx := context.Background()
	const versionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	repo := NewMockrevisionChangesRepo(gomock.NewController(t))
	repo.EXPECT().FindVersion(ctx, gomock.Any(), versionUUID).Return(nil, nil)

	_, err := (&Manager{revisionChangesRepo: repo}).revisionChangesInTx(ctx, nil, versionUUID, 0, 0)

	require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
}
