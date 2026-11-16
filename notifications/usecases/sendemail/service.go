// Package sendemail sends one entry's email: suppressions first, then the
// provider the mode and Resend's joint quota pick, recording every attempt
// without its content. Once a provider accepted, or answered uncertainly,
// the email is never sent again by either provider.
package sendemail

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/internal/business/render"
	"github.com/diegobermudez03/playhoot/notifications/internal/business/routing"
	"github.com/diegobermudez03/playhoot/notifications/internal/business/unsubscribe"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// emptySkeleton is what an entry not shown in Playhoot keeps once its email
// is decided.
const emptySkeleton = "{}"

var (
	outcomeLabel = metrics.NewLabel("outcome",
		string(notifications.EmailSent), string(notifications.EmailRejected), string(notifications.EmailFailed),
		string(notifications.EmailRetry), string(notifications.EmailAlreadyDecided), string(notifications.EmailNotApplicable))
	sendsTotal = metrics.NewCounter("notifications_emails_total", "Email task runs by what was decided.", outcomeLabel)

	providerLabel = metrics.NewLabel("provider", string(notifications.ProviderResend), string(notifications.ProviderSES))
	answerLabel   = metrics.NewLabel("answer",
		string(notifications.ProviderAccepted), string(notifications.ProviderQuotaRejected), string(notifications.ProviderRefused),
		string(notifications.ProviderFailed), string(notifications.ProviderUncertain))
	answersTotal = metrics.NewCounter("notifications_provider_answers_total", "Provider answers to sends, by provider.", providerLabel, answerLabel)
)

// Enqueuer leaves a task in Notifications' queue inside a transaction.
type Enqueuer interface {
	Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error
}

// Links are the addresses emails point to: the notifications page of the
// web app, and the API's unsubscribe route prefix (the token is appended).
type Links struct {
	OpenURL        string
	UnsubscribeURL string
}

// Service sends entries' email.
type Service struct {
	repo      *internalrepo.Repo
	queue     Enqueuer
	providers map[notifications.Provider]notifications.EmailProvider
	sealer    *unsubscribe.Sealer
	links     Links
	now       func() time.Time
}

// New constructs a Service on db with both providers; unsubscribeKey (32
// bytes) seals addresses into unsubscribe links.
func New(db *gorm.DB, queue Enqueuer, unsubscribeKey []byte, links Links, providers ...notifications.EmailProvider) (*Service, error) {
	sealer, err := unsubscribe.New(unsubscribeKey)
	if err != nil {
		return nil, err
	}
	byName := map[notifications.Provider]notifications.EmailProvider{}
	for _, p := range providers {
		byName[p.Provider()] = p
	}
	return &Service{repo: internalrepo.New(db), queue: queue, providers: byName, sealer: sealer, links: links, now: time.Now}, nil
}

// SendEmail decides entryUUID's email for to.
func (s *Service) SendEmail(ctx context.Context, entryUUID string, to notifications.Recipient) (notifications.SendOutcome, error) {
	defer logging.Step(ctx, "notifications.send_email").Close()
	outcome, err := s.send(ctx, entryUUID, to)
	if err != nil {
		return "", err
	}
	sendsTotal.Inc(ctx, outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("notification_email", string(outcome)))
	return outcome, nil
}

// start is what the locked check before any provider call found.
type start struct {
	entry    *internalrepo.Entry
	outcome  notifications.SendOutcome
	provider notifications.Provider
	mode     notifications.Mode
}

func (s *Service) send(ctx context.Context, entryUUID string, to notifications.Recipient) (notifications.SendOutcome, error) {
	entry, err := s.repo.EntryByUUID(ctx, s.repo.GetDB(), entryUUID)
	if err != nil {
		return "", err
	}
	if entry == nil || !entry.EmailChannel {
		return notifications.EmailNotApplicable, nil
	}
	spec, known := notifications.Spec(notifications.Type(entry.Type))
	if !known {
		return "", txqueue.Permanent(fmt.Errorf("entry %s has unknown type %q", entryUUID, entry.Type))
	}
	email := notifications.NormalizeEmail(to.Email)
	st, err := s.begin(ctx, entry, spec, email)
	if err != nil || st.outcome != "" {
		return st.outcome, err
	}
	provider := st.provider
	for {
		answer, deliveryID, err := s.attempt(ctx, entry, spec, provider, email, to.Language)
		if err != nil {
			return "", err
		}
		outcome, err := s.decide(ctx, entry, provider, deliveryID, answer)
		if err != nil {
			return "", err
		}
		if answer.Outcome == notifications.ProviderQuotaRejected {
			if routing.FallbackOnQuota(st.mode, provider) {
				provider = notifications.ProviderSES
				continue
			}
			if provider == notifications.ProviderResend {
				return notifications.EmailRetry, nil
			}
		}
		return outcome, nil
	}
}

