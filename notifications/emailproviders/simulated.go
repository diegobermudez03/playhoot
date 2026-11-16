// Package emailproviders holds the outbound email providers behind
// notifications.EmailProvider. Delivery 1 runs simulated local providers
// with the real contract: Resend's joint free quota with clear quota
// refusals, SES always accepting at a per-email price, signed webhooks, and
// failures or uncertain answers injected on demand.
package emailproviders

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/notifications"
)

// SignatureHeader carries a simulated webhook's HMAC-SHA256 of the body.
const SignatureHeader = "X-Simulated-Signature"

// ErrBadSignature means a webhook was not signed with the provider's secret.
var ErrBadSignature = errors.New("webhook signature mismatch")

// Reference prices of one accepted email.
const (
	ResendUSDPerEmail = 0
	SESUSDPerEmail    = 0.0001
)

// Simulated is a local provider. A zero DailyQuota or MonthlyQuota does not
// limit; the quota is counted for the whole simulated account.
type Simulated struct {
	provider     notifications.Provider
	dailyQuota   int
	monthlyQuota int
	usdPerEmail  float64
	secret       []byte
	now          func() time.Time

	mu       sync.Mutex
	day      string
	month    string
	today    int
	monthly  int
	injected []notifications.ProviderOutcome
	sent     []notifications.OutboundEmail
	seq      int
}

// NewResend simulates Resend's free plan: 100 a day and 3,000 a month,
// overage off.
func NewResend(webhookSecret []byte) *Simulated {
	return &Simulated{
		provider: notifications.ProviderResend, dailyQuota: notifications.ResendDailyQuota,
		monthlyQuota: notifications.ResendMonthlyQuota, usdPerEmail: ResendUSDPerEmail, secret: webhookSecret, now: time.Now,
	}
}

// NewSES simulates SES pay-as-you-go: always accepts, priced per email.
func NewSES(webhookSecret []byte) *Simulated {
	return &Simulated{provider: notifications.ProviderSES, usdPerEmail: SESUSDPerEmail, secret: webhookSecret, now: time.Now}
}

// WithClock replaces the clock the quota windows follow.
func (s *Simulated) WithClock(now func() time.Time) *Simulated {
	s.now = now
	return s
}

// Inject makes the next sends answer outcomes, in order, before the normal
// behavior resumes. An injected accept still counts against the quota.
func (s *Simulated) Inject(outcomes ...notifications.ProviderOutcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injected = append(s.injected, outcomes...)
}

// Sent returns the messages this provider accepted.
func (s *Simulated) Sent() []notifications.OutboundEmail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]notifications.OutboundEmail(nil), s.sent...)
}

// Provider names the provider.
func (s *Simulated) Provider() notifications.Provider {
	return s.provider
}

// USDPerEmail is the reference price of one accepted email.
func (s *Simulated) USDPerEmail() float64 {
	return s.usdPerEmail
}

// Send answers like the provider would.
func (s *Simulated) Send(_ context.Context, email notifications.OutboundEmail) notifications.ProviderAnswer {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	if day := now.Format(time.DateOnly); day != s.day {
		s.day, s.today = day, 0
	}
	if month := now.Format("2006-01"); month != s.month {
		s.month, s.monthly = month, 0
	}
	outcome := notifications.ProviderAccepted
	if len(s.injected) > 0 {
		outcome, s.injected = s.injected[0], s.injected[1:]
	}
	if outcome == notifications.ProviderAccepted && !strings.Contains(email.To, "@") {
		outcome = notifications.ProviderRefused
	}
	if outcome == notifications.ProviderAccepted &&
		((s.dailyQuota > 0 && s.today >= s.dailyQuota) || (s.monthlyQuota > 0 && s.monthly >= s.monthlyQuota)) {
		outcome = notifications.ProviderQuotaRejected
	}
	if outcome != notifications.ProviderAccepted {
		return notifications.ProviderAnswer{Outcome: outcome}
	}
	s.today++
	s.monthly++
	s.seq++
	s.sent = append(s.sent, email)
	return notifications.ProviderAnswer{Outcome: outcome, MessageID: fmt.Sprintf("%s-sim-%d", s.provider, s.seq)}
}

type webhookBody struct {
	Events []struct {
		Type  string `json:"type"`
		Email string `json:"email"`
	} `json:"events"`
}

// VerifyWebhook checks the body's signature and reads its bounces and
// complaints; other event types are ignored.
func (s *Simulated) VerifyWebhook(header http.Header, body []byte) ([]notifications.Feedback, error) {
	got, err := hex.DecodeString(header.Get(SignatureHeader))
	if err != nil || !hmac.Equal(got, Sign(s.secret, body)) {
		return nil, ErrBadSignature
	}
	var parsed webhookBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("reading webhook: %w", err)
	}
	var feedback []notifications.Feedback
	for _, event := range parsed.Events {
		switch notifications.FeedbackReason(event.Type) {
		case notifications.ReasonBounce, notifications.ReasonComplaint:
			feedback = append(feedback, notifications.Feedback{Email: event.Email, Reason: notifications.FeedbackReason(event.Type)})
		}
	}
	return feedback, nil
}

// Sign is the simulated webhook signature of body.
func Sign(secret, body []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return mac.Sum(nil)
}
