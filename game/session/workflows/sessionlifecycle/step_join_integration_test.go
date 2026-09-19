package sessionlifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestManagerJoin_Integration(t *testing.T) {
	type test struct {
		before func(t *testing.T, db *gorm.DB) (fx testfixtures.SessionFixture, joinCode JoinCode, userUUID UserUUID)
		assert func(t *testing.T, db *gorm.DB, fx testfixtures.SessionFixture, userUUID UserUUID, result JoinResult, err error)
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"creates_actor_and_activates_participant": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) (testfixtures.SessionFixture, JoinCode, UserUUID) {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedJoinCode(t, db, fx.SessionID, 1111, false)
					return fx, 1111, UserUUID(uuid.NewString())
				},
				assert: func(t *testing.T, db *gorm.DB, fx testfixtures.SessionFixture, userUUID UserUUID, result JoinResult, err error) {
					require.NoError(t, err)
					require.Equal(t, DisplayName("Alice"), result.DisplayName)

					var actorCount int64
					require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE session_id = ? AND user_uuid = ?`, fx.SessionID, string(userUUID)).Scan(&actorCount).Error)
					require.Equal(t, int64(1), actorCount)

					var active bool
					require.NoError(t, db.Raw(`
						SELECT p.active FROM session_participants p
						INNER JOIN session_actors a ON a.id = p.session_actor_id
						WHERE a.session_id = ? AND a.user_uuid = ?
					`, fx.SessionID, string(userUUID)).Scan(&active).Error)
					require.True(t, active)
				},
			}
		},
		"a_revoked_join_code_is_rejected": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) (testfixtures.SessionFixture, JoinCode, UserUUID) {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedJoinCode(t, db, fx.SessionID, 1112, true)
					return fx, 1112, UserUUID(uuid.NewString())
				},
				assert: func(t *testing.T, db *gorm.DB, fx testfixtures.SessionFixture, userUUID UserUUID, result JoinResult, err error) {
					require.ErrorIs(t, err, session.ErrJoinCodeInvalid)
				},
			}
		},
		"lazily_materializes_expired_lobby_and_rejects": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) (testfixtures.SessionFixture, JoinCode, UserUUID) {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
					testfixtures.SeedJoinCode(t, db, fx.SessionID, 1113, false)
					return fx, 1113, UserUUID(uuid.NewString())
				},
				assert: func(t *testing.T, db *gorm.DB, fx testfixtures.SessionFixture, userUUID UserUUID, result JoinResult, err error) {
					require.NoError(t, err)
					require.Equal(t, JoinOutcomeLobbyExpired, result.Outcome)

					var phase string
					require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
					require.Equal(t, session.PhaseTerminal, phase)
				},
			}
		},
		"enforces_players_max": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) (testfixtures.SessionFixture, JoinCode, UserUUID) {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedJoinCode(t, db, fx.SessionID, 1114, false)
					testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Existing Player")
					return fx, 1114, UserUUID(uuid.NewString())
				},
				assert: func(t *testing.T, db *gorm.DB, fx testfixtures.SessionFixture, userUUID UserUUID, result JoinResult, err error) {
					require.NoError(t, err)
					require.Equal(t, JoinOutcomeLobbyFull, result.Outcome)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenSessionDB(t)
			tc := setup(t, db)
			fx, joinCode, userUUID := tc.before(t, db)

			m := New(db, nil, stubPinnedGameReader{playersMax: 1})
			result, err := m.Join(context.Background(), joinCode, userUUID, "Alice", IdempotencyKey("join-key-"+uuid.NewString()))
			tc.assert(t, db, fx, userUUID, result, err)
		})
	}
}

// TestManagerJoin_Integration_TokenSemantics proves GAME-ADR-0021's
// token-aware Join semantics against a real Postgres database: a repeated
// identical token replays; the same token with different fields conflicts;
// a *different* token while already active is rejected AlreadyJoined
// (never silently replayed or succeeded).
func TestManagerJoin_Integration_TokenSemantics(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 2211, false)
	m := New(db, nil, stubPinnedGameReader{playersMax: 4})
	userUUID := UserUUID(uuid.NewString())

	first, err := m.Join(context.Background(), 2211, userUUID, "Casey", "join-key-replay")
	require.NoError(t, err)
	require.Equal(t, DisplayName("Casey"), first.DisplayName)

	t.Run("same_token_same_fields_replays", func(t *testing.T) {
		replay, err := m.Join(context.Background(), 2211, userUUID, "Casey", "join-key-replay")
		require.NoError(t, err)
		require.Equal(t, first, replay)

		var participantCount int64
		require.NoError(t, db.Raw(`
			SELECT COUNT(*) FROM session_participants p
			INNER JOIN session_actors a ON a.id = p.session_actor_id
			WHERE a.session_id = ? AND a.user_uuid = ?
		`, fx.SessionID, string(userUUID)).Scan(&participantCount).Error)
		require.Equal(t, int64(1), participantCount)
	})

	t.Run("same_token_different_fields_conflicts", func(t *testing.T) {
		_, err := m.Join(context.Background(), 2211, userUUID, "Different Name", "join-key-replay")
		require.ErrorIs(t, err, session.ErrIdempotencyConflict)
	})

	t.Run("different_token_while_already_active_is_rejected", func(t *testing.T) {
		result, err := m.Join(context.Background(), 2211, userUUID, "Casey", "join-key-second")
		require.NoError(t, err)
		require.Equal(t, JoinOutcomeAlreadyJoined, result.Outcome)

		var actorCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE session_id = ? AND user_uuid = ?`, fx.SessionID, string(userUUID)).Scan(&actorCount).Error)
		require.Equal(t, int64(1), actorCount, "same user must never end up with two SessionActor rows for the same Session")
	})
}

