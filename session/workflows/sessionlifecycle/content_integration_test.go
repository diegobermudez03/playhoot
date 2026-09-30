package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedStartableSession seeds a LOBBY session pinned to a fresh game version
// whose backend script is unique to this call, so a test that tampers with,
// or counts reads of, that script's object never affects another test that
// shares the content-addressed store. It returns the session, its version and
// the host's user UUID.
func seedStartableSession(t *testing.T, db *gorm.DB) (testfixtures.SessionFixture, testfixtures.GameVersionFixture, string, string) {
	t.Helper()

	backendScript := "function backend() { /* " + uuid.NewString() + " */ }"
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	gv := testfixtures.SeedCurrentGameVersion(t, db, backendScript, "function frontend() {}", 1, nil)
	require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)

	hostUUID := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
	testfixtures.SeedJoinCode(t, db, fx.SessionID, uint(1000+time.Now().UnixNano()%8000), false)
	return fx, gv, hostUUID, backendScript
}

func backendScriptKey(t *testing.T, db *gorm.DB, definitionUUID string) string {
	t.Helper()
	var key string
	require.NoError(t, db.Raw(`SELECT backend_script_key FROM session_game_version_artifacts WHERE definition_uuid = ?`, definitionUUID).Scan(&key).Error)
	return key
}

// TestBackendScriptLoadedFromObjectStorage_Integration proves the Executor is
// handed exactly the script bytes stored in object storage, that a missing,
// tampered or unreachable object fails the step closed without starting the
// Session, and that once loaded a script is not fetched again.
func TestBackendScriptLoadedFromObjectStorage_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	ctx := context.Background()

	t.Run("executor_receives_the_script_stored_in_object_storage", func(t *testing.T) {
		fx, _, hostUUID, script := seedStartableSession(t, db)
		var got string
		exec := &executor.Fake{ExecuteFunc: func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			got = in.Script.Source
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		}}

		result, err := newTestManager(db, exec).Start(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-1")
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeStarted, result.Outcome)
		require.Equal(t, script, got)
	})

	t.Run("a_script_is_fetched_once_and_reused_by_later_steps", func(t *testing.T) {
		fx, gv, hostUUID, _ := seedStartableSession(t, db)
		store := testfixtures.ContentStore()
		key := backendScriptKey(t, db, gv.DefinitionUUID)
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})), store)

		_, err := m.Start(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-1")
		require.NoError(t, err)
		require.Equal(t, 1, store.GetCount(key), "Start loads the script cold")

		result, err := m.SubmitPlayerEvent(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "move", nil, "submit-1")
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)
		require.Equal(t, 1, store.GetCount(key), "the second step must reuse the verified script, not hit storage")
	})

	t.Run("a_tampered_object_fails_closed_and_the_session_does_not_start", func(t *testing.T) {
		fx, gv, hostUUID, _ := seedStartableSession(t, db)
		testfixtures.ContentStore().Corrupt(backendScriptKey(t, db, gv.DefinitionUUID), []byte("function evil() {}"))

		_, err := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{}))).Start(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-1")
		require.ErrorIs(t, err, objectstore.ErrHashMismatch)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase, "a failed script load must roll the whole Start back")
		var turns int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turns).Error)
		require.Zero(t, turns)
	})

	t.Run("a_missing_object_fails_closed", func(t *testing.T) {
		fx, _, hostUUID, _ := seedStartableSession(t, db)
		emptyStore := objectstore.NewFake()

		_, err := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})), emptyStore).Start(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-1")
		require.ErrorIs(t, err, objectstore.ErrNotFound)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase)
	})

	t.Run("a_storage_outage_fails_the_step_and_leaves_the_session_untouched", func(t *testing.T) {
		fx, _, hostUUID, _ := seedStartableSession(t, db)
		outage := errors.New("storage unavailable")
		down := objectstore.NewFake()
		down.GetErr = outage

		_, err := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})), down).Start(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "start-1")
		require.ErrorIs(t, err, outage)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase)
	})
}