// begin locks the entry and decides, before any provider call, whether the
// email is already over, goes nowhere, or which provider to try.
func (s *Service) begin(ctx context.Context, entry *internalrepo.Entry, spec notifications.TypeSpec, email string) (start, error) {
	return utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (start, error) {
		if err := s.repo.LockEntry(ctx, tx, entry.ID); err != nil {
			return start{}, err
		}
		deliveries, err := s.repo.DeliveriesForEntry(ctx, tx, entry.ID)
		if err != nil {
			return start{}, err
		}
		now := s.now().UTC()
		for _, d := range deliveries {
			if d.ClosesEmail || d.State == string(notifications.DeliverySent) {
				return start{outcome: notifications.EmailAlreadyDecided}, nil
			}
			if d.State == string(notifications.DeliveryPending) {
				// An earlier run stopped while the provider held the message:
				// as with an uncertain answer, it is never resent.
				if _, err := s.repo.DecideDelivery(ctx, tx, d.ID, string(notifications.DeliveryPending), string(notifications.DeliveryFailed), "", notifications.DetailInterrupted, true, now); err != nil {
					return start{}, err
				}
				return start{outcome: notifications.EmailFailed}, s.closeContent(ctx, tx, entry, now)
			}
		}
		if email == "" {
			return start{outcome: notifications.EmailRejected}, s.rejectBefore(ctx, tx, entry, notifications.DetailNoAddress, now)
		}
		reasons, err := s.repo.SuppressionReasons(ctx, tx, email)
		if err != nil {
			return start{}, err
		}
		for _, reason := range reasons {
			if reason != string(notifications.ReasonUnsubscribe) || spec.Email == notifications.EmailOptional {
				return start{outcome: notifications.EmailRejected}, s.rejectBefore(ctx, tx, entry, notifications.DetailSuppressed, now)
			}
		}
		mode := notifications.ModeAutomatic
		if row, err := s.repo.LatestMode(ctx, tx); err != nil {
			return start{}, err
		} else if row != nil && notifications.ValidMode(notifications.Mode(row.Mode)) {
			mode = notifications.Mode(row.Mode)
		}
		today, month, err := s.resendUsage(ctx, tx, now)
		if err != nil {
			return start{}, err
		}
		choice := routing.Choose(mode, today, month)
		if choice.Wait {
			return start{outcome: notifications.EmailRetry}, nil
		}
		return start{provider: choice.Provider, mode: mode}, nil
	})
}

func (s *Service) resendUsage(ctx context.Context, tx *gorm.DB, now time.Time) (int, int, error) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	provider := string(notifications.ProviderResend)
	today, err := s.repo.UsageBetween(ctx, tx, provider, day, day.AddDate(0, 0, 1))
	if err != nil {
		return 0, 0, err
	}
	month, err := s.repo.UsageBetween(ctx, tx, provider, monthStart, monthStart.AddDate(0, 1, 0))
	return today, month, err
}

// rejectBefore records a rejection decided before any provider was called.
func (s *Service) rejectBefore(ctx context.Context, tx *gorm.DB, entry *internalrepo.Entry, detail string, now time.Time) error {
	row := internalrepo.Delivery{
		EntryID: entry.ID, Provider: string(notifications.ProviderNone), State: string(notifications.DeliveryRejected),
		Detail: detail, ClosesEmail: true, DecidedAt: &now, CreatedAt: now,
	}
	if err := s.repo.InsertDelivery(ctx, tx, &row); err != nil {
		return err
	}
	return s.closeContent(ctx, tx, entry, now)
}

