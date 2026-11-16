package acceptentry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/internal/testdb"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestAcceptEntry(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(notifications.QueueDomain)
	service := New(db, queue)
	sendTasks := func(entryUUID string) int {
		var count int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM notifications_queue_tasks WHERE type = ? AND idempotency_key = ?`, notifications.SendTaskType, entryUUID).Scan(&count).Error)
		return count
	}
	notice := func(typ notifications.Type) notifications.Notice {
		return notifications.Notice{
			SourceDomain: "budget", Recipient: uuid.NewString(), Type: typ, IdempotencyKey: uuid.NewString(),
			ExternalToken: "tok", Skeleton: map[string]any{"level": "paused"}, HasEmail: true,
		}
	}
	tests := map[string]func(t *testing.T){
		"the same source and key are accepted once": func(t *testing.T) {
			n := notice(notifications.TypeAccountSecurity)
			first, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			require.Equal(t, notifications.Accepted, first.Outcome)
			require.True(t, first.Email)
			again, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			require.Equal(t, notifications.AcceptedBefore, again.Outcome)
			require.Equal(t, first.EntryUUID, again.EntryUUID)
			var entries int
			require.NoError(t, db.Raw(`SELECT COUNT(*) FROM notifications_entries WHERE idempotency_key = ?`, n.IdempotencyKey).Scan(&entries).Error)
			require.Equal(t, 1, entries)
			require.Equal(t, 1, sendTasks(first.EntryUUID))
		},
		"the same key from another source is another entry": func(t *testing.T) {
			n := notice(notifications.TypeWelcome)
			first, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			n.SourceDomain = "identity"
			second, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			require.Equal(t, notifications.Accepted, second.Outcome)
			require.NotEqual(t, first.EntryUUID, second.EntryUUID)
		},
		"saved preferences trim optional email only": func(t *testing.T) {
			n := notice(notifications.TypeAIWorkFinished)
			require.NoError(t, db.Exec(`INSERT INTO notifications_preferences (user_uuid, ai_email, capacity_email, review_email, marketing_email, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
				n.Recipient, false, false, false, false, time.Now()).Error)
			off, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			require.False(t, off.Email)
			require.Equal(t, 0, sendTasks(off.EntryUUID))
			necessary := notice(notifications.TypePaymentFailed)
			necessary.Recipient = n.Recipient
			on, err := service.AcceptEntry(ctx, necessary)
			require.NoError(t, err)
			require.True(t, on.Email)
		},
		"a future send time schedules the email task then": func(t *testing.T) {
			n := notice(notifications.TypeTrialUpdate)
			n.SendAt = time.Now().Add(24 * time.Hour)
			result, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			var runAt time.Time
			require.NoError(t, db.Raw(`SELECT run_at FROM notifications_queue_tasks WHERE idempotency_key = ?`, result.EntryUUID).Scan(&runAt).Error)
			require.WithinDuration(t, n.SendAt, runAt, time.Second)
		},
		"an unknown type or recipient is invalid": func(t *testing.T) {
			n := notice("push-alert")
			result, err := service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			require.Equal(t, notifications.AcceptInvalid, result.Outcome)
			n = notice(notifications.TypeWelcome)
			n.Recipient = "admin"
			result, err = service.AcceptEntry(ctx, n)
			require.NoError(t, err)
			require.Equal(t, notifications.AcceptInvalid, result.Outcome)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
