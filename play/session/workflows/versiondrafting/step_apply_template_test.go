package versiondrafting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestApplyTemplateInTx(t *testing.T) {
	const (
		versionUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		templateGame = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		templateUUID = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
		versionID    = uint(21)
		templateID   = uint(40)
		attemptID    = uint(5)
		fingerprint  = "fingerprint"
	)
	ctx := context.Background()
	tx := &gorm.DB{}
	revision := func(n int64) *int64 { return &n }
	draftAt := func(current int64) *internalrepo.Version {
		return &internalrepo.Version{ID: versionID, UUID: versionUUID, GameUUID: "game", Status: "DRAFT", ScriptProfile: "sdk-v1", CurrentRevision: revision(current)}
	}
	published := &internalrepo.Version{ID: templateID, UUID: templateUUID, GameUUID: templateGame, Status: "EXECUTABLE", ScriptProfile: "sdk-v1", CurrentRevision: revision(9)}
	template := sessionpkg.DraftTemplate{GameUUID: templateGame, VersionUUID: templateUUID}
	project := `{"format":"playhoot.project.v1","languages":["es"],"assets":{"cardBack":{"type":"image","description":"Back"}}}`
	projectHash := draftsources.SHA256Hex(project)
	draftRevision := draftsources.Revision{
		Files:  map[string]draftsources.File{"notes.md": {SHA256: "notes"}, "server/game.ts": {SHA256: "draft"}},
		Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "mine", MIMEType: "image/png"}},
	}
	templateRevision := draftsources.Revision{
		Files: map[string]draftsources.File{"server/game.ts": {SHA256: "template"}, "game.project.json": {SHA256: projectHash}},
	}
	claims := func(repo *MockapplyTemplateRepo, version *internalrepo.Version) {
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(version, nil)
		repo.EXPECT().ClaimChangeAttempt(ctx, tx, versionID, "template", "key", fingerprint, int64(3), "mcp").Return(attemptID, nil, nil)
	}

	t.Run("switches_to_the_template_keeping_the_notes_and_uploads", func(t *testing.T) {
		repo := NewMockapplyTemplateRepo(gomock.NewController(t))
		claims(repo, draftAt(3))
		expected := draftsources.Revision{
			Files:  map[string]draftsources.File{"notes.md": {SHA256: "notes"}, "server/game.ts": {SHA256: "template"}, "game.project.json": {SHA256: projectHash}},
			Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "mine", MIMEType: "image/png"}},
		}
		gomock.InOrder(
			repo.EXPECT().FindRevision(ctx, tx, versionID, int64(3)).Return(draftRevision, true, nil),
			repo.EXPECT().FindVersion(ctx, tx, templateUUID).Return(published, nil),
			repo.EXPECT().FindRevision(ctx, tx, templateID, int64(9)).Return(templateRevision, true, nil),
			repo.EXPECT().FindSourceContents(ctx, tx, []string{projectHash}).Return(map[string]string{projectHash: project}, nil),
			repo.EXPECT().AdvanceRevision(ctx, tx, versionID, "DRAFT", int64(3)).Return(true, nil),
			repo.EXPECT().CreateRevision(ctx, tx, versionID, int64(4), expected, internalrepo.Provenance{
				Kind: "template", Origin: "mcp", Actor: "ai", ParentRevision: revision(3), SDK: "sdk-v1",
				FromGameUUID: templateGame, FromVersionUUID: templateUUID, FromRevision: revision(9),
			}).Return(nil),
			repo.EXPECT().QueueBuild(ctx, tx, versionID, int64(4), "build-uuid", "QUEUED", "SUPERSEDED").Return(0, nil),
			repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "applied", revision(4), nil).Return(nil),
		)
		m := &Manager{applyTemplateRepo: repo, newUUID: func() string { return "build-uuid" }}

		got, err := m.applyTemplateInTx(ctx, tx, versionUUID, template, 3, "key", "mcp", fingerprint)

		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 4}, got.result)
	})

	t.Run("a_stale_base_is_a_conflict", func(t *testing.T) {
		repo := NewMockapplyTemplateRepo(gomock.NewController(t))
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(draftAt(5), nil)
		repo.EXPECT().ClaimChangeAttempt(ctx, tx, versionID, "template", "key", fingerprint, int64(3), "mcp").Return(attemptID, nil, nil)
		repo.EXPECT().CompleteChangeAttempt(ctx, tx, attemptID, "conflict", revision(5), nil).Return(nil)
		m := &Manager{applyTemplateRepo: repo}

		got, err := m.applyTemplateInTx(ctx, tx, versionUUID, template, 3, "key", "mcp", fingerprint)

		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeConflict, got.result.Outcome)
	})

	t.Run("a_version_that_is_not_the_template_s_published_one", func(t *testing.T) {
		repo := NewMockapplyTemplateRepo(gomock.NewController(t))
		claims(repo, draftAt(3))
		repo.EXPECT().FindRevision(ctx, tx, versionID, int64(3)).Return(draftRevision, true, nil)
		other := *published
		other.GameUUID = "someone-else"
		repo.EXPECT().FindVersion(ctx, tx, templateUUID).Return(&other, nil)
		m := &Manager{applyTemplateRepo: repo}

		_, err := m.applyTemplateInTx(ctx, tx, versionUUID, template, 3, "key", "mcp", fingerprint)

		require.ErrorIs(t, err, sessionpkg.ErrDraftTemplateNotFound)
	})
}

func TestCreateDraftVersionFromATemplate(t *testing.T) {
	const (
		gameUUID     = "16fd2706-8baf-433b-82eb-8c7fada847da"
		versionUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		templateGame = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		templateUUID = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	)
	ctx := context.Background()
	tx := &gorm.DB{}
	nine := int64(9)
	copied := draftsources.Revision{Files: map[string]draftsources.File{"server/game.ts": {SHA256: "template"}}}
	repo := NewMockcreateDraftVersionRepo(gomock.NewController(t))
	gomock.InOrder(
		repo.EXPECT().LockGame(ctx, tx, gameUUID).Return(uint(7), nil),
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(nil, nil),
		repo.EXPECT().FindVersionOfGameInStatus(ctx, tx, uint(7), "DRAFT").Return(nil, nil),
		repo.EXPECT().FindVersion(ctx, tx, templateUUID).Return(&internalrepo.Version{ID: 40, UUID: templateUUID, GameUUID: templateGame, Status: "EXECUTABLE", CurrentRevision: &nine}, nil),
		repo.EXPECT().FindRevision(ctx, tx, uint(40), int64(9)).Return(copied, true, nil),
		repo.EXPECT().CreateVersion(ctx, tx, uint(7), versionUUID, "DRAFT", "sdk-v1", int64(1)).Return(uint(21), nil),
		repo.EXPECT().CreateRevision(ctx, tx, uint(21), int64(1), copied, internalrepo.Provenance{
			Kind: "template", Actor: "platform", SDK: "sdk-v1", FromGameUUID: templateGame, FromVersionUUID: templateUUID, FromRevision: &nine,
		}).Return(nil),
		repo.EXPECT().QueueBuild(ctx, tx, uint(21), int64(1), "build-uuid", "QUEUED", "SUPERSEDED").Return(0, nil),
	)
	m := &Manager{createDraftVersionRepo: repo, newUUID: func() string { return "build-uuid" }}

	got, err := m.createDraftVersionInTx(ctx, tx, gameUUID, versionUUID, sessionpkg.DraftStart{FromVersionUUID: templateUUID, TemplateGameUUID: templateGame})

	require.NoError(t, err)
	require.Equal(t, sessionpkg.DraftCreated, got.result.Outcome)
}
