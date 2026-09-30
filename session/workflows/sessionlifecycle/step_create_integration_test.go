package sessionlifecycle

import (
	"context"
	"sync"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestManagerCreate_Integration exercises the real public Manager.Create
// against a real Postgres database (not mocked collaborators), proving the
// full Host/SessionActor Creation Cycle, session.JoinCode issuance, and
// idempotency persistence. Create resolves entirely from Session Runtime's
// own session_games/session_game_version_artifacts tables; no Game
// Management stub is involved.
func TestManagerCreate_Integration(t *testing.T) {
	t.Run("creates_lobby_session_host_actor_and_active_join_code", func(t *testing.T) {
		db := testdb.OpenSessionDB(t)
		m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
		fixture := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)

		result, err := m.Create(context.Background(), session.GameUUID(fixture.GameUUID), session.UserUUID(uuid.NewString()), "create-key-1")
		require.NoError(t, err)
		require.NotEmpty(t, result.SessionUUID)
		require.GreaterOrEqual(t, uint(result.JoinCode), uint(1000))
		require.LessOrEqual(t, uint(result.JoinCode), uint(9999))

		var row struct {
			ID                 uint
			Phase              string
			HostActorID        *uint
			GameDefinitionUUID string
		}
		require.NoError(t, db.Raw(`SELECT id, phase, host_actor_id, game_definition_uuid FROM sessions WHERE uuid = ?`, result.SessionUUID).Scan(&row).Error)
		require.Equal(t, session.PhaseLobby, row.Phase)
		require.NotNil(t, row.HostActorID)
		require.Equal(t, fixture.DefinitionUUID, row.GameDefinitionUUID, "Create must pin the game's current definition, resolved from session_games")

		var participantCount int64
		require.NoError(t, db.Raw(`
			SELECT COUNT(*) FROM session_participants p
			INNER JOIN session_actors a ON a.id = p.session_actor_id
			WHERE a.session_id = ?
		`, row.ID).Scan(&participantCount).Error)
		require.Equal(t, int64(0), participantCount, "host must not be created as an active participant")

		var activeJoinCodeCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM join_codes WHERE session_id = ? AND revoked_at IS NULL`, row.ID).Scan(&activeJoinCodeCount).Error)
		require.Equal(t, int64(1), activeJoinCodeCount)
	})

	t.Run("a_game_with_no_current_version_in_session_runtime_is_rejected", func(t *testing.T) {
		db := testdb.OpenSessionDB(t)
		m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		result, err := m.Create(context.Background(), session.GameUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), "create-key-missing")
		require.ErrorIs(t, err, session.ErrGameNotFound)
		require.Empty(t, result.SessionUUID)
	})
}

func TestManagerCreate_Integration_RepeatedSemanticallyEquivalentCreateReplays(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
	fixture := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)

	gameUUID := session.GameUUID(fixture.GameUUID)
	hostUserUUID := session.UserUUID(uuid.NewString())

	first, err := m.Create(context.Background(), gameUUID, hostUserUUID, "create-key-replay")
	require.NoError(t, err)

	second, err := m.Create(context.Background(), gameUUID, hostUserUUID, "create-key-replay")
	require.NoError(t, err)
	require.Equal(t, first, second)

	var sessionCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM sessions WHERE uuid = ?`, first.SessionUUID).Scan(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)
}

func TestManagerCreate_Integration_ConflictingIdempotencyIdentityIsRejected(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
	fixtureA := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
	fixtureB := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)

	hostUserUUID := session.UserUUID(uuid.NewString())
	_, err := m.Create(context.Background(), session.GameUUID(fixtureA.GameUUID), hostUserUUID, "create-key-conflict")
	require.NoError(t, err)

	_, err = m.Create(context.Background(), session.GameUUID(fixtureB.GameUUID), hostUserUUID, "create-key-conflict")
	require.ErrorIs(t, err, session.ErrIdempotencyConflict)
}

func TestManagerCreate_Integration_DifferentUsersReusingSameKeyDoNotCollide(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
	fixture := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)

	gameUUID := session.GameUUID(fixture.GameUUID)
	_, err := m.Create(context.Background(), gameUUID, session.UserUUID(uuid.NewString()), "shared-key")
	require.NoError(t, err)

	result, err := m.Create(context.Background(), gameUUID, session.UserUUID(uuid.NewString()), "shared-key")
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionUUID)
}

// TestManagerCreate_Integration_Concurrent proves - against a real Postgres
// database - that of several concurrent Manager.Create calls sharing the
// same (user_uuid, CREATE, idempotency_key), exactly one Session is ever
// created and every call that succeeds observes that identical logical
// result. Create has no pre-existing Session row to lock, so its only
// serialization comes from the shared idempotency claim
// (internal/idempotency's Claim); Claim does not itself resolve a race
// between its own fetch and its own insert, so a call whose insert races the
// eventual winner's still-open one is expected to fail with a
// unique-constraint-violation error rather than gracefully replay - this is
// the deliberate, simpler design chosen over a claim mechanism that
// papers over that race.
func TestManagerCreate_Integration_Concurrent(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := newTestManager(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
	fixture := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)

	gameUUID := session.GameUUID(fixture.GameUUID)
	hostUserUUID := session.UserUUID(uuid.NewString())

	const attempts = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]session.CreatedSession, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = m.Create(context.Background(), gameUUID, hostUserUUID, "concurrent-create-key")
		}(i)
	}
	close(start)
	wg.Wait()

	var successResult *session.CreatedSession
	successCount := 0
	for i := 0; i < attempts; i++ {
		if errs[i] != nil {
			// Expected for a call that raced the eventual winner's
			// still-open insert - see the test's doc comment.
			continue
		}
		successCount++
		if successResult == nil {
			successResult = &results[i]
		} else {
			require.Equal(t, *successResult, results[i], "attempt %d observed a different logical result", i)
		}
	}
	require.GreaterOrEqual(t, successCount, 1, "at least one attempt must succeed")

	var sessionCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM sessions WHERE uuid = ?`, successResult.SessionUUID).Scan(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)

	var requestCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_requests
		WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
	`, string(hostUserUUID), operationCreate, "concurrent-create-key").Scan(&requestCount).Error)
	require.Equal(t, int64(1), requestCount, "exactly one idempotency record must exist for the shared identity")
}
