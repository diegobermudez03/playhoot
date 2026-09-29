package repo

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/internal/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRepoResolveVisibility(t *testing.T) {
	db := testdb.OpenGameDB(t)
	r := New(db)

	gameUUID := uuid.NewString()
	now := time.Now().UTC()
	require.NoError(t, db.Exec(`
		INSERT INTO games (uuid, name, description, owner_uuid, logo_image_url, visibility, created_at, updated_at)
		VALUES (?, 'Test Game', 'A game', ?, '', 'public', ?, ?)
	`, gameUUID, uuid.NewString(), now, now).Error)

	visibility, err := r.ResolveVisibility(context.Background(), gameUUID)
	require.NoError(t, err)
	require.NotNil(t, visibility)
	require.Equal(t, "public", *visibility)
}

func TestRepoResolveVisibility_NotFound(t *testing.T) {
	db := testdb.OpenGameDB(t)
	r := New(db)

	visibility, err := r.ResolveVisibility(context.Background(), uuid.NewString())
	require.NoError(t, err)
	require.Nil(t, visibility)
}

func TestRepoResolveVisibility_SoftDeletedIsNotFound(t *testing.T) {
	db := testdb.OpenGameDB(t)
	r := New(db)

	gameUUID := uuid.NewString()
	now := time.Now().UTC()
	require.NoError(t, db.Exec(`
		INSERT INTO games (uuid, name, description, owner_uuid, logo_image_url, visibility, created_at, updated_at, deleted_at)
		VALUES (?, 'Test Game', 'A game', ?, '', 'public', ?, ?, ?)
	`, gameUUID, uuid.NewString(), now, now, now).Error)

	visibility, err := r.ResolveVisibility(context.Background(), gameUUID)
	require.NoError(t, err)
	require.Nil(t, visibility)
}
