package sweepresourcegrants

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestSweepThroughTheQueue(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	userUUID := uuid.NewString()
	require.NoError(t, db.Exec(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?)`, userUUID, "Ana").Error)
	var userID int64
	require.NoError(t, db.Raw(`SELECT id FROM identity_users WHERE user_uuid = ?`, userUUID).Scan(&userID).Error)
	now := time.Now().UTC()
	grant := func(expiresAt time.Time) string {
		grantUUID := uuid.NewString()
		require.NoError(t, db.Exec(`INSERT INTO identity_resource_grants (uuid, user_id, resource_kind, resource_id, scope, token_sha256, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			grantUUID, userID, "game", "g-1", "s", uuid.NewString(), expiresAt.Add(-15*time.Minute), expiresAt).Error)
		return grantUUID
	}
	old := grant(now.Add(-48 * time.Hour))
	recent := grant(now.Add(-time.Hour))
	live := grant(now.Add(time.Hour))

	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(identity.QueueDomain)
	New(db).Register(queue)
	require.NoError(t, queue.SchedulePeriodic(ctx))
	claimed, err := queue.ProcessDue(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, claimed)

	var remaining []string
	require.NoError(t, db.Raw(`SELECT uuid FROM identity_resource_grants WHERE user_id = ? ORDER BY expires_at`, userID).Scan(&remaining).Error)
	require.Equal(t, []string{recent, live}, remaining)
	require.NotContains(t, remaining, old)

	var next []time.Time
	require.NoError(t, db.Raw(`SELECT run_at FROM identity_queue_tasks WHERE type = ? AND status = ?`, TaskType, txqueue.StatusPending).Scan(&next).Error)
	require.Len(t, next, 1, "the sweep scheduled its next run")
	require.WithinDuration(t, now.Add(Every), next[0], time.Minute)
}
