package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestDeactivateSessionOf(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// A session whose first token was rotated: old (inactive) -> current (active).
	sessionUUID := uuid.NewString()
	old, current := "phr_"+uuid.NewString(), "phr_"+uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at, deactivated_at)
		VALUES (?, 1, ?, ?, ?, ?)`, sessionUUID, old, now, now, now).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at)
		VALUES (?, 1, ?, ?, ?)`, sessionUUID, current, now, now).Error)

	ended, err := repo.DeactivateSessionOf(ctx, old, now)
	require.NoError(t, err)
	require.True(t, ended, "any token of the session ends it")

	var active int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM identity_sessions WHERE session_uuid = ? AND deactivated_at IS NULL`, sessionUUID).Scan(&active).Error)
	require.Zero(t, active)

	again, err := repo.DeactivateSessionOf(ctx, current, now)
	require.NoError(t, err)
	require.False(t, again, "an ended session ends nothing more")

	unknown, err := repo.DeactivateSessionOf(ctx, "phr_unknown", now)
	require.NoError(t, err)
	require.False(t, unknown)
}

func TestDeactivateSessionOfWhileItRotates(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	sessionUUID := uuid.NewString()
	current, rotated := "phr_"+uuid.NewString(), "phr_"+uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at)
		VALUES (?, 1, ?, ?, ?)`, sessionUUID, current, now, now).Error)

	// A rotation holds the current row while signing out waits on it.
	rotation := db.Begin()
	require.NoError(t, rotation.Exec(`UPDATE identity_sessions SET deactivated_at = ? WHERE refresh_token = ?`, now, current).Error)
	require.NoError(t, rotation.Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at)
		VALUES (?, 1, ?, ?, ?)`, sessionUUID, rotated, now, now).Error)

	done := make(chan error, 1)
	go func() {
		_, err := repo.DeactivateSessionOf(ctx, current, now)
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, rotation.Commit().Error)
	require.NoError(t, <-done)

	var active int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM identity_sessions WHERE session_uuid = ? AND deactivated_at IS NULL`, sessionUUID).Scan(&active).Error)
	require.Zero(t, active, "the row the rotation created is deactivated too")
}
