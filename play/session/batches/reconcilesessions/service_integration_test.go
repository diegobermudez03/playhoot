package reconcilesessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestReconcileSessionsIntegration(t *testing.T) {
	type test struct {
		sessionID uint
		// held keeps the Session's row locked by another transaction while
		// reconciliation runs.
		held bool

		expectErr           bool
		expectedStatus      sessionStatus
		expectedHistory     []string
		expectedActiveCodes int
	}

	db := testdb.OpenSessionDB(t)
	now := time.Now().UTC()
	text := func(value string) *string { return &value }
	// running seeds a RUNNING Session whose owner lease runs out at lease
	// from now, its join code released as a started Session's is.
	running := func(t *testing.T, lease time.Duration) uint {
		sessionID := seedSession(t, db, "RUNNING", now.Add(time.Hour))
		require.NoError(t, db.Exec(`UPDATE play_sessions SET owner_lease_expires_at = ? WHERE id = ?`, now.Add(lease), sessionID).Error)
		require.NoError(t, db.Exec(`UPDATE play_session_join_codes SET released_at = ? WHERE session_id = ?`, now, sessionID).Error)
		return sessionID
	}

	tests := map[string]func(t *testing.T) test{
		"expired_lobby_expires_releasing_its_join_code": func(t *testing.T) test {
			return test{
				sessionID:       seedSession(t, db, "LOBBY", now.Add(-time.Second)),
				expectedStatus:  sessionStatus{Status: "EXPIRED", EndCause: text("LOBBY_EXPIRED")},
				expectedHistory: []string{"LOBBY -> EXPIRED LOBBY_EXPIRED by platform"},
			}
		},
		"lobby_before_its_expiry_stays": func(t *testing.T) test {
			return test{
				sessionID:           seedSession(t, db, "LOBBY", now.Add(time.Hour)),
				expectedStatus:      sessionStatus{Status: "LOBBY"},
				expectedActiveCodes: 1,
			}
		},
		"finished_game_whose_lease_ran_out_finishes": func(t *testing.T) test {
			sessionID := running(t, -time.Minute)
			require.NoError(t, db.Exec(`
				INSERT INTO play_replays (session_id, game_version_id, final_session_time_ms, finalized_at)
				SELECT id, game_version_id, 5000, ? FROM play_sessions WHERE id = ?
			`, now, sessionID).Error)
			return test{
				sessionID:       sessionID,
				expectedStatus:  sessionStatus{Status: "FINISHED", EndCause: text("GAME_FINISHED")},
				expectedHistory: []string{"RUNNING -> FINISHED GAME_FINISHED by platform"},
			}
		},
		"game_past_its_lifetime_expires": func(t *testing.T) test {
			sessionID := running(t, time.Minute)
			require.NoError(t, db.Exec(`UPDATE play_sessions SET expires_at = ? WHERE id = ?`, now.Add(-time.Second), sessionID).Error)
			return test{
				sessionID:       sessionID,
				expectedStatus:  sessionStatus{Status: "EXPIRED", EndCause: text("MAX_LIFETIME")},
				expectedHistory: []string{"RUNNING -> EXPIRED MAX_LIFETIME by platform"},
			}
		},
		"game_whose_owner_lease_ran_out_is_aborted": func(t *testing.T) test {
			return test{
				sessionID:       running(t, -time.Second),
				expectedStatus:  sessionStatus{Status: "ABORTED", EndCause: text("OWNER_LOST")},
				expectedHistory: []string{"RUNNING -> ABORTED OWNER_LOST by platform"},
			}
		},
		"game_with_its_lease_left_stays": func(t *testing.T) test {
			return test{sessionID: running(t, time.Minute), expectedStatus: sessionStatus{Status: "RUNNING"}}
		},
		"session_held_too_long_by_another_transaction_waits_for_the_next_pass": func(t *testing.T) test {
			return test{
				sessionID:           seedSession(t, db, "LOBBY", now.Add(-time.Second)),
				held:                true,
				expectErr:           true,
				expectedStatus:      sessionStatus{Status: "LOBBY"},
				expectedActiveCodes: 1,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			service := New(db)
			// Other tests' Sessions share the database: judge them all.
			service.batch = 100000
			service.lockTimeout = 200 * time.Millisecond
			if tc.held {
				holder := db.Begin()
				require.NoError(t, holder.Exec(`SELECT id FROM play_sessions WHERE id = ? FOR UPDATE`, tc.sessionID).Error)
				defer holder.Rollback()
			}

			_, err := service.ReconcileSessions(context.Background())

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedStatus, statusOf(t, db, tc.sessionID))
			require.Equal(t, tc.expectedActiveCodes, activeCodes(t, db, tc.sessionID))
			require.Equal(t, tc.expectedHistory, statusHistory(t, db, tc.sessionID))
		})
	}
}

