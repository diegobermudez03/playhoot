package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestLockUserAndChangeDisplayName(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	userUUID := uuid.NewString()
	var userID int64
	require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, NULL) RETURNING id`, userUUID).Scan(&userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)

	unnamed, err := repo.LockUser(ctx, db, userUUID)
	require.NoError(t, err)
	require.Equal(t, &User{ID: userID, HasAccount: true}, unnamed)

	require.NoError(t, repo.ChangeDisplayName(ctx, db, userID, nil, "Ana", now))
	first := "Ana"
	require.NoError(t, repo.ChangeDisplayName(ctx, db, userID, &first, "Bea", now.Add(time.Minute)))

	named, err := repo.LockUser(ctx, db, userUUID)
	require.NoError(t, err)
	require.Equal(t, "Bea", *named.DisplayName)
	var history []struct {
		FromName *string
		ToName   string
	}
	require.NoError(t, db.Raw(`SELECT from_name, to_name FROM identity_display_name_changes WHERE user_id = ? ORDER BY id`, userID).Scan(&history).Error)
	require.Len(t, history, 2)
	require.Nil(t, history[0].FromName)
	require.Equal(t, "Ana", history[0].ToName)
	require.Equal(t, "Ana", *history[1].FromName)
	require.Equal(t, "Bea", history[1].ToName)

	missing, err := repo.LockUser(ctx, db, uuid.NewString())
	require.NoError(t, err)
	require.Nil(t, missing)
}
