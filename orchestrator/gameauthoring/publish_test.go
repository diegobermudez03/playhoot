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

// newPublishTestManager is newTestManager without a default answer for
// releases being published, which these tests decide.
func newPublishTestManager(t *testing.T) (*Manager, dependencies) {
	ctrl := gomock.NewController(t)
	deps := dependencies{games: NewMockgameCreator(ctrl), editions: NewMockeditableVersionFinder(ctrl), drafts: NewMockdraftWriter(ctrl), links: NewMockresourceLinkWriter(ctrl), previews: NewMockpreviewStarter(ctrl), publications: NewMockgamePublisher(ctrl), versions: NewMockversionPublisher(ctrl), cards: NewMockcardNamer(ctrl)}
	deps.publications.EXPECT().FindPublication(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	return New(deps.games, deps.editions, deps.drafts, deps.links, deps.previews, deps.publications, deps.versions, deps.cards), deps
}

func TestPublish(t *testing.T) {
	ctx := context.Background()
	editable := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}
	release := game.Release{UUID: "11111111-0000-4000-8000-000000000001", GameUUID: testGameUUID, Sequence: 1, VersionUUID: testVersionUUID, Revision: 3, Visibility: game.Private}

	tests := map[string]struct {
		setup    func(deps dependencies)
		expected Publication
		wantErr  bool
	}{
		"publishes_the_current_revision": {
			setup: func(deps dependencies) {
				gomock.InOrder(
					deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil),
					deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil),
					deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: 3}, nil),
					deps.publications.EXPECT().BeginPublication(ctx, testUserUUID, testGameUUID, int64(3), game.Private, testKey).Return(game.Publication{Outcome: game.PublicationStarted, Release: &release}, nil),
					deps.versions.EXPECT().FreezeVersion(ctx, testGameUUID, testVersionUUID, int64(3)).Return(sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeFrozen, Revision: 3, Languages: []string{"es", "en"}}, nil),
					deps.publications.EXPECT().CompletePublication(ctx, release.UUID, []string{"es", "en"}).Return(release, nil),
					deps.versions.EXPECT().CleanUnusedObjects(ctx).Return(sessionpkg.CleanedObjects{Assets: 1}, nil),
				)
			},
			expected: Publication{Outcome: Published, Release: &release},
		},
		"a_failed_cleanup_does_not_fail_the_publication": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil)
				deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: 3}, nil)
				deps.publications.EXPECT().BeginPublication(ctx, testUserUUID, testGameUUID, int64(3), game.Private, testKey).Return(game.Publication{Outcome: game.PublicationStarted, Release: &release}, nil)
				deps.versions.EXPECT().FreezeVersion(ctx, testGameUUID, testVersionUUID, int64(3)).Return(sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeFrozen, Revision: 3, Languages: []string{"es", "en"}}, nil)
				deps.publications.EXPECT().CompletePublication(ctx, release.UUID, []string{"es", "en"}).Return(release, nil)
				deps.versions.EXPECT().CleanUnusedObjects(ctx).Return(sessionpkg.CleanedObjects{}, errors.New("storage down"))
			},
			expected: Publication{Outcome: Published, Release: &release},
		},
		"play_not_ready_answers_its_blockers_and_records_nothing": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil)
				deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishNotReady, Revision: 3, Blockers: []sessionpkg.PublishBlocker{
					{Code: sessionpkg.PublishBuildNotSucceeded, BuildStatus: "FAILED"},
					{Code: sessionpkg.PublishMissingRequiredAssets, Keys: []string{"cardBack"}},
				}}, nil)
			},
			expected: Publication{Outcome: PublishNotReady, Blockers: []PublishBlocker{
				{Code: "build_not_succeeded", BuildStatus: "FAILED"},
				{Code: "missing_required_assets", Keys: []string{"cardBack"}},
			}},
		},
		"the_card_without_a_name_is_not_ready": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil)
				deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: 3}, nil)
				deps.publications.EXPECT().BeginPublication(ctx, testUserUUID, testGameUUID, int64(3), game.Private, testKey).Return(game.Publication{Outcome: game.PublicationNotReady, Blockers: []string{"name_required"}}, nil)
			},
			expected: Publication{Outcome: PublishNotReady, Blockers: []PublishBlocker{{Code: "name_required"}}},
		},
		"a_revision_moved_meanwhile_abandons_the_release": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil)
				deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: 3}, nil)
				deps.publications.EXPECT().BeginPublication(ctx, testUserUUID, testGameUUID, int64(3), game.Private, testKey).Return(game.Publication{Outcome: game.PublicationStarted, Release: &release}, nil)
				deps.versions.EXPECT().FreezeVersion(ctx, testGameUUID, testVersionUUID, int64(3)).Return(sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeNotReady, Revision: 4, Blockers: []sessionpkg.PublishBlocker{{Code: sessionpkg.PublishRevisionChanged}}}, nil)
				deps.publications.EXPECT().AbandonPublication(ctx, release.UUID, "revision_changed").Return(release, nil)
			},
			expected: Publication{Outcome: PublishNotReady, Release: &release, Blockers: []PublishBlocker{{Code: "revision_changed"}}},
		},
		"a_release_a_crash_left_is_completed_first": {
			setup: func(deps dependencies) {
				gomock.InOrder(
					deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(&release, nil),
					deps.versions.EXPECT().FreezeVersion(ctx, testGameUUID, testVersionUUID, int64(3)).Return(sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeAlreadyFrozen, Revision: 3, Languages: []string{"es", "en"}}, nil),
					deps.publications.EXPECT().CompletePublication(ctx, release.UUID, []string{"es", "en"}).Return(release, nil),
					deps.versions.EXPECT().CleanUnusedObjects(ctx).Return(sessionpkg.CleanedObjects{}, nil),
					deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotEditable}, nil),
				)
			},
			expected: Publication{Outcome: PublishNotEditable},
		},
		"a_retry_of_a_completed_release_answers_it": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil)
				deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: 3}, nil)
				deps.publications.EXPECT().BeginPublication(ctx, testUserUUID, testGameUUID, int64(3), game.Private, testKey).Return(game.Publication{Outcome: game.PublicationCompleted, Release: &release}, nil)
			},
			expected: Publication{Outcome: Published, Release: &release},
		},
		"another_users_game_is_not_found": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)
			},
			expected: Publication{Outcome: PublishNotFound},
		},
		"a_reused_key_keeps_its_error": {
			setup: func(deps dependencies) {
				deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
				deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil)
				deps.versions.EXPECT().CheckPublication(ctx, testVersionUUID).Return(sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: 3}, nil)
				deps.publications.EXPECT().BeginPublication(ctx, testUserUUID, testGameUUID, int64(3), game.Private, testKey).Return(game.Publication{}, game.ErrIdempotencyConflict)
			},
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, deps := newPublishTestManager(t)
			tc.setup(deps)
			published, err := m.Publish(ctx, testUserUUID, testGameUUID, game.Private, nil, testKey)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, published)
		})
	}
}