// TestManagerJoin_Integration_DifferentUsersSameKeyDoNotCollide proves the
// idempotency namespace is (user_uuid, operation, idempotency_key).
func TestManagerJoin_Integration_DifferentUsersSameKeyDoNotCollide(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 2212, false)
	m := New(db, nil, stubPinnedGameReader{playersMax: 4})

	_, err := m.Join(context.Background(), 2212, UserUUID(uuid.NewString()), "A", "shared-join-key")
	require.NoError(t, err)

	result, err := m.Join(context.Background(), 2212, UserUUID(uuid.NewString()), "B", "shared-join-key")
	require.NoError(t, err)
	require.Equal(t, DisplayName("B"), result.DisplayName)
}

// TestManagerJoin_Integration_PinnedDefinitionImmutability proves Join
// enforces the Session's pinned game_definition_uuid, never the Game's
// current version, by simulating "current version changed" directly in the
// pinned-definition reader rather than through a literal publish workflow.
func TestManagerJoin_Integration_PinnedDefinitionImmutability(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 2213, false)
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Existing")

	pinnedReader := recordingPinnedGameReader{
		playersMax: 1, // V1's players.max, pinned at Create time
	}
	m := New(db, nil, &pinnedReader)

	result, err := m.Join(context.Background(), 2213, UserUUID(uuid.NewString()), "Late Joiner", "join-key-pinned")
	require.NoError(t, err)
	require.Equal(t, JoinOutcomeLobbyFull, result.Outcome, "Join must still enforce the pinned V1 players.max, not a hypothetical current V2")
	require.Equal(t, []string{fx.GameDefinitionUUID}, pinnedReader.requestedUUIDs, "Join must load the Definition by the Session's pinned game_definition_uuid, never by re-resolving the Game's current version")
}

type recordingPinnedGameReader struct {
	playersMax     int
	requestedUUIDs []string
}

func (r *recordingPinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	r.requestedUUIDs = append(r.requestedUUIDs, gameDefinitionUUID)
	return &program.Definition{Players: program.PlayerPolicy{Max: r.playersMax}}, nil
}

