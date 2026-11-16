// Package recordsuppression records an address that must not be emailed:
// an unsubscribe from the own link, or a bounce or complaint a provider
// reported. Both providers honor every suppression.
package recordsuppression

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/internal/business/unsubscribe"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

var (
	reasonLabel = metrics.NewLabel("reason", string(notifications.ReasonUnsubscribe), string(notifications.ReasonBounce), string(notifications.ReasonComplaint))
	resultLabel = metrics.NewLabel("outcome", string(notifications.SuppressionRecorded), string(notifications.SuppressionDuplicate), string(notifications.SuppressionInvalid))
	totals      = metrics.NewCounter("notifications_suppressions_total", "Suppression reports by reason and outcome.", reasonLabel, resultLabel)
)

// Service records suppressions.
type Service struct {
	repo      *internalrepo.Repo
	sealer    *unsubscribe.Sealer
	providers map[notifications.Provider]notifications.EmailProvider
	now       func() time.Time
}

// New constructs a Service on db; unsubscribeKey opens unsubscribe tokens
// and providers verify their webhooks.
func New(db *gorm.DB, unsubscribeKey []byte, providers ...notifications.EmailProvider) (*Service, error) {
	sealer, err := unsubscribe.New(unsubscribeKey)
	if err != nil {
		return nil, err
	}
	byName := map[notifications.Provider]notifications.EmailProvider{}
	for _, p := range providers {
		byName[p.Provider()] = p
	}
	return &Service{repo: internalrepo.New(db), sealer: sealer, providers: byName, now: time.Now}, nil
}

// Unsubscribe records the address sealed in token as unsubscribed from
// optional email. An unreadable token is invalid.
func (s *Service) Unsubscribe(ctx context.Context, token string) (notifications.SuppressionOutcome, error) {
	defer logging.Step(ctx, "notifications.unsubscribe").Close()
	email, err := s.sealer.Open(token)
	if err != nil {
		totals.Inc(ctx, reasonLabel.Value(string(notifications.ReasonUnsubscribe)), resultLabel.Value(string(notifications.SuppressionInvalid)))
		return notifications.SuppressionInvalid, nil
	}
	return s.RecordSuppression(ctx, email, notifications.ReasonUnsubscribe, notifications.OriginPlayhoot)
}

// ProviderWebhook verifies a provider's callback and records each bounce
// and complaint it reports, answering how many were new.
func (s *Service) ProviderWebhook(ctx context.Context, provider notifications.Provider, header map[string][]string, body []byte) (int, error) {
	defer logging.Step(ctx, "notifications.provider_webhook").Close()
	p, ok := s.providers[provider]
	if !ok {
		return 0, fmt.Errorf("%w: provider %s is not configured", notifications.ErrWebhookRejected, provider)
	}
	feedback, err := p.VerifyWebhook(header, body)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", notifications.ErrWebhookRejected, err)
	}
	recorded := 0
	for _, f := range feedback {
		outcome, err := s.RecordSuppression(ctx, f.Email, f.Reason, notifications.SuppressionOrigin(provider))
		if err != nil {
			return recorded, err
		}
		if outcome == notifications.SuppressionRecorded {
			recorded++
		}
	}
	return recorded, nil
}

// RecordSuppression appends email, reason and origin once; a repetition is
// a duplicate.
func (s *Service) RecordSuppression(ctx context.Context, email string, reason notifications.FeedbackReason, origin notifications.SuppressionOrigin) (notifications.SuppressionOutcome, error) {
	outcome, err := s.record(ctx, notifications.NormalizeEmail(email), reason, origin)
	if err != nil {
		return "", err
	}
	totals.Inc(ctx, reasonLabel.Value(string(reason)), resultLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("notification_suppression", string(outcome)))
	return outcome, nil
}

func (s *Service) record(ctx context.Context, email string, reason notifications.FeedbackReason, origin notifications.SuppressionOrigin) (notifications.SuppressionOutcome, error) {
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 320 {
		return notifications.SuppressionInvalid, nil
	}
	switch reason {
	case notifications.ReasonUnsubscribe, notifications.ReasonBounce, notifications.ReasonComplaint:
	default:
		return notifications.SuppressionInvalid, nil
	}
	switch origin {
	case notifications.OriginPlayhoot, notifications.OriginResend, notifications.OriginSES:
	default:
		return notifications.SuppressionInvalid, nil
	}
	return utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (notifications.SuppressionOutcome, error) {
		if err := s.repo.LockSuppressions(ctx, tx, email); err != nil {
			return "", err
		}
		exists, err := s.repo.HasSuppression(ctx, tx, email, string(reason), string(origin))
		if err != nil {
			return "", err
		}
		if exists {
			return notifications.SuppressionDuplicate, nil
		}
		row := internalrepo.Suppression{Email: email, Reason: string(reason), Origin: string(origin), CreatedAt: s.now().UTC()}
		if err := s.repo.InsertSuppression(ctx, tx, &row); err != nil {
			return "", err
		}
		return notifications.SuppressionRecorded, nil
	})
}