func TestPublishRetryOfAnEndedRelease(t *testing.T) {
	ctx := context.Background()
	release := game.Release{UUID: "r-1", VersionUUID: testVersionUUID, Revision: 3}
	ctrl := gomock.NewController(t)
	deps := dependencies{publications: NewMockgamePublisher(ctrl)}
	m := New(nil, nil, nil, nil, nil, deps.publications, nil, nil)
	gomock.InOrder(
		deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil),
		deps.publications.EXPECT().FindPublication(ctx, testUserUUID, testGameUUID, testKey).Return(&game.Publication{Outcome: game.PublicationCompleted, Release: &release}, nil),
	)
	published, err := m.Publish(ctx, testUserUUID, testGameUUID, game.Private, nil, testKey)
	require.NoError(t, err)
	require.Equal(t, Publication{Outcome: Published, Release: &release}, published)
}

func TestPublishWithName(t *testing.T) {
	ctx := context.Background()
	name := "Memoria"

	t.Run("a refused name is not ready and nothing else runs", func(t *testing.T) {
		m, deps := newPublishTestManager(t)
		deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
		deps.cards.EXPECT().UpdateCard(ctx, testUserUUID, testGameUUID, game.CardChange{Name: &name}).Return(game.CardEdit{Outcome: game.CardInvalid}, nil)
		published, err := m.Publish(ctx, testUserUUID, testGameUUID, game.Public, &name, testKey)
		require.NoError(t, err)
		require.Equal(t, Publication{Outcome: PublishNotReady, Blockers: []PublishBlocker{{Code: "invalid_name"}}}, published)
	})

	t.Run("the name is set on the card before validating", func(t *testing.T) {
		m, deps := newPublishTestManager(t)
		gomock.InOrder(
			deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil),
			deps.cards.EXPECT().UpdateCard(ctx, testUserUUID, testGameUUID, game.CardChange{Name: &name}).Return(game.CardEdit{Outcome: game.CardDone}, nil),
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotEditable}, nil),
		)
		published, err := m.Publish(ctx, testUserUUID, testGameUUID, game.Public, &name, testKey)
		require.NoError(t, err)
		require.Equal(t, PublishNotEditable, published.Outcome)
	})
}

