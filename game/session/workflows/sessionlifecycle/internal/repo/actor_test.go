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

func TestRepoFindAndCreateActor(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	userUUID := uuid.NewString()

	none, err := r.FindActor(context.Background(), db, fx.SessionID, userUUID)
	require.NoError(t, err)
	require.Nil(t, none)

	actorID, err := r.CreateActor(context.Background(), db, fx.SessionID, userUUID)
	require.NoError(t, err)
	require.NotZero(t, actorID)

	found, err := r.FindActor(context.Background(), db, fx.SessionID, userUUID)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, actorID, found.ID)
	require.Equal(t, userUUID, found.UserUUID)
}