// TestContentAccess_Integration covers GetFrontendScriptAccess and
// GetAssetAccess against a real database: participant-only, pinned-version
// only, and signing exactly the object the version's locator names.
func TestContentAccess_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	ctx := context.Background()
	m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

	t.Run("a_participant_gets_a_signed_url_for_the_pinned_frontend_script", func(t *testing.T) {
		frontend := "function frontend() { /* " + uuid.NewString() + " */ }"
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", frontend, 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		userUUID := uuid.NewString()
		testfixtures.SeedActor(t, db, fx.SessionID, userUUID)

		result, err := m.GetFrontendScriptAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID))
		require.NoError(t, err)

		want := objectstore.LocatorFor("scripts", []byte(frontend))
		require.Equal(t, session.ContentAccessOutcomeGranted, result.Outcome)
		require.Contains(t, result.URL, want.ObjectKey, "the signed URL must be for exactly the pinned script's object")
		require.Equal(t, want.SHA256, result.SHA256)
		require.Equal(t, want.Size, result.Size)
		require.Equal(t, "text/javascript", result.ContentType)
		require.WithinDuration(t, time.Now().Add(defaultSignedURLTTL), result.ExpiresAt, 5*time.Second)
	})

	t.Run("a_non_participant_is_declined", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		testfixtures.SeedActor(t, db, fx.SessionID, uuid.NewString())

		result, err := m.GetFrontendScriptAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeNotAParticipant, result.Outcome)
		require.Empty(t, result.URL, "a declined caller must never receive a URL")

		asset, err := m.GetAssetAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(uuid.NewString()), "anything")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeNotAParticipant, asset.Outcome)
		require.Empty(t, asset.URL)
	})

	t.Run("a_participant_of_a_different_session_is_declined", func(t *testing.T) {
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		mine := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		other := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		for _, fx := range []testfixtures.SessionFixture{mine, other} {
			require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		}
		otherUser := uuid.NewString()
		testfixtures.SeedActor(t, db, other.SessionID, otherUser)

		result, err := m.GetFrontendScriptAccess(ctx, session.SessionUUID(mine.SessionUUID), session.UserUUID(otherUser))
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeNotAParticipant, result.Outcome)
	})

	t.Run("an_unknown_session_returns_session_not_found", func(t *testing.T) {
		_, err := m.GetFrontendScriptAccess(ctx, session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrSessionNotFound)
		_, err = m.GetAssetAccess(ctx, session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), "k")
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})

	t.Run("a_declared_asset_is_signed_and_an_undeclared_key_is_not_found", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)
		userUUID := uuid.NewString()
		testfixtures.SeedActor(t, db, fx.SessionID, userUUID)
		png := []byte("png-" + uuid.NewString())
		locator := testfixtures.SeedAsset(t, db, gv.DefinitionUUID, "card-back", "image", "image/png", png)

		granted, err := m.GetAssetAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "card-back")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeGranted, granted.Outcome)
		require.Contains(t, granted.URL, locator.ObjectKey)
		require.Equal(t, locator.SHA256, granted.SHA256)
		require.Equal(t, "image/png", granted.ContentType)
		require.Equal(t, int64(len(png)), granted.Size)

		missing, err := m.GetAssetAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "not-declared")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeContentNotFound, missing.Outcome)
		require.Empty(t, missing.URL)
	})

	t.Run("a_session_only_ever_gets_the_version_it_is_pinned_to", func(t *testing.T) {
		frontendA := "function frontendA() { /* " + uuid.NewString() + " */ }"
		frontendB := "function frontendB() { /* " + uuid.NewString() + " */ }"
		versionA := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", frontendA, 1, nil)
		versionB := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", frontendB, 1, nil)
		testfixtures.SeedAsset(t, db, versionB.DefinitionUUID, "only-in-b", "image", "image/png", []byte("b-"+uuid.NewString()))

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, versionA.DefinitionUUID, fx.SessionID).Error)
		userUUID := uuid.NewString()
		testfixtures.SeedActor(t, db, fx.SessionID, userUUID)

		result, err := m.GetFrontendScriptAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID))
		require.NoError(t, err)
		require.Equal(t, objectstore.LocatorFor("scripts", []byte(frontendA)).SHA256, result.SHA256, "must serve the pinned version A")
		require.NotEqual(t, objectstore.LocatorFor("scripts", []byte(frontendB)).SHA256, result.SHA256)

		leak, err := m.GetAssetAccess(ctx, session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "only-in-b")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeContentNotFound, leak.Outcome, "an asset declared only by another version must not resolve")
	})
}

// TestGameVersionAssets_Integration proves the schema itself enforces one
// asset per (version, logical key) and refuses assets for a version that has
// no artifact row.
func TestGameVersionAssets_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
	testfixtures.SeedAsset(t, db, gv.DefinitionUUID, "logo", "image", "image/png", []byte("one-"+uuid.NewString()))

	err := insertAssetRow(db, gv.DefinitionUUID, "logo")
	require.Error(t, err, "a second asset under the same (version, key) must be rejected")

	err = insertAssetRow(db, uuid.NewString(), "logo")
	require.Error(t, err, "an asset for a version with no artifact must be rejected")
}

func insertAssetRow(db *gorm.DB, definitionUUID, key string) error {
	return db.Exec(`
		INSERT INTO session_game_version_assets (definition_uuid, key, kind, object_key, sha256, size, content_type)
		VALUES (?, ?, 'image', 'assets/x', 'x', 1, 'image/png')
	`, definitionUUID, key).Error
}
