package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestCancelAbandonedSessionIntegration(t *testing.T) {
	type test struct {
		session startableSession

		expectedStatus  integrationStatus
		expectedHistory []string
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	allLeft := "ALL_PLAYERS_LEFT"

	tests := map[string]func(t *testing.T) test{
		"running_session_is_cancelled_with_its_cause": func(t *testing.T) test {
			return test{
				session:         seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				expectedStatus:  integrationStatus{Status: "CANCELLED", EndCause: &allLeft},
				expectedHistory: []string{"RUNNING -> CANCELLED ALL_PLAYERS_LEFT by platform"},
			}
		},
		"lobby_is_left_as_it_is": func(t *testing.T) test {
			return test{
				session:        seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				expectedStatus: integrationStatus{Status: "LOBBY"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			err := manager.CancelAbandonedSession(context.Background(), tc.session.UUID)

			require.NoError(t, err)
			status, _, _, _ := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedStatus, status)
			require.Equal(t, tc.expectedHistory, statusHistory(t, db, tc.session.ID))
		})
	}
}
