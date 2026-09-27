package repo

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

func TestRepoLockSessionByIDAndByUUID(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))

	byID, err := r.LockSessionByID(context.Background(), db, fx.SessionID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	require.Equal(t, fx.SessionUUID, byID.UUID)
	require.Equal(t, fx.GameDefinitionUUID, byID.GameDefinitionUUID)

	byUUID, err := r.LockSessionByUUID(context.Background(), db, fx.SessionUUID)
	require.NoError(t, err)
	require.NotNil(t, byUUID)
	require.Equal(t, fx.SessionID, byUUID.ID)

	missing, err := r.LockSessionByID(context.Background(), db, 0)
	require.NoError(t, err)
	require.Nil(t, missing)
}
