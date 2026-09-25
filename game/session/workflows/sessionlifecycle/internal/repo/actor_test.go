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

func TestRepoFindActorsByIDs(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))

	firstUUID, secondUUID := uuid.NewString(), uuid.NewString()
	firstID, err := r.CreateActor(context.Background(), db, fx.SessionID, firstUUID)
	require.NoError(t, err)
	secondID, err := r.CreateActor(context.Background(), db, fx.SessionID, secondUUID)
	require.NoError(t, err)

	otherFx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	otherUUID := uuid.NewString()
	otherID, err := r.CreateActor(context.Background(), db, otherFx.SessionID, otherUUID)
	require.NoError(t, err)

	t.Run("resolves every requested id in one call", func(t *testing.T) {
		found, err := r.FindActorsByIDs(context.Background(), db, fx.SessionID, []uint{firstID, secondID})
		require.NoError(t, err)
		byID := make(map[uint]Actor, len(found))
		for _, a := range found {
			byID[a.ID] = a
		}
		require.Len(t, byID, 2)
		require.Equal(t, firstUUID, byID[firstID].UserUUID)
		require.Equal(t, secondUUID, byID[secondID].UserUUID)
	})

	t.Run("never returns an id belonging to a different session", func(t *testing.T) {
		found, err := r.FindActorsByIDs(context.Background(), db, fx.SessionID, []uint{firstID, otherID})
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, firstUUID, found[0].UserUUID)
	})

	t.Run("returns nil for no ids", func(t *testing.T) {
		found, err := r.FindActorsByIDs(context.Background(), db, fx.SessionID, nil)
		require.NoError(t, err)
		require.Nil(t, found)
	})
}
