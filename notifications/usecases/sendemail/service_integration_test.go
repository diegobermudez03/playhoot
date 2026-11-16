package sendemail

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/emailproviders"
	"github.com/diegobermudez03/playhoot/notifications/internal/testdb"
	"github.com/diegobermudez03/playhoot/notifications/usecases/acceptentry"
	"github.com/diegobermudez03/playhoot/notifications/usecases/recordsuppression"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

type fixture struct {
	db           *gorm.DB
	resend       *emailproviders.Simulated
	ses          *emailproviders.Simulated
	service      *Service
	accept       *acceptentry.Service
	suppressions *recordsuppression.Service
}

var key = bytes.Repeat([]byte{1}, 32)

func newFixture(t *testing.T) fixture {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(notifications.QueueDomain)
	resend, ses := emailproviders.NewResend(nil), emailproviders.NewSES(nil)
	service, err := New(db, queue, key, Links{OpenURL: "http://web/notifications", UnsubscribeURL: "http://api/v1/email/unsubscribe/"}, resend, ses)
	require.NoError(t, err)
	suppressions, err := recordsuppression.New(db, key)
	require.NoError(t, err)
	return fixture{db: db, resend: resend, ses: ses, suppressions: suppressions, accept: acceptentry.New(db, queue), service: service}
}

func (f fixture) entry(t *testing.T, typ notifications.Type) string {
	t.Helper()
	result, err := f.accept.AcceptEntry(context.Background(), notifications.Notice{
		SourceDomain: "test", Recipient: uuid.NewString(), Type: typ, IdempotencyKey: uuid.NewString(),
		Skeleton: map[string]any{"code": "123456", "grade": "9.5"}, HasEmail: true, MarketingConsent: true,
	})
	require.NoError(t, err)
	require.True(t, result.Email)
	return result.EntryUUID
}

type deliveryRow struct {
	Provider string
	State    string
	Detail   string
}

func (f fixture) deliveries(t *testing.T, entryUUID string) []deliveryRow {
	t.Helper()
	var rows []deliveryRow
	require.NoError(t, f.db.Raw(`SELECT d.provider, d.state, d.detail FROM notifications_email_deliveries d
		JOIN notifications_entries e ON e.id = d.entry_id WHERE e.entry_uuid = ? ORDER BY d.id`, entryUUID).Scan(&rows).Error)
	return rows
}

func (f fixture) setResendUsage(t *testing.T, sent int) {
	t.Helper()
	day := time.Now().UTC().Truncate(24 * time.Hour)
	require.NoError(t, f.db.Exec(`INSERT INTO notifications_provider_usage (provider, day, sent) VALUES (?, ?, ?)`, notifications.ProviderResend, day, sent).Error)
}

func (f fixture) setMode(t *testing.T, mode notifications.Mode) {
	t.Helper()
	require.NoError(t, f.db.Exec(`INSERT INTO notifications_provider_settings (mode, changed_by_user_uuid, reason, created_at) VALUES (?, ?, ?, ?)`, mode, uuid.NewString(), "test", time.Now()).Error)
}

var to = notifications.Recipient{Email: "Ana@Example.com", Language: notifications.LanguageEnglish}

