package repo

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRepoParticipantLifecycle(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	userUUID := uuid.NewString()
	actorID, err := r.CreateActor(context.Background(), db, fx.SessionID, userUUID)
	require.NoError(t, err)

	none, err := r.FindParticipant(context.Background(), db, actorID)
	require.NoError(t, err)
	require.Nil(t, none)

	count, err := r.CountActiveParticipants(context.Background(), db, fx.SessionID)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	require.NoError(t, r.CreateParticipant(context.Background(), db, actorID, "Alice", time.Now().UTC()))

	participant, err := r.FindParticipant(context.Background(), db, actorID)
	require.NoError(t, err)
	require.NotNil(t, participant)
	require.True(t, participant.Active)
	require.Equal(t, "Alice", participant.DisplayName)

	count, err = r.CountActiveParticipants(context.Background(), db, fx.SessionID)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	require.NoError(t, r.DeactivateParticipant(context.Background(), db, participant.ID, time.Now().UTC()))
	participant, err = r.FindParticipant(context.Background(), db, actorID)
	require.NoError(t, err)
	require.False(t, participant.Active)

	count, err = r.CountActiveParticipants(context.Background(), db, fx.SessionID)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	require.NoError(t, r.ActivateParticipant(context.Background(), db, participant.ID, "Alice Renamed", time.Now().UTC()))
	participant, err = r.FindParticipant(context.Background(), db, actorID)
	require.NoError(t, err)
	require.True(t, participant.Active)
	require.Equal(t, "Alice Renamed", participant.DisplayName)
}