// attempt records a pending attempt, then renders and sends outside any
// transaction.
func (s *Service) attempt(ctx context.Context, entry *internalrepo.Entry, spec notifications.TypeSpec, provider notifications.Provider, email string, lang notifications.Language) (notifications.ProviderAnswer, int64, error) {
	p, ok := s.providers[provider]
	if !ok {
		return notifications.ProviderAnswer{}, 0, fmt.Errorf("email provider %s is not configured", provider)
	}
	now := s.now().UTC()
	row := internalrepo.Delivery{EntryID: entry.ID, Provider: string(provider), State: string(notifications.DeliveryPending), CreatedAt: now}
	if err := s.repo.InsertDelivery(ctx, s.repo.GetDB(), &row); err != nil {
		return notifications.ProviderAnswer{}, 0, err
	}
	skeleton := map[string]any{}
	if err := json.Unmarshal([]byte(entry.Skeleton), &skeleton); err != nil {
		return notifications.ProviderAnswer{}, 0, fmt.Errorf("reading skeleton: %w", err)
	}
	links := render.Links{Open: s.links.OpenURL}
	if spec.Email == notifications.EmailOptional {
		token, err := s.sealer.Seal(email)
		if err != nil {
			return notifications.ProviderAnswer{}, 0, err
		}
		links.Unsubscribe = s.links.UnsubscribeURL + token
	}
	msg := render.Render(spec, skeleton, lang, links)
	answer := p.Send(ctx, notifications.OutboundEmail{
		To: email, Subject: msg.Subject, Text: msg.Text, Unsubscribe: msg.Unsubscribe, IdempotencyKey: entry.EntryUUID,
	})
	answersTotal.Inc(ctx, providerLabel.Value(string(provider)), answerLabel.Value(string(answer.Outcome)))
	return answer, row.ID, nil
}

// decide records the provider's answer on the attempt; an accepted email
// also counts against the provider's usage and reports its spend.
func (s *Service) decide(ctx context.Context, entry *internalrepo.Entry, provider notifications.Provider, deliveryID int64, answer notifications.ProviderAnswer) (notifications.SendOutcome, error) {
	var (
		state   notifications.DeliveryState
		detail  string
		closes  bool
		outcome notifications.SendOutcome
	)
	switch answer.Outcome {
	case notifications.ProviderAccepted:
		state, closes, outcome = notifications.DeliverySent, true, notifications.EmailSent
	case notifications.ProviderQuotaRejected:
		state, detail, outcome = notifications.DeliveryRejected, notifications.DetailQuota, notifications.EmailRejected
		closes = provider == notifications.ProviderSES
	case notifications.ProviderRefused:
		state, detail, closes, outcome = notifications.DeliveryRejected, notifications.DetailRefused, true, notifications.EmailRejected
	case notifications.ProviderFailed:
		state, detail, outcome = notifications.DeliveryFailed, notifications.DetailProviderFailure, notifications.EmailRetry
	default:
		state, detail, closes, outcome = notifications.DeliveryFailed, notifications.DetailUncertain, true, notifications.EmailFailed
	}
	_, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		now := s.now().UTC()
		decided, err := s.repo.DecideDelivery(ctx, tx, deliveryID, string(notifications.DeliveryPending), string(state), answer.MessageID, detail, closes, now)
		if err != nil || !decided {
			return struct{}{}, err
		}
		if state == notifications.DeliverySent {
			day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			if err := s.repo.IncrementUsage(ctx, tx, string(provider), day); err != nil {
				return struct{}{}, err
			}
			key := "email-delivery:" + strconv.FormatInt(deliveryID, 10)
			spend := notifications.SpendTask{
				Category: notifications.SpendCategory, Service: notifications.SpendService(provider), Reference: notifications.EmailReference(entry.EntryUUID),
				Quantity: 1, Unit: notifications.SpendUnit, Measured: true, IdempotencyKey: key, OccurredAt: now,
			}
			if err := s.queue.Enqueue(tx, txqueue.TypeSpendEvent, spend, time.Time{}, key); err != nil {
				return struct{}{}, err
			}
		}
		if closes {
			return struct{}{}, s.closeContent(ctx, tx, entry, now)
		}
		return struct{}{}, nil
	})
	return outcome, err
}

// closeContent empties an entry not shown in Playhoot once its email ends,
// so content such as an access code is never kept.
func (s *Service) closeContent(ctx context.Context, tx *gorm.DB, entry *internalrepo.Entry, now time.Time) error {
	if entry.InApp {
		return nil
	}
	return s.repo.ClearContent(ctx, tx, entry.ID, emptySkeleton, now)
}
