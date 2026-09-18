package sessionlifecycle

import (
	"context"
	"sync"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestManagerCreate_Integration exercises the real public Manager.Create
// against a real Postgres database (not mocked collaborators), proving the
// full Host/SessionActor Creation Cycle, JoinCode issuance, and idempotency
// persistence WORK-0001 requires.
func TestManagerCreate_Integration(t *testing.T) {
	type test struct {
		gameUUID       GameUUID
		hostUserUUID   UserUUID
		idempotencyKey IdempotencyKey
		assert         func(t *testing.T, db *gorm.DB, result CreatedSession, err error)
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"creates_lobby_session_host_actor_and_active_join_code": func(t *testing.T, db *gorm.DB) test {
			return test{
				gameUUID: GameUUID(uuid.NewString()), hostUserUUID: UserUUID(uuid.NewString()), idempotencyKey: "create-key-1",
				assert: func(t *testing.T, db *gorm.DB, result CreatedSession, err error) {
					require.NoError(t, err)
					require.NotEmpty(t, result.SessionUUID)
					require.GreaterOrEqual(t, uint(result.JoinCode), uint(1000))
					require.LessOrEqual(t, uint(result.JoinCode), uint(9999))

					var row struct {
						ID          uint
						Phase       string
						HostActorID *uint
					}
					require.NoError(t, db.Raw(`SELECT id, phase, host_actor_id FROM sessions WHERE uuid = ?`, result.SessionUUID).Scan(&row).Error)
					require.Equal(t, session.PhaseLobby, row.Phase)
					require.NotNil(t, row.HostActorID)

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
				},
			}
		},
		"a_game_definition_that_fails_to_compile_is_rejected": func(t *testing.T, db *gorm.DB) test {
			return test{
				gameUUID: GameUUID(uuid.NewString()), hostUserUUID: UserUUID(uuid.NewString()), idempotencyKey: "create-key-broken",
				assert: func(t *testing.T, db *gorm.DB, result CreatedSession, err error) {
					require.ErrorIs(t, err, session.ErrDefinitionDoesNotCompile)
					require.Empty(t, result.SessionUUID)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenSessionDB(t)
			tc := setup(t, db)

			definition := compilableDefinitionForTest(4)
			if name == "a_game_definition_that_fails_to_compile_is_rejected" {
				definition = uncompilableDefinitionForTest()
			}
			reader := stubCurrentGameReader{versionUUID: uuid.NewString(), definition: definition}

			m := New(db, reader, stubPinnedGameReader{})
			result, err := m.Create(context.Background(), tc.gameUUID, tc.hostUserUUID, tc.idempotencyKey)
			tc.assert(t, db, result, err)
		})
	}
}

func TestManagerCreate_Integration_RepeatedSemanticallyEquivalentCreateReplays(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	reader := stubCurrentGameReader{versionUUID: uuid.NewString(), definition: compilableDefinitionForTest(4)}
	m := New(db, reader, stubPinnedGameReader{})

	gameUUID := GameUUID(uuid.NewString())
	hostUserUUID := UserUUID(uuid.NewString())

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
	reader := stubCurrentGameReader{versionUUID: uuid.NewString(), definition: compilableDefinitionForTest(4)}
	m := New(db, reader, stubPinnedGameReader{})

	hostUserUUID := UserUUID(uuid.NewString())
	_, err := m.Create(context.Background(), GameUUID(uuid.NewString()), hostUserUUID, "create-key-conflict")
	require.NoError(t, err)

	_, err = m.Create(context.Background(), GameUUID(uuid.NewString()), hostUserUUID, "create-key-conflict")
	require.ErrorIs(t, err, session.ErrIdempotencyConflict)
}

func TestManagerCreate_Integration_DifferentUsersReusingSameKeyDoNotCollide(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	reader := stubCurrentGameReader{versionUUID: uuid.NewString(), definition: compilableDefinitionForTest(4)}
	m := New(db, reader, stubPinnedGameReader{})

	gameUUID := GameUUID(uuid.NewString())
	_, err := m.Create(context.Background(), gameUUID, UserUUID(uuid.NewString()), "shared-key")
	require.NoError(t, err)

	result, err := m.Create(context.Background(), gameUUID, UserUUID(uuid.NewString()), "shared-key")
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionUUID)
}

// TestManagerCreate_Integration_Concurrent proves - against a real Postgres
// database - that two concurrent Manager.Create calls sharing the same
// (user_uuid, CREATE, idempotency_key) produce exactly one Session and both
// observe the identical logical result (WORK-0001's Concurrent Create
// Correctness).
func TestManagerCreate_Integration_Concurrent(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	reader := stubCurrentGameReader{versionUUID: uuid.NewString(), definition: compilableDefinitionForTest(4)}
	m := New(db, reader, stubPinnedGameReader{})

	gameUUID := GameUUID(uuid.NewString())
	hostUserUUID := UserUUID(uuid.NewString())

	const attempts = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]CreatedSession, attempts)
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

	var successResult *CreatedSession
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
	`, string(hostUserUUID), operationCreate, "concurrent-create-key").Scan(&requestCount).Error)
	require.Equal(t, int64(1), requestCount, "exactly one idempotency record must exist for the shared identity")
}
