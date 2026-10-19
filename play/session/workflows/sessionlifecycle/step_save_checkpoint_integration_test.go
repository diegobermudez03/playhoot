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

func TestSaveCheckpointIntegration(t *testing.T) {
	type test struct {
		session startableSession
		// stored is the checkpoint the Session already has, if any.
		stored     *sessionpkg.Checkpoint
		checkpoint sessionpkg.Checkpoint

		expected            sessionpkg.SaveCheckpointOutcome
		expectedCheckpoints []integrationCheckpoint
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	older := sessionpkg.Checkpoint{ContextRevision: 1, Content: []byte(`{"revision":1}`)}
	newer := sessionpkg.Checkpoint{ContextRevision: 5, Content: []byte(`{"revision":5}`)}

	tests := map[string]func(t *testing.T) test{
		"first_checkpoint_of_a_running_session": func(t *testing.T) test {
			return test{
				session:             seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				checkpoint:          older,
				expected:            sessionpkg.CheckpointSaved,
				expectedCheckpoints: []integrationCheckpoint{{ContextRevision: 1, Content: older.Content}},
			}
		},
		"newer_checkpoint_replaces_the_stored_one": func(t *testing.T) test {
			return test{
				session:             seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				stored:              &older,
				checkpoint:          newer,
				expected:            sessionpkg.CheckpointSaved,
				expectedCheckpoints: []integrationCheckpoint{{ContextRevision: 5, Content: newer.Content}},
			}
		},
		"older_checkpoint_never_replaces_a_newer_one": func(t *testing.T) test {
			return test{
				session:             seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				stored:              &newer,
				checkpoint:          older,
				expected:            sessionpkg.CheckpointSaved,
				expectedCheckpoints: []integrationCheckpoint{{ContextRevision: 5, Content: newer.Content}},
			}
		},
		"ended_session_stores_nothing": func(t *testing.T) test {
			return test{
				session:    seedStartableSession(t, db, store, "CANCELLED", now.Add(time.Hour)),
				checkpoint: older,
				expected:   sessionpkg.CheckpointSessionNotRunning,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)
			if tc.stored != nil {
				_, err := manager.SaveCheckpoint(context.Background(), tc.session.UUID, *tc.stored)
				require.NoError(t, err)
			}

			outcome, err := manager.SaveCheckpoint(context.Background(), tc.session.UUID, tc.checkpoint)

			require.NoError(t, err)
			require.Equal(t, tc.expected, outcome)
			_, _, checkpoints, _ := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedCheckpoints, checkpoints)
		})
	}
}