// TestReconcileSessionsDrainsEveryDueSession reads two due Sessions at a time:
// one pass still ends every one of them, past one it cannot lock.
func TestReconcileSessionsDrainsEveryDueSession(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	expired := time.Now().UTC().Add(-time.Second)
	var due []uint
	for range 5 {
		due = append(due, seedSession(t, db, "LOBBY", expired))
	}
	held := due[2]
	holder := db.Begin()
	require.NoError(t, holder.Exec(`SELECT id FROM play_sessions WHERE id = ? FOR UPDATE`, held).Error)
	defer holder.Rollback()
	service := New(db)
	service.batch, service.lockTimeout = 2, 200*time.Millisecond

	_, err := service.ReconcileSessions(context.Background())

	require.Error(t, err, "the held Session is reported")
	for _, sessionID := range due {
		if sessionID == held {
			require.Equal(t, "LOBBY", statusOf(t, db, sessionID).Status, "a Session held too long waits for the next pass")
			continue
		}
		require.Equal(t, "EXPIRED", statusOf(t, db, sessionID).Status)
	}
}

type sessionStatus struct {
	Status   string
	EndCause *string
}

// seedSession creates a Session in status, whose lobby expires at
// lobbyExpiresAt, with an active join code, and returns its id.
func seedSession(t *testing.T, db *gorm.DB, status string, lobbyExpiresAt time.Time) uint {
	t.Helper()
	var gameID, versionID, sessionID uint
	require.NoError(t, db.Raw(`INSERT INTO play_games (game_uuid) VALUES (?) RETURNING id`, uuid.NewString()).Scan(&gameID).Error)
	require.NoError(t, db.Raw(`
		INSERT INTO play_game_versions (uuid, game_id, status, manifest)
		VALUES (?, ?, 'EXECUTABLE', '{"schema_version":1,"players":{"min":1,"max":4}}') RETURNING id
	`, uuid.NewString(), gameID).Scan(&versionID).Error)
	require.NoError(t, db.Raw(`
		INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, uuid.NewString(), versionID, uuid.NewString(), status, lobbyExpiresAt, lobbyExpiresAt.Add(4*time.Hour)).Scan(&sessionID).Error)
	code := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
	require.NoError(t, db.Exec(`INSERT INTO play_session_join_codes (session_id, code, expires_at) VALUES (?, ?, ?)`, sessionID, code, lobbyExpiresAt).Error)
	return sessionID
}

func statusOf(t *testing.T, db *gorm.DB, sessionID uint) sessionStatus {
	t.Helper()
	var status sessionStatus
	require.NoError(t, db.Raw(`SELECT status, end_cause FROM play_sessions WHERE id = ?`, sessionID).Scan(&status).Error)
	return status
}

func activeCodes(t *testing.T, db *gorm.DB, sessionID uint) int {
	t.Helper()
	var count int
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM play_session_join_codes WHERE session_id = ? AND released_at IS NULL`, sessionID).Scan(&count).Error)
	return count
}

// statusHistory is sessionID's status changes, oldest first, as
// "FROM -> TO [cause] by <user | platform>".
func statusHistory(t *testing.T, db *gorm.DB, sessionID uint) []string {
	t.Helper()
	var changes []struct {
		FromStatus        *string
		ToStatus          string
		EndCause          *string
		ChangedByUserUUID *string
	}
	require.NoError(t, db.Raw(`
		SELECT from_status, to_status, end_cause, changed_by_user_uuid
		FROM play_session_status_changes WHERE session_id = ? ORDER BY id
	`, sessionID).Scan(&changes).Error)
	var history []string
	for _, change := range changes {
		entry := "CREATED"
		if change.FromStatus != nil {
			entry = *change.FromStatus
		}
		entry += " -> " + change.ToStatus
		if change.EndCause != nil {
			entry += " " + *change.EndCause
		}
		by := "platform"
		if change.ChangedByUserUUID != nil {
			by = *change.ChangedByUserUUID
		}
		history = append(history, entry+" by "+by)
	}
	return history
}
