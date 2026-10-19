package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestRecordOwnerHeartbeatIntegration(t *testing.T) {
	type test struct {
		session startableSession

		expected sessionpkg.OwnerHeartbeatResult
		// expectedBeat means the heartbeat was stored.
		expectedBeat bool
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()

	tests := map[string]func(t *testing.T) test{
		"running_session_records_it": func(t *testing.T) test {
			return test{
				session:      seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				expected:     sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatRecorded},
				expectedBeat: true,
			}
		},
		"ended_session_tells_how_it_ended": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour))
			_, err := New(db, store).EndSession(context.Background(), session.UUID, sessionpkg.SessionEnd{Cause: sessionpkg.EndOwnerLost})
			require.NoError(t, err)
			return test{
				session:  session,
				expected: sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatSessionNotRunning, Status: "ABORTED", EndCause: "OWNER_LOST"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			result, err := New(db, store).RecordOwnerHeartbeat(context.Background(), tc.session.UUID)

			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
			require.Equal(t, tc.expectedBeat, ownerLeaseOf(t, db, tc.session.ID) != nil)
		})
	}
}

// ownerLeaseOf returns when sessionID's owner lease runs out; nil if it has
// none.
func ownerLeaseOf(t *testing.T, db *gorm.DB, sessionID uint) *time.Time {
	t.Helper()
	var lease struct{ OwnerLeaseExpiresAt *time.Time }
	require.NoError(t, db.Raw(`SELECT owner_lease_expires_at FROM play_sessions WHERE id = ?`, sessionID).Scan(&lease).Error)
	return lease.OwnerLeaseExpiresAt
}
