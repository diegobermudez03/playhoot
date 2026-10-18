package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestAbortStartIntegration(t *testing.T) {
	type test struct {
		session startableSession

		expectedStatus      integrationStatus
		expectedActiveCodes int
		expectedHistory     []string
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	cause := "GAME_START_FAILURE"

	tests := map[string]func(t *testing.T) test{
		"lobby_is_aborted_and_its_code_released": func(t *testing.T) test {
			return test{
				session:        seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				expectedStatus:  integrationStatus{Status: "ABORTED", EndCause: &cause},
				expectedHistory: []string{"LOBBY -> ABORTED GAME_START_FAILURE by platform"},
			}
		},
		"running_session_is_left_as_it_is": func(t *testing.T) test {
			return test{
				session:             seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				expectedStatus:      integrationStatus{Status: "RUNNING"},
				expectedActiveCodes: 1,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			err := manager.AbortStart(context.Background(), tc.session.UUID)

			require.NoError(t, err)
			status, _, _, active := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedStatus, status)
			require.Len(t, active, tc.expectedActiveCodes)
			require.Equal(t, tc.expectedHistory, statusHistory(t, db, tc.session.ID))
		})
	}
}
