package gameauthoring

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestApplyDraftChanges(t *testing.T) {
	type test struct {
		expected        DraftEdit
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	change := sessionpkg.DraftChange{BaseRevision: 1, Files: []sessionpkg.DraftFileContent{{Path: "notes.md", Content: "x"}}, IdempotencyKey: testKey, Origin: sessionpkg.DraftOriginWeb}
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}
	applied := sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"the_owner_changes_the_version_being_built": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(applied, nil)
			return test{expected: DraftEdit{Access: game.EditableVersionFound, Result: applied}}
		},
		"not_found_never_reaches_play": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)
			return test{expected: DraftEdit{Access: game.EditableVersionNotFound}}
		},
		"not_editable_never_reaches_play": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotEditable}, nil)
			return test{expected: DraftEdit{Access: game.EditableVersionNotEditable}}
		},
		"unknown_access_outcome_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: "maybe"}, nil)
			return test{expectErr: true}
		},
		"found_without_version_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionFound}, nil)
			return test{expectErr: true}
		},
		"a_version_play_does_not_have_yet_is_created_and_the_change_applied": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			gomock.InOrder(
				deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(sessionpkg.DraftChangeResult{}, sessionpkg.ErrDraftNotFound),
				deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
				deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(applied, nil),
			)
			return test{expected: DraftEdit{Access: game.EditableVersionFound, Result: applied}}
		},
		"a_failed_repair_is_an_error": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(sessionpkg.DraftChangeResult{}, sessionpkg.ErrDraftNotFound)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{}, errors.New("database down"))
			return test{expectErr: true}
		},
		"another_version_being_built_in_play_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(sessionpkg.DraftChangeResult{}, sessionpkg.ErrDraftNotFound)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, VersionUUID: "other"}, nil)
			return test{expectErr: true}
		},
		"still_missing_after_the_repair_is_an_error": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(sessionpkg.DraftChangeResult{}, sessionpkg.ErrDraftNotFound).Times(2)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated}, nil)
			return test{expectErr: true}
		},
		"a_reused_key_keeps_its_error": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ApplyChanges(ctx, testVersionUUID, change).Return(sessionpkg.DraftChangeResult{}, sessionpkg.ErrIdempotencyConflict)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrIdempotencyConflict}
		},
		"authorization_fails": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{}, errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			manager, deps := newTestManager(t)
			tc := setup(t, deps)

			result, err := manager.ApplyDraftChanges(ctx, testUserUUID, testGameUUID, change)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestRestoreDraftRevision(t *testing.T) {
	ctx := context.Background()
	restored := sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 3}

	t.Run("the_owner_restores_a_revision", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}, nil)
		deps.drafts.EXPECT().RestoreRevision(ctx, testVersionUUID, int64(1), int64(2), testKey, sessionpkg.DraftOriginWeb).Return(restored, nil)

		result, err := manager.RestoreDraftRevision(ctx, testUserUUID, testGameUUID, 1, 2, testKey, sessionpkg.DraftOriginWeb)

		require.NoError(t, err)
		require.Equal(t, DraftEdit{Access: game.EditableVersionFound, Result: restored}, result)
	})

	t.Run("not_found_never_reaches_play", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		result, err := manager.RestoreDraftRevision(ctx, testUserUUID, testGameUUID, 1, 2, testKey, sessionpkg.DraftOriginWeb)

		require.NoError(t, err)
		require.Equal(t, DraftEdit{Access: game.EditableVersionNotFound}, result)
	})
}

func TestEnsureDraft(t *testing.T) {
	ctx := context.Background()
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}

	t.Run("a_missing_version_is_created", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated}, nil)

		access, err := manager.EnsureDraft(ctx, testUserUUID, testGameUUID)

		require.NoError(t, err)
		require.Equal(t, game.EditableVersionFound, access)
	})

	t.Run("a_present_version_is_kept", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated}, nil)

		access, err := manager.EnsureDraft(ctx, testUserUUID, testGameUUID)

		require.NoError(t, err)
		require.Equal(t, game.EditableVersionFound, access)
	})

	t.Run("a_caller_who_may_not_edit_never_reaches_play", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		access, err := manager.EnsureDraft(ctx, testUserUUID, testGameUUID)

		require.NoError(t, err)
		require.Equal(t, game.EditableVersionNotFound, access)
	})

	t.Run("another_version_being_built_is_broken_data", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, VersionUUID: "other"}, nil)

		_, err := manager.EnsureDraft(ctx, testUserUUID, testGameUUID)

		require.Error(t, err)
	})
}