func TestSendEmail(t *testing.T) {
	ctx := context.Background()
	tests := map[string]func(t *testing.T){
		"automatic sends by Resend and reports its spend": func(t *testing.T) {
			f := newFixture(t)
			entry := f.entry(t, notifications.TypeAccountSecurity)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailSent, outcome)
			require.Len(t, f.resend.Sent(), 1)
			require.Equal(t, "ana@example.com", f.resend.Sent()[0].To)
			require.Empty(t, f.ses.Sent())
			require.Equal(t, []deliveryRow{{Provider: "resend", State: "sent"}}, f.deliveries(t, entry))
			var spend int
			require.NoError(t, f.db.Raw(`SELECT COUNT(*) FROM notifications_queue_tasks WHERE type = ?`, txqueue.TypeSpendEvent).Scan(&spend).Error)
			require.Equal(t, 1, spend)
		},
		"an accepted email is never sent again by either provider": func(t *testing.T) {
			f := newFixture(t)
			entry := f.entry(t, notifications.TypeAccountSecurity)
			_, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			f.setMode(t, notifications.ModeSESOnly)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailAlreadyDecided, outcome)
			require.Len(t, f.resend.Sent(), 1)
			require.Empty(t, f.ses.Sent())
		},
		"our counted quota sends straight to SES": func(t *testing.T) {
			f := newFixture(t)
			f.setResendUsage(t, notifications.ResendDailyQuota)
			entry := f.entry(t, notifications.TypeWelcome)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailSent, outcome)
			require.Empty(t, f.resend.Sent())
			require.Len(t, f.ses.Sent(), 1)
		},
		"a clear quota refusal by Resend goes to SES": func(t *testing.T) {
			f := newFixture(t)
			f.resend.Inject(notifications.ProviderQuotaRejected)
			entry := f.entry(t, notifications.TypeWelcome)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailSent, outcome)
			require.Equal(t, []deliveryRow{{Provider: "resend", State: "rejected", Detail: notifications.DetailQuota}, {Provider: "ses", State: "sent"}}, f.deliveries(t, entry))
		},
		"other failures never switch provider": func(t *testing.T) {
			f := newFixture(t)
			f.resend.Inject(notifications.ProviderFailed)
			entry := f.entry(t, notifications.TypeWelcome)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailRetry, outcome)
			require.Empty(t, f.ses.Sent())
			outcome, err = f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailSent, outcome, "a clear failure may be retried later")
			require.Len(t, f.resend.Sent(), 1)
		},
		"an uncertain answer is not resent by the other provider nor later": func(t *testing.T) {
			f := newFixture(t)
			f.resend.Inject(notifications.ProviderUncertain)
			entry := f.entry(t, notifications.TypeWelcome)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailFailed, outcome)
			outcome, err = f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailAlreadyDecided, outcome)
			require.Empty(t, f.ses.Sent())
			require.Empty(t, f.resend.Sent())
		},
		"an attempt left pending is treated as uncertain": func(t *testing.T) {
			f := newFixture(t)
			entry := f.entry(t, notifications.TypeWelcome)
			require.NoError(t, f.db.Exec(`INSERT INTO notifications_email_deliveries (entry_id, provider, state, provider_message_id, detail, closes_email, created_at)
				SELECT id, ?, ?, ?, ?, ?, ? FROM notifications_entries WHERE entry_uuid = ?`, "resend", "pending", "", "", false, time.Now(), entry).Error)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailFailed, outcome)
			require.Empty(t, f.resend.Sent())
		},
		"resend only waits beyond its quota": func(t *testing.T) {
			f := newFixture(t)
			f.setMode(t, notifications.ModeResendOnly)
			f.setResendUsage(t, notifications.ResendDailyQuota)
			entry := f.entry(t, notifications.TypeWelcome)
			outcome, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailRetry, outcome)
			require.Empty(t, f.deliveries(t, entry))
			f.resend.Inject(notifications.ProviderQuotaRejected)
			f.db.Exec(`DELETE FROM notifications_provider_usage`)
			outcome, err = f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailRetry, outcome)
			require.Empty(t, f.ses.Sent())
		},
		"a bounce is honored on both providers": func(t *testing.T) {
			f := newFixture(t)
			_, err := f.suppressions.RecordSuppression(ctx, to.Email, notifications.ReasonBounce, notifications.OriginSES)
			require.NoError(t, err)
			for _, mode := range []notifications.Mode{notifications.ModeAutomatic, notifications.ModeSESOnly, notifications.ModeResendOnly} {
				f.setMode(t, mode)
				entry := f.entry(t, notifications.TypeAccountSecurity)
				outcome, err := f.service.SendEmail(ctx, entry, to)
				require.NoError(t, err)
				require.Equal(t, notifications.EmailRejected, outcome, mode)
				require.Equal(t, []deliveryRow{{Provider: "none", State: "rejected", Detail: notifications.DetailSuppressed}}, f.deliveries(t, entry))
			}
			require.Empty(t, f.resend.Sent())
			require.Empty(t, f.ses.Sent())
		},
		"the unsubscribe link stops optional email and keeps necessary email": func(t *testing.T) {
			f := newFixture(t)
			optional := f.entry(t, notifications.TypeDiscoveryReview)
			_, err := f.service.SendEmail(ctx, optional, to)
			require.NoError(t, err)
			link := f.resend.Sent()[0].Unsubscribe
			require.True(t, strings.HasPrefix(link, "http://api/v1/email/unsubscribe/"))
			outcome, err := f.suppressions.Unsubscribe(ctx, strings.TrimPrefix(link, "http://api/v1/email/unsubscribe/"))
			require.NoError(t, err)
			require.Equal(t, notifications.SuppressionRecorded, outcome)
			again, err := f.service.SendEmail(ctx, f.entry(t, notifications.TypeDiscoveryReview), to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailRejected, again)
			necessary, err := f.service.SendEmail(ctx, f.entry(t, notifications.TypeAccountSecurity), to)
			require.NoError(t, err)
			require.Equal(t, notifications.EmailSent, necessary)
		},
		"sending never marks the entry read": func(t *testing.T) {
			f := newFixture(t)
			entry := f.entry(t, notifications.TypeAccountSecurity)
			_, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			var unread int
			require.NoError(t, f.db.Raw(`SELECT COUNT(*) FROM notifications_entries WHERE entry_uuid = ? AND read_at IS NULL`, entry).Scan(&unread).Error)
			require.Equal(t, 1, unread)
		},
		"records carry no content and emails only allowed values": func(t *testing.T) {
			f := newFixture(t)
			entry := f.entry(t, notifications.TypeAccessCode)
			_, err := f.service.SendEmail(ctx, entry, to)
			require.NoError(t, err)
			sent := f.resend.Sent()[0]
			require.Contains(t, sent.Text, "123456")
			require.NotContains(t, sent.Text, "9.5")
			var columns []string
			require.NoError(t, f.db.Raw(`SELECT column_name FROM information_schema.columns WHERE table_name = ?`, "notifications_email_deliveries").Scan(&columns).Error)
			require.ElementsMatch(t, []string{"id", "entry_id", "provider", "state", "provider_message_id", "detail", "closes_email", "decided_at", "created_at"}, columns)
			var skeleton string
			require.NoError(t, f.db.Raw(`SELECT skeleton::text FROM notifications_entries WHERE entry_uuid = ?`, entry).Scan(&skeleton).Error)
			require.Equal(t, "{}", skeleton, "an access code is not kept once its email is decided")
		},
		"a recipient without address is rejected before any provider": func(t *testing.T) {
			f := newFixture(t)
			entry := f.entry(t, notifications.TypeAccountSecurity)
			outcome, err := f.service.SendEmail(ctx, entry, notifications.Recipient{})
			require.NoError(t, err)
			require.Equal(t, notifications.EmailRejected, outcome)
			require.Equal(t, []deliveryRow{{Provider: "none", State: "rejected", Detail: notifications.DetailNoAddress}}, f.deliveries(t, entry))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
