package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestLoadRunningSessionIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	manager := New(db, store)
	ctx := context.Background()

	t.Run("committed_start_is_read_back", func(t *testing.T) {
		session := seedStartableSession(t, db, store, "LOBBY", time.Now().Add(time.Hour))
		commit := sessionpkg.StartCommit{
			Participants: []sessionpkg.StartParticipant{{UserUUID: uuid.NewString(), ParticipantID: "P1", DisplayName: "Ana"}, {UserUUID: uuid.NewString(), ParticipantID: "P2", DisplayName: "Bruno"}},
			Checkpoint:   sessionpkg.Checkpoint{ContextRevision: 1, Content: []byte(`{"schema_version":1}`)},
		}
		outcome, err := manager.CommitStart(ctx, session.UUID, session.CreatorUUID, commit)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.CommitStartCommitted, outcome)

		running, err := manager.LoadRunningSession(ctx, session.UUID)

		require.NoError(t, err)
		require.Equal(t, sessionpkg.RunningSession{
			GameVersionUUID: session.VersionUUID,
			Script:          []byte(startableScript),
			Participants:    commit.Participants,
			Checkpoint:      commit.Checkpoint,
		}, running)
	})
	t.Run("lobby_is_not_running", func(t *testing.T) {
		session := seedStartableSession(t, db, store, "LOBBY", time.Now().Add(time.Hour))

		_, err := manager.LoadRunningSession(ctx, session.UUID)

		require.ErrorIs(t, err, sessionpkg.ErrSessionNotRunning)
	})
}