func TestOpenEdition(t *testing.T) {
	ctx := context.Background()
	const baseVersion = "22222222-0000-4000-8000-000000000002"

	t.Run("opens the edition and builds it from the published version", func(t *testing.T) {
		m, deps := newPublishTestManager(t)
		gomock.InOrder(
			deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil),
			deps.publications.EXPECT().OpenEdition(ctx, testUserUUID, testGameUUID, testKey).Return(game.Edition{Outcome: game.EditionOpened, VersionUUID: testVersionUUID, BaseVersionUUID: baseVersion}, nil),
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, baseVersion).Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, Revision: 1}, nil),
		)
		opened, err := m.OpenEdition(ctx, testUserUUID, testGameUUID, testKey)
		require.NoError(t, err)
		require.Equal(t, EditionOpening{Outcome: game.EditionOpened, VersionUUID: testVersionUUID, Revision: 1}, opened)
	})

	t.Run("an unpublished game has no new edition", func(t *testing.T) {
		m, deps := newPublishTestManager(t)
		deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
		deps.publications.EXPECT().OpenEdition(ctx, testUserUUID, testGameUUID, testKey).Return(game.Edition{Outcome: game.EditionNotPublished}, nil)
		opened, err := m.OpenEdition(ctx, testUserUUID, testGameUUID, testKey)
		require.NoError(t, err)
		require.Equal(t, game.EditionNotPublished, opened.Outcome)
	})

	t.Run("a later edition missing in Play is repaired from the published version", func(t *testing.T) {
		m, deps := newPublishTestManager(t)
		deps.publications.EXPECT().FindPendingPublication(ctx, testUserUUID, testGameUUID).Return(nil, nil)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID, BaseVersionUUID: baseVersion}, nil)
		gomock.InOrder(
			deps.drafts.EXPECT().RemoveAsset(ctx, testVersionUUID, "cardBack", testKey, "web").Return(sessionpkg.DraftAssetResult{}, sessionpkg.ErrDraftNotFound),
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, baseVersion).Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, Revision: 1}, nil),
			deps.drafts.EXPECT().RemoveAsset(ctx, testVersionUUID, "cardBack", testKey, "web").Return(sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetUnchanged, Revision: 1}, nil),
		)
		_, err := m.RemoveDraftAsset(ctx, testUserUUID, testGameUUID, "cardBack", testKey, "web")
		require.NoError(t, err)
	})
}