// TestManagerJoin_Integration_ConcurrentJoinsForFinalSlot proves - against a
// real Postgres database - that when only one lobby slot remains, exactly
// one of two concurrent Joins succeeds, via the per-Session DB lock rather
// than an application-level check-then-act race.
func TestManagerJoin_Integration_ConcurrentJoinsForFinalSlot(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 6001, false)
	m := New(db, nil, stubPinnedGameReader{playersMax: 1})

	userA := UserUUID(uuid.NewString())
	userB := UserUUID(uuid.NewString())

	start := make(chan struct{})
	var wg sync.WaitGroup
	var resA, resB JoinResult
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		resA, errA = m.Join(context.Background(), 6001, userA, "A", "concurrent-join-a")
	}()
	go func() {
		defer wg.Done()
		<-start
		resB, errB = m.Join(context.Background(), 6001, userB, "B", "concurrent-join-b")
	}()
	close(start)
	wg.Wait()

	require.NoError(t, errA)
	require.NoError(t, errB)

	successCount := 0
	if resA.Outcome == JoinOutcomeJoined {
		successCount++
		require.NotEmpty(t, resA.DisplayName)
	} else {
		require.Equal(t, JoinOutcomeLobbyFull, resA.Outcome)
	}
	if resB.Outcome == JoinOutcomeJoined {
		successCount++
		require.NotEmpty(t, resB.DisplayName)
	} else {
		require.Equal(t, JoinOutcomeLobbyFull, resB.Outcome)
	}
	require.Equal(t, 1, successCount, "exactly one Join must succeed when only one slot remains")

	var activeCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ? AND p.active = TRUE
	`, fx.SessionID).Scan(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
}

// TestManagerJoin_Integration_ConcurrentJoinRacingLeave proves - through the
// real public Join/Leave Manager methods against a real Postgres database -
// that a Join racing a Leave around capacity resolves correctly under the
// same per-Session DB lock: whichever operation wins serialization
// determines a consistent, non-corrupted final state.
func TestManagerJoin_Integration_ConcurrentJoinRacingLeave(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db, nil, stubPinnedGameReader{playersMax: 1})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 7001, false)

	existingUser := UserUUID(uuid.NewString())
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, string(existingUser), "Existing")
	newUser := UserUUID(uuid.NewString())

	start := make(chan struct{})
	var wg sync.WaitGroup
	var joinResult JoinResult
	var joinErr, leaveErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		joinResult, joinErr = m.Join(context.Background(), 7001, newUser, "New", "race-join")
	}()
	go func() {
		defer wg.Done()
		<-start
		_, leaveErr = m.Leave(context.Background(), SessionUUID(fx.SessionUUID), existingUser, "race-leave")
	}()
	close(start)
	wg.Wait()

	require.NoError(t, leaveErr, "Leave has no reason to fail regardless of ordering")
	require.NoError(t, joinErr)

	var activeCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ? AND p.active = TRUE
	`, fx.SessionID).Scan(&activeCount).Error)

	if joinResult.Outcome == JoinOutcomeJoined {
		require.NotEmpty(t, joinResult.DisplayName)
		require.Equal(t, int64(1), activeCount, "Join won the race after Leave released the slot")
	} else {
		require.Equal(t, JoinOutcomeLobbyFull, joinResult.Outcome)
		require.Equal(t, int64(0), activeCount, "Join lost the race while the existing Participant still held the only slot")
	}
}

// TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration proves
// that two concurrent operations racing an already-expired (but not yet
// materialized) lobby never revive it: both must observe the expiration, and
// the Session ends up TERMINAL exactly once, via the same per-Session lock
// rather than two independent lazy-materialization attempts corrupting each
// other.
func TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db, nil, stubPinnedGameReader{playersMax: 4})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 7002, false)

	existingUser := UserUUID(uuid.NewString())
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, string(existingUser), "Existing")
	newUser := UserUUID(uuid.NewString())

	start := make(chan struct{})
	var wg sync.WaitGroup
	var joinResult JoinResult
	var leaveResult LeaveResult
	var joinErr, leaveErr error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		joinResult, joinErr = m.Join(context.Background(), 7002, newUser, "New", "race-expire-join")
	}()
	go func() {
		defer wg.Done()
		<-start
		leaveResult, leaveErr = m.Leave(context.Background(), SessionUUID(fx.SessionUUID), existingUser, "race-expire-leave")
	}()
	close(start)
	wg.Wait()

	require.NoError(t, joinErr)
	require.NoError(t, leaveErr)
	require.Equal(t, JoinOutcomeLobbyExpired, joinResult.Outcome)
	require.Equal(t, LeaveOutcomeNotInLobby, leaveResult.Outcome)

	var phase, terminalReason string
	require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, fx.SessionID).Scan(&phase).Error)
	require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE id = ?`, fx.SessionID).Scan(&terminalReason).Error)
	require.Equal(t, session.PhaseTerminal, phase)
	require.Equal(t, session.TerminalReasonLobbyExpired, terminalReason)
}
