package emailproviders

import (
	"context"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
)

func TestSimulated(t *testing.T) {
	ctx := context.Background()
	email := notifications.OutboundEmail{To: "ana@example.com", Subject: "s", Text: "t"}
	tests := map[string]func(t *testing.T){
		"resend refuses clearly once the daily quota is spent and renews the next day": func(t *testing.T) {
			now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
			resend := NewResend(nil).WithClock(func() time.Time { return now })
			for i := 0; i < notifications.ResendDailyQuota; i++ {
				require.Equal(t, notifications.ProviderAccepted, resend.Send(ctx, email).Outcome)
			}
			require.Equal(t, notifications.ProviderQuotaRejected, resend.Send(ctx, email).Outcome)
			now = now.Add(24 * time.Hour)
			require.Equal(t, notifications.ProviderAccepted, resend.Send(ctx, email).Outcome)
		},
		"resend honors the monthly quota across days": func(t *testing.T) {
			now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			resend := NewResend(nil).WithClock(func() time.Time { return now })
			for day := 0; day < 30; day++ {
				for i := 0; i < notifications.ResendDailyQuota; i++ {
					resend.Send(ctx, email)
				}
				now = now.Add(24 * time.Hour)
			}
			now = time.Date(2026, 11, 30, 23, 0, 0, 0, time.UTC)
			require.Equal(t, notifications.ProviderQuotaRejected, resend.Send(ctx, email).Outcome)
		},
		"ses always accepts at its price": func(t *testing.T) {
			ses := NewSES(nil)
			for i := 0; i < 500; i++ {
				require.Equal(t, notifications.ProviderAccepted, ses.Send(ctx, email).Outcome)
			}
			require.InDelta(t, 0.0001, ses.USDPerEmail(), 1e-12)
		},
		"injected answers come first": func(t *testing.T) {
			ses := NewSES(nil)
			ses.Inject(notifications.ProviderUncertain, notifications.ProviderFailed)
			require.Equal(t, notifications.ProviderUncertain, ses.Send(ctx, email).Outcome)
			require.Equal(t, notifications.ProviderFailed, ses.Send(ctx, email).Outcome)
			require.Equal(t, notifications.ProviderAccepted, ses.Send(ctx, email).Outcome)
			require.Len(t, ses.Sent(), 1)
		},
		"webhooks need the signature and yield bounces and complaints": func(t *testing.T) {
			secret := []byte("secret")
			ses := NewSES(secret)
			body := []byte(`{"events":[{"type":"bounce","email":"a@x.com"},{"type":"delivered","email":"b@x.com"},{"type":"complaint","email":"c@x.com"}]}`)
			_, err := ses.VerifyWebhook(http.Header{}, body)
			require.ErrorIs(t, err, ErrBadSignature)
			header := http.Header{}
			header.Set(SignatureHeader, hex.EncodeToString(Sign(secret, body)))
			feedback, err := ses.VerifyWebhook(header, body)
			require.NoError(t, err)
			require.Equal(t, []notifications.Feedback{{Email: "a@x.com", Reason: notifications.ReasonBounce}, {Email: "c@x.com", Reason: notifications.ReasonComplaint}}, feedback)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
