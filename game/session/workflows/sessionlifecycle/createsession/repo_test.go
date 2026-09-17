package createsession

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepoCreateSession(t *testing.T) {
	type test struct {
		ctx    context.Context
		params createParams
		assert func(t *testing.T, db *gorm.DB, result Result, err error)
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"creates_lobby_session_host_actor_and_active_join_code": func(t *testing.T, db *gorm.DB) test {
			return test{
				ctx: context.Background(),
				params: createParams{
					GameUUID:           "11111111-1111-1111-1111-111111111111",
					HostUserUUID:       "22222222-2222-2222-2222-222222222222",
					IdempotencyKey:     "create-key-1",
					GameDefinitionUUID: "33333333-3333-3333-3333-333333333333",
					LobbyTTL:           10 * time.Minute,
				},
				assert: func(t *testing.T, db *gorm.DB, result Result, err error) {
					require.NoError(t, err)
					require.NotEmpty(t, result.SessionUUID)
					require.GreaterOrEqual(t, result.JoinCode, uint(1000))
					require.LessOrEqual(t, result.JoinCode, uint(9999))

					var row sessionRowForAssert
					require.NoError(t, db.Raw(`
						SELECT id, uuid, game_definition_uuid, host_actor_id, phase
						FROM sessions WHERE uuid = ?
					`, result.SessionUUID).Scan(&row).Error)
					require.Equal(t, session.PhaseLobby, row.Phase)
					require.NotNil(t, row.HostActorID)
					require.Equal(t, "33333333-3333-3333-3333-333333333333", row.GameDefinitionUUID)

					var actorCount int64
					require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE session_id = ?`, row.ID).Scan(&actorCount).Error)
					require.Equal(t, int64(1), actorCount)

					var participantCount int64
					require.NoError(t, db.Raw(`
						SELECT COUNT(*) FROM session_participants p
						INNER JOIN session_actors a ON a.id = p.session_actor_id
						WHERE a.session_id = ?
					`, row.ID).Scan(&participantCount).Error)
					require.Equal(t, int64(0), participantCount, "host must not be created as an active participant")

					var activeJoinCodeCount int64
					require.NoError(t, db.Raw(`
						SELECT COUNT(*) FROM join_codes WHERE session_id = ? AND revoked_at IS NULL
					`, row.ID).Scan(&activeJoinCodeCount).Error)
					require.Equal(t, int64(1), activeJoinCodeCount)

					var requestStatus string
					require.NoError(t, db.Raw(`
						SELECT status FROM session_requests
						WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
					`, "22222222-2222-2222-2222-222222222222", operationCreate, "create-key-1").Scan(&requestStatus).Error)
					require.Equal(t, "COMPLETED", requestStatus)
				},
			}
		},
		"repeated_semantically_equivalent_create_replays_same_result": func(t *testing.T, db *gorm.DB) test {
			r := newRepo(db)
			first, err := r.createSession(context.Background(), createParams{
				GameUUID:           "44444444-4444-4444-4444-444444444444",
				HostUserUUID:       "55555555-5555-5555-5555-555555555555",
				IdempotencyKey:     "create-key-replay",
				GameDefinitionUUID: "66666666-6666-6666-6666-666666666666",
				LobbyTTL:           10 * time.Minute,
			})
			require.NoError(t, err)

			return test{
				ctx: context.Background(),
				params: createParams{
					GameUUID:           "44444444-4444-4444-4444-444444444444",
					HostUserUUID:       "55555555-5555-5555-5555-555555555555",
					IdempotencyKey:     "create-key-replay",
					GameDefinitionUUID: "66666666-6666-6666-6666-666666666666",
					LobbyTTL:           10 * time.Minute,
				},
				assert: func(t *testing.T, db *gorm.DB, result Result, err error) {
					require.NoError(t, err)
					require.Equal(t, first, result)

					var sessionCount int64
					require.NoError(t, db.Raw(`SELECT COUNT(*) FROM sessions WHERE uuid = ?`, first.SessionUUID).Scan(&sessionCount).Error)
					require.Equal(t, int64(1), sessionCount)
				},
			}
		},
		"conflicting_create_with_same_idempotency_identity_is_rejected": func(t *testing.T, db *gorm.DB) test {
			r := newRepo(db)
			_, err := r.createSession(context.Background(), createParams{
				GameUUID:           "77777777-7777-7777-7777-777777777777",
				HostUserUUID:       "88888888-8888-8888-8888-888888888888",
				IdempotencyKey:     "create-key-conflict",
				GameDefinitionUUID: "99999999-9999-9999-9999-999999999999",
				LobbyTTL:           10 * time.Minute,
			})
			require.NoError(t, err)

			return test{
				ctx: context.Background(),
				params: createParams{
					GameUUID:           "different-game-uuid",
					HostUserUUID:       "88888888-8888-8888-8888-888888888888",
					IdempotencyKey:     "create-key-conflict",
					GameDefinitionUUID: "different-version-uuid",
					LobbyTTL:           10 * time.Minute,
				},
				assert: func(t *testing.T, db *gorm.DB, result Result, err error) {
					require.ErrorIs(t, err, session.ErrIdempotencyConflict)
				},
			}
		},
		"different_users_reusing_same_idempotency_key_do_not_collide": func(t *testing.T, db *gorm.DB) test {
			r := newRepo(db)
			_, err := r.createSession(context.Background(), createParams{
				GameUUID:           "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				HostUserUUID:       "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
				IdempotencyKey:     "shared-key",
				GameDefinitionUUID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
				LobbyTTL:           10 * time.Minute,
			})
			require.NoError(t, err)

			return test{
				ctx: context.Background(),
				params: createParams{
					GameUUID:           "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
					HostUserUUID:       "dddddddd-dddd-dddd-dddd-dddddddddddd",
					IdempotencyKey:     "shared-key",
					GameDefinitionUUID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
					LobbyTTL:           10 * time.Minute,
				},
				assert: func(t *testing.T, db *gorm.DB, result Result, err error) {
					require.NoError(t, err)
					require.NotEmpty(t, result.SessionUUID)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenSessionDB(t)
			repo := newRepo(db)
			tc := setup(t, db)

			result, err := repo.createSession(tc.ctx, tc.params)
			tc.assert(t, db, result, err)
		})
	}
}

// TestRepoCreateSession_ConcurrentSameIdempotencyIdentity proves - against a
// real Postgres database, not a mocked driver - that two concurrent
// CreateSession calls sharing the same (user_uuid, CREATE, idempotency_key)
// produce exactly one Session and both observe the identical logical result,
// per WORK-0001's Concurrent Create Correctness section.
func TestRepoCreateSession_ConcurrentSameIdempotencyIdentity(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := newRepo(db)

	params := createParams{
		GameUUID:           "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
		HostUserUUID:       "ffffffff-ffff-ffff-ffff-ffffffffffff",
		IdempotencyKey:     "concurrent-create-key",
		GameDefinitionUUID: "10101010-1010-1010-1010-101010101010",
		LobbyTTL:           10 * time.Minute,
	}

	const attempts = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]Result, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.createSession(context.Background(), params)
		}(i)
	}
	close(start)
	wg.Wait()

	var successResult *Result
	for i := 0; i < attempts; i++ {
		require.NoError(t, errs[i], "attempt %d should not fail under the claim-then-create design", i)
		if successResult == nil {
			successResult = &results[i]
		} else {
			require.Equal(t, *successResult, results[i], "attempt %d observed a different logical result", i)
		}
	}

	var sessionCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM sessions WHERE uuid = ?`, successResult.SessionUUID).Scan(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)

	var requestCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_requests
		WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
	`, params.HostUserUUID, operationCreate, params.IdempotencyKey).Scan(&requestCount).Error)
	require.Equal(t, int64(1), requestCount, "exactly one idempotency record must exist for the shared identity")
}

type sessionRowForAssert struct {
	ID                 uint
	UUID               string
	GameDefinitionUUID string
	HostActorID        *uint
	Phase              string
}
