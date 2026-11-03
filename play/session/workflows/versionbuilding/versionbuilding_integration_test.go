package versionbuilding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/gamebuild"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting"
)

// validScript passes scriptcheck's sdk-v1 profile.
const validScript = `"use strict";
function start(p) { return { state: {} }; }
function project(s) { return {}; }
function __playhoot_module_values() { return []; }
`

const validManifest = `{"schema_version":1,"players":{"min":1,"max":8},"inputs":{},"commands":{},"entrypoints":[]}`

// fakeBuilder answers what output and err say, recording the sources it was
// given.
type fakeBuilder struct {
	mu      sync.Mutex
	output  gamebuild.Output
	err     error
	builds  int
	sources map[string]string
}

func (f *fakeBuilder) Version(ctx context.Context) (string, error) { return "toolchain-1", nil }

func (f *fakeBuilder) Build(ctx context.Context, input gamebuild.Input) (gamebuild.Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.builds++
	f.sources = map[string]string{}
	_ = filepath.WalkDir(input.Dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			content, _ := os.ReadFile(path)
			relative, _ := filepath.Rel(input.Dir, path)
			f.sources[filepath.ToSlash(relative)] = string(content)
		}
		return nil
	})
	return f.output, f.err
}

func TestBuildingIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	ctx := context.Background()
	store := objectstore.NewFake()
	drafts := versiondrafting.New(db, store, nil)
	builder := &fakeBuilder{}
	m := New(db, store, builder, nil, 1)
	built := gamebuild.Output{OK: true, Server: []byte(validScript), Client: []byte("<html></html>"), Manifest: []byte(validManifest), AspectRatio: "16:9", Languages: []string{"es", "en"}}

	newDraft := func(t *testing.T) string {
		t.Helper()
		versionUUID := uuid.NewString()
		_, err := drafts.CreateDraftVersion(ctx, uuid.NewString(), versionUUID, "")
		require.NoError(t, err)
		return versionUUID
	}
	// runAll runs queued builds, other cases' ones included, until none waits.
	runAll := func(t *testing.T) {
		t.Helper()
		for {
			ran, err := m.RunNextBuild(ctx)
			require.NoError(t, err)
			if !ran {
				return
			}
		}
	}
	change := func(t *testing.T, versionUUID string, base int64, content string) {
		t.Helper()
		applied, err := drafts.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{
			BaseRevision: base, IdempotencyKey: uuid.NewString(), Origin: sessionpkg.DraftOriginWeb,
			Files: []sessionpkg.DraftFileContent{{Path: "notes.md", Content: content}},
		})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, applied.Outcome)
	}

	t.Run("a_built_revision_is_checked_stored_and_succeeded", func(t *testing.T) {
		builder.output, builder.err = built, nil
		versionUUID := newDraft(t)
		runAll(t)

		succeeded, err := drafts.ReadBuild(ctx, versionUUID, 1, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildSucceeded, succeeded.Status, succeeded.Diagnostics)
		require.NotNil(t, succeeded.StartedAt)
		require.NotNil(t, succeeded.FinishedAt)
		require.Equal(t, "16:9", succeeded.AspectRatio)
		require.Equal(t, objectstore.HashHex([]byte(validScript)), succeeded.Server.SHA256)
		require.Contains(t, builder.sources, "server/game.ts", "the revision's sources are what the toolchain builds")

		stored, err := store.Get(ctx, backendScriptPrefix+"/"+succeeded.Server.SHA256)
		require.NoError(t, err)
		require.Equal(t, validScript, string(stored))
		var registered int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM play_build_artifact_objects WHERE sha256 IN ?`, []string{succeeded.Server.SHA256, succeeded.Client.SHA256}).Scan(&registered).Error)
		require.Equal(t, int64(2), registered)

		snapshot, err := drafts.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, int64(1), snapshot.LastSuccessfulBuild.Revision)
	})

	t.Run("a_revision_with_the_same_inputs_reuses_the_build", func(t *testing.T) {
		builder.output, builder.err = built, nil
		versionUUID := newDraft(t)
		runAll(t)
		change(t, versionUUID, 1, "changed")
		runAll(t)
		before := builder.builds
		restored, err := drafts.RestoreRevision(ctx, versionUUID, 1, 2, uuid.NewString(), sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, int64(3), restored.Revision)
		runAll(t)

		reused, err := drafts.ReadBuild(ctx, versionUUID, 3, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildSucceeded, reused.Status)
		require.True(t, reused.Reused)
		require.Equal(t, before, builder.builds, "a reused build runs no toolchain")
	})

	t.Run("mistakes_of_the_game_and_refusals_of_the_runtime_fail_with_diagnostics", func(t *testing.T) {
		builder.output = gamebuild.Output{Diagnostics: []build.Diagnostic{{Stage: build.StageTypes, File: "server/game.ts", Line: 3, Column: 7, Message: "no such property"}}}
		versionUUID := newDraft(t)
		runAll(t)
		failed, err := drafts.ReadBuild(ctx, versionUUID, 1, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildFailed, failed.Status)
		require.Equal(t, []sessionpkg.DraftBuildDiagnostic{{Stage: "types", File: "server/game.ts", Line: 3, Column: 7, Message: "no such property"}}, failed.Diagnostics)

		builder.output = gamebuild.Output{OK: true, Server: []byte("let state = 1;\nfunction start() {}"), Client: []byte("x"), Manifest: []byte(`{}`), AspectRatio: "21:9"}
		change(t, versionUUID, 1, "refused")
		runAll(t)
		refused, err := drafts.ReadBuild(ctx, versionUUID, 2, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildFailed, refused.Status)
		stages := map[string]bool{}
		for _, d := range refused.Diagnostics {
			stages[d.Stage] = true
		}
		require.Equal(t, map[string]bool{"sandbox": true, "manifest": true}, stages)
		require.Nil(t, refused.Server)
	})

	t.Run("the_platform_failing_is_infrastructure_never_the_game", func(t *testing.T) {
		builder.err = gamebuild.ErrUnavailable
		versionUUID := newDraft(t)
		runAll(t)
		failed, err := drafts.ReadBuild(ctx, versionUUID, 1, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildFailed, failed.Status)
		require.Len(t, failed.Diagnostics, 1)
		require.Equal(t, "infrastructure", failed.Diagnostics[0].Stage)
		require.Equal(t, build.CodeToolchainUnavailable, failed.Diagnostics[0].Code)

		builder.err = context.DeadlineExceeded
		change(t, versionUUID, 1, "slow")
		runAll(t)
		slow, err := drafts.ReadBuild(ctx, versionUUID, 2, 0)
		require.NoError(t, err)
		require.Equal(t, build.CodeTimeout, slow.Diagnostics[0].Code)

		builder.err = errors.New("node crashed")
		change(t, versionUUID, 2, "crash")
		runAll(t)
		crashed, err := drafts.ReadBuild(ctx, versionUUID, 3, 0)
		require.NoError(t, err)
		require.Equal(t, build.CodeToolchainFailed, crashed.Diagnostics[0].Code)
		builder.err = nil
	})

	t.Run("builds_left_running_are_queued_again", func(t *testing.T) {
		versionUUID := newDraft(t)
		require.NoError(t, db.Exec(`
			UPDATE play_game_version_builds SET status = ?
			WHERE game_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
		`, string(build.Running), versionUUID).Error)
		requeued, err := m.RequeueInterrupted(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, requeued, 1)
		queued, err := drafts.ReadBuild(ctx, versionUUID, 1, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildQueued, queued.Status)
		runAll(t)
	})
}
