package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestAbortRunningSessionIntegration(t *testing.T) {
	type test struct {
		session startableSession
		cause   sessionpkg.AbortCause

		expectedStatus  integrationStatus
		expectedHistory []string
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	infra := "UNRECOVERABLE_INFRA_FAILURE"

	tests := map[string]func(t *testing.T) test{
		"running_session_is_aborted_with_its_cause": func(t *testing.T) test {
			return test{
				session:         seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				cause:           sessionpkg.AbortUnrecoverableInfraFailure,
				expectedStatus:  integrationStatus{Status: "ABORTED", EndCause: &infra},
				expectedHistory: []string{"RUNNING -> ABORTED UNRECOVERABLE_INFRA_FAILURE by platform"},
			}
		},
		"lobby_is_left_as_it_is": func(t *testing.T) test {
			return test{
				session:        seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				cause:          sessionpkg.AbortScriptFailure,
				expectedStatus: integrationStatus{Status: "LOBBY"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			err := manager.AbortRunningSession(context.Background(), tc.session.UUID, tc.cause)

			require.NoError(t, err)
			status, _, _, _ := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedStatus, status)
			require.Equal(t, tc.expectedHistory, statusHistory(t, db, tc.session.ID))
		})
	}
}
