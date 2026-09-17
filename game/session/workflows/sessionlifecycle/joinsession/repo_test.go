package joinsession

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepoResolveActiveSessionForJoinCode(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := newRepo(db)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 4242, false)
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 5252, true)

	t.Run("resolves_active_code", func(t *testing.T) {
		resolution, err := repo.resolveActiveSessionForJoinCode(context.Background(), 4242)
		require.NoError(t, err)
		require.NotNil(t, resolution)
		require.Equal(t, fx.SessionID, resolution.SessionID)
		require.Equal(t, fx.GameDefinitionUUID, resolution.GameDefinitionUUID)
	})

	t.Run("returns_nil_for_revoked_code", func(t *testing.T) {
		resolution, err := repo.resolveActiveSessionForJoinCode(context.Background(), 5252)
		require.NoError(t, err)
		require.Nil(t, resolution)
	})

	t.Run("returns_nil_for_unknown_code", func(t *testing.T) {
		resolution, err := repo.resolveActiveSessionForJoinCode(context.Background(), 9090)
		require.NoError(t, err)
		require.Nil(t, resolution)
	})
}

func TestRepoJoinSession(t *testing.T) {
	type test struct {
		before func(t *testing.T, db *gorm.DB) joinParams
		assert func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error)
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"creates_actor_and_activates_participant": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) joinParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					return joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       1111,
						UserUUID:       uuid.NewString(),
						DisplayName:    "Alice",
						IdempotencyKey: "join-key-1",
						PlayersMax:     4,
					}
				},
				assert: func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error) {
					require.NoError(t, err)
					require.Equal(t, "Alice", result.DisplayName)

					var actorCount int64
					require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE session_id = ? AND user_uuid = ?`, params.SessionID, params.UserUUID).Scan(&actorCount).Error)
					require.Equal(t, int64(1), actorCount)

					var active bool
					require.NoError(t, db.Raw(`
						SELECT p.active FROM session_participants p
						INNER JOIN session_actors a ON a.id = p.session_actor_id
						WHERE a.session_id = ? AND a.user_uuid = ?
					`, params.SessionID, params.UserUUID).Scan(&active).Error)
					require.True(t, active)
				},
			}
		},
		"repeated_active_join_does_not_duplicate_actor_or_slot": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) joinParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					params := joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       2222,
						UserUUID:       uuid.NewString(),
						DisplayName:    "Bob",
						IdempotencyKey: "join-key-first",
						PlayersMax:     4,
					}
					r := newRepo(db)
					_, err := r.joinSession(context.Background(), params)
					require.NoError(t, err)

					// Second Join call: different idempotency key (a fresh
					// logical Join attempt, not merely an idempotent
					// replay), but the same actor already holds an active
					// Participant.
					params.IdempotencyKey = "join-key-second"
					return params
				},
				assert: func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error) {
					require.NoError(t, err)

					var actorCount int64
					require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE session_id = ? AND user_uuid = ?`, params.SessionID, params.UserUUID).Scan(&actorCount).Error)
					require.Equal(t, int64(1), actorCount, "same user must never end up with two SessionActor rows for the same Session")

					var participantCount int64
					require.NoError(t, db.Raw(`
						SELECT COUNT(*) FROM session_participants p
						INNER JOIN session_actors a ON a.id = p.session_actor_id
						WHERE a.session_id = ? AND a.user_uuid = ?
					`, params.SessionID, params.UserUUID).Scan(&participantCount).Error)
					require.Equal(t, int64(1), participantCount, "repeated active Join must not consume another slot")
				},
			}
		},
		"repeated_identical_join_with_same_idempotency_key_replays_result": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) joinParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					params := joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       2211,
						UserUUID:       uuid.NewString(),
						DisplayName:    "Casey",
						IdempotencyKey: "join-key-replay",
						PlayersMax:     4,
					}
					r := newRepo(db)
					first, err := r.joinSession(context.Background(), params)
					require.NoError(t, err)
					require.Equal(t, "Casey", first.DisplayName)
					return params
				},
				assert: func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error) {
					require.NoError(t, err)
					require.Equal(t, "Casey", result.DisplayName)

					var participantCount int64
					require.NoError(t, db.Raw(`
						SELECT COUNT(*) FROM session_participants p
						INNER JOIN session_actors a ON a.id = p.session_actor_id
						WHERE a.session_id = ? AND a.user_uuid = ?
					`, params.SessionID, params.UserUUID).Scan(&participantCount).Error)
					require.Equal(t, int64(1), participantCount)
				},
			}
		},
		"conflicting_join_with_same_idempotency_identity_is_rejected": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) joinParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					userUUID := uuid.NewString()
					r := newRepo(db)
					_, err := r.joinSession(context.Background(), joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       2311,
						UserUUID:       userUUID,
						DisplayName:    "Dana",
						IdempotencyKey: "join-key-conflict",
						PlayersMax:     4,
					})
					require.NoError(t, err)

					return joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       9999,
						UserUUID:       userUUID,
						DisplayName:    "Different Name",
						IdempotencyKey: "join-key-conflict",
						PlayersMax:     4,
					}
				},
				assert: func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error) {
					require.ErrorIs(t, err, session.ErrIdempotencyConflict)
				},
			}
		},
		"enforces_players_max": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) joinParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Existing Player")
					return joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       3333,
						UserUUID:       uuid.NewString(),
						DisplayName:    "Late Joiner",
						IdempotencyKey: "join-key-full",
						PlayersMax:     1,
					}
				},
				assert: func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error) {
					require.ErrorIs(t, err, session.ErrLobbyFull)
				},
			}
		},
		"lazily_materializes_expired_lobby_and_rejects": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) joinParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
					return joinParams{
						SessionID:      fx.SessionID,
						JoinCode:       4444,
						UserUUID:       uuid.NewString(),
						DisplayName:    "Too Late",
						IdempotencyKey: "join-key-expired",
						PlayersMax:     4,
					}
				},
				assert: func(t *testing.T, db *gorm.DB, params joinParams, result Result, err error) {
					require.ErrorIs(t, err, session.ErrLobbyExpired)

					var phase string
					require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE id = ?`, params.SessionID).Scan(&phase).Error)
					require.Equal(t, session.PhaseTerminal, phase)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenSessionDB(t)
			repo := newRepo(db)
			tc := setup(t, db)
			params := tc.before(t, db)

			result, err := repo.joinSession(context.Background(), params)
			tc.assert(t, db, params, result, err)
		})
	}
}

// TestRepoJoinSession_ConcurrentJoinsForFinalSlot proves - against a real
// Postgres database - that when only one lobby slot remains, exactly one of
// two concurrent Joins succeeds, via the per-Session DB lock rather than an
// application-level check-then-act race.
func TestRepoJoinSession_ConcurrentJoinsForFinalSlot(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := newRepo(db)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))

	userA := uuid.NewString()
	userB := uuid.NewString()

	start := make(chan struct{})
	var wg sync.WaitGroup
	var resA, resB Result
	var errA, errB error

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		resA, errA = repo.joinSession(context.Background(), joinParams{
			SessionID: fx.SessionID, JoinCode: 6001, UserUUID: userA, DisplayName: "A", IdempotencyKey: "concurrent-join-a", PlayersMax: 1,
		})
	}()
	go func() {
		defer wg.Done()
		<-start
		resB, errB = repo.joinSession(context.Background(), joinParams{
			SessionID: fx.SessionID, JoinCode: 6002, UserUUID: userB, DisplayName: "B", IdempotencyKey: "concurrent-join-b", PlayersMax: 1,
		})
	}()
	close(start)
	wg.Wait()

	successCount := 0
	if errA == nil {
		successCount++
		require.NotEmpty(t, resA.DisplayName)
	} else {
		require.ErrorIs(t, errA, session.ErrLobbyFull)
	}
	if errB == nil {
		successCount++
		require.NotEmpty(t, resB.DisplayName)
	} else {
		require.ErrorIs(t, errB, session.ErrLobbyFull)
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
