package emailadmin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/emailproviders"
	"github.com/diegobermudez03/playhoot/notifications/internal/testdb"
	"github.com/diegobermudez03/playhoot/notifications/usecases/expiredeliveries"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestEmailAdmin(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := New(db, emailproviders.NewResend(nil), emailproviders.NewSES(nil))
	delivery := func(state notifications.DeliveryState, age time.Duration) {
		require.NoError(t, db.Exec(`INSERT INTO notifications_email_deliveries (entry_id, provider, state, provider_message_id, detail, closes_email, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			1, "ses", state, "", "", true, time.Now().Add(-age)).Error)
	}
	tests := map[string]func(t *testing.T){
		"status shows the mode, usage, estimated cost and recent deliveries": func(t *testing.T) {
			testdb.Reset(t, db)
			day := time.Now().UTC().Truncate(24 * time.Hour)
			require.NoError(t, db.Exec(`INSERT INTO notifications_provider_usage (provider, day, sent) VALUES (?, ?, ?)`, notifications.ProviderSES, day, 1000).Error)
			delivery(notifications.DeliverySent, time.Hour)
			delivery(notifications.DeliveryFailed, time.Hour)
			delivery(notifications.DeliverySent, 31*24*time.Hour)
			status, err := service.AdminEmailStatus(ctx)
			require.NoError(t, err)
			require.Equal(t, notifications.ModeAutomatic, status.Mode)
			require.Equal(t, map[notifications.DeliveryState]int{"pending": 0, "sent": 1, "rejected": 0, "failed": 1}, status.Deliveries)
			require.Equal(t, 1000, status.Usage[1].Today)
			require.InDelta(t, 0.1, status.Usage[1].EstimatedUSD, 1e-9)
		},
		"the mode changes with actor and reason, once": func(t *testing.T) {
			testdb.Reset(t, db)
			admin := uuid.NewString()
			outcome, err := service.AdminSetEmailMode(ctx, admin, notifications.ModeSESOnly, "resend incident")
			require.NoError(t, err)
			require.Equal(t, notifications.ModeChanged, outcome)
			outcome, err = service.AdminSetEmailMode(ctx, admin, notifications.ModeSESOnly, "again")
			require.NoError(t, err)
			require.Equal(t, notifications.ModeUnchanged, outcome)
			outcome, err = service.AdminSetEmailMode(ctx, admin, "push", "x")
			require.NoError(t, err)
			require.Equal(t, notifications.ModeInvalid, outcome)
			status, err := service.AdminEmailStatus(ctx)
			require.NoError(t, err)
			require.Equal(t, notifications.ModeSESOnly, status.Mode)
			require.Equal(t, "resend incident", status.ModeReason)
		},
		"records older than 30 days expire": func(t *testing.T) {
			testdb.Reset(t, db)
			delivery(notifications.DeliverySent, 31*24*time.Hour)
			delivery(notifications.DeliverySent, 29*24*time.Hour)
			deleted, err := expiredeliveries.New(db).Expire(ctx)
			require.NoError(t, err)
			require.EqualValues(t, 1, deleted)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
