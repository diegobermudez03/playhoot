package leavesession

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepoLeaveSession(t *testing.T) {
	type test struct {
		before func(t *testing.T, db *gorm.DB) leaveParams
		assert func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error)
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"active_participant_leaves_and_slot_is_released": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) leaveParams {
					userUUID := uuid.NewString()
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User One")
					return leaveParams{SessionUUID: fx.SessionUUID, UserUUID: userUUID, IdempotencyKey: "leave-key-1"}
				},
				assert: func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error) {
					require.NoError(t, err)
					require.Equal(t, params.SessionUUID, result.SessionUUID)

					var active bool
					require.NoError(t, db.Raw(`
						SELECT p.active FROM session_participants p
						INNER JOIN session_actors a ON a.id = p.session_actor_id
						WHERE a.user_uuid = ?
					`, params.UserUUID).Scan(&active).Error)
					require.False(t, active)

					var actorCount int64
					require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE user_uuid = ?`, params.UserUUID).Scan(&actorCount).Error)
					require.Equal(t, int64(1), actorCount, "SessionActor must remain durable after Leave")
				},
			}
		},
		"host_retains_host_authority_after_leaving": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) leaveParams {
					hostUUID := uuid.NewString()
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
					require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
					testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
					return leaveParams{SessionUUID: fx.SessionUUID, UserUUID: hostUUID, IdempotencyKey: "leave-key-host"}
				},
				assert: func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error) {
					require.NoError(t, err)

					var hostActorID *uint
					require.NoError(t, db.Raw(`SELECT host_actor_id FROM sessions WHERE uuid = ?`, params.SessionUUID).Scan(&hostActorID).Error)
					require.NotNil(t, hostActorID)
				},
			}
		},
		"retried_leave_with_same_idempotency_key_is_idempotent": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) leaveParams {
					userUUID := uuid.NewString()
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User Two")
					params := leaveParams{SessionUUID: fx.SessionUUID, UserUUID: userUUID, IdempotencyKey: "leave-key-retry"}

					r := newRepo(db)
					_, err := r.leaveSession(context.Background(), params)
					require.NoError(t, err)
					return params
				},
				assert: func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error) {
					require.NoError(t, err)
					require.Equal(t, params.SessionUUID, result.SessionUUID)
				},
			}
		},
		"rejects_leave_when_session_already_terminal": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) leaveParams {
					userUUID := uuid.NewString()
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User Three")
					require.NoError(t, db.Exec(`UPDATE sessions SET phase = 'TERMINAL' WHERE id = ?`, fx.SessionID).Error)
					return leaveParams{SessionUUID: fx.SessionUUID, UserUUID: userUUID, IdempotencyKey: "leave-key-terminal"}
				},
				assert: func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error) {
					require.ErrorIs(t, err, session.ErrNotInLobbyPhase)
				},
			}
		},
		"lazily_materializes_expired_lobby_and_rejects": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) leaveParams {
					userUUID := uuid.NewString()
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
					testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User Four")
					return leaveParams{SessionUUID: fx.SessionUUID, UserUUID: userUUID, IdempotencyKey: "leave-key-expired"}
				},
				assert: func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error) {
					require.ErrorIs(t, err, session.ErrNotInLobbyPhase)

					var phase string
					require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, params.SessionUUID).Scan(&phase).Error)
					require.Equal(t, session.PhaseTerminal, phase)
				},
			}
		},
		"rejects_when_actor_never_joined": func(t *testing.T, db *gorm.DB) test {
			return test{
				before: func(t *testing.T, db *gorm.DB) leaveParams {
					fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
					return leaveParams{SessionUUID: fx.SessionUUID, UserUUID: uuid.NewString(), IdempotencyKey: "leave-key-missing"}
				},
				assert: func(t *testing.T, db *gorm.DB, params leaveParams, result Result, err error) {
					require.ErrorIs(t, err, session.ErrActorNotFound)
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

			result, err := repo.leaveSession(context.Background(), params)
			tc.assert(t, db, params, result, err)
		})
	}
}
