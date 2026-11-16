// Package acceptentry turns one event into one logical entry: its channels
// come from the type's row of the matrix and the recipient's preferences,
// and its email task is left in the same transaction.
package acceptentry

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/internal/business/channels"
	"github.com/diegobermudez03/playhoot/notifications/internal/prefs"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

// Limits of a notice's fields.
const (
	maxKeyLen      = 200
	maxSourceLen   = 64
	maxSkeletonLen = 8 << 10
)

var (
	outcomeLabel = metrics.NewLabel("outcome", string(notifications.Accepted), string(notifications.AcceptedBefore), string(notifications.AcceptInvalid))
	emailLabel   = metrics.NewLabel("email", "true", "false")
	acceptsTotal = metrics.NewCounter(
		"notifications_entries_accepted_total",
		"Notices received by what acceptance did and whether email applies.",
		outcomeLabel, emailLabel,
	)
)

// Enqueuer leaves a task in Notifications' queue inside a transaction.
type Enqueuer interface {
	Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error
}

// Service accepts entries.
type Service struct {
	repo  *internalrepo.Repo
	queue Enqueuer
	now   func() time.Time
}

// New constructs a Service on db, enqueueing into queue.
func New(db *gorm.DB, queue Enqueuer) *Service {
	return &Service{repo: internalrepo.New(db), queue: queue, now: time.Now}
}

// AcceptEntry stores notice once per source and key; a repeat returns the
// existing entry. The email task is due at the notice's send time.
func (s *Service) AcceptEntry(ctx context.Context, notice notifications.Notice) (notifications.Acceptance, error) {
	defer logging.Step(ctx, "notifications.accept_entry").Close()
	result, err := s.accept(ctx, notice)
	if err != nil {
		return result, err
	}
	acceptsTotal.Inc(ctx, outcomeLabel.Value(string(result.Outcome)), emailLabel.Value(boolText(result.Email)))
	logging.LogFields(ctx, logging.Field("notification_accept", string(result.Outcome)))
	return result, nil
}

func (s *Service) accept(ctx context.Context, notice notifications.Notice) (notifications.Acceptance, error) {
	invalid := notifications.Acceptance{Outcome: notifications.AcceptInvalid}
	spec, known := notifications.Spec(notice.Type)
	if !known || notice.SourceDomain == "" || len(notice.SourceDomain) > maxSourceLen || notice.IdempotencyKey == "" ||
		len(notice.IdempotencyKey) > maxKeyLen || len(notice.ExternalToken) > maxKeyLen {
		return invalid, nil
	}
	if _, err := uuid.Parse(notice.Recipient); err != nil {
		return invalid, nil
	}
	skeleton := notice.Skeleton
	if skeleton == nil {
		skeleton = map[string]any{}
	}
	content, err := json.Marshal(skeleton)
	if err != nil || len(content) > maxSkeletonLen {
		return invalid, nil
	}
	return utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (notifications.Acceptance, error) {
		existing, err := s.repo.EntryByKey(ctx, tx, notice.SourceDomain, notice.IdempotencyKey)
		if err != nil {
			return notifications.Acceptance{}, err
		}
		if existing != nil {
			return duplicate(existing), nil
		}
		preferences, err := prefs.Current(ctx, s.repo, tx, notice.Recipient)
		if err != nil {
			return notifications.Acceptance{}, err
		}
		decision := channels.Decide(spec, notice.HasEmail, notice.Student, notice.MarketingConsent, preferences)
		now := s.now().UTC()
		sendAt := notice.SendAt.UTC()
		if sendAt.Before(now) {
			sendAt = now
		}
		entry := internalrepo.Entry{
			EntryUUID: uuid.NewString(), UserUUID: notice.Recipient, SourceDomain: notice.SourceDomain, Type: string(notice.Type),
			ExternalToken: notice.ExternalToken, IdempotencyKey: notice.IdempotencyKey, Skeleton: string(content), SendAt: sendAt,
			InApp: decision.InApp, EmailChannel: decision.Email, Necessary: spec.Necessary(), CreatedAt: now,
		}
		inserted, err := s.repo.InsertEntry(ctx, tx, &entry)
		if err != nil {
			return notifications.Acceptance{}, err
		}
		if !inserted {
			// A concurrent acceptance of the same notice won the insert.
			existing, err := s.repo.EntryByKey(ctx, tx, notice.SourceDomain, notice.IdempotencyKey)
			if err != nil || existing == nil {
				return notifications.Acceptance{}, err
			}
			return duplicate(existing), nil
		}
		if decision.Email {
			task := notifications.SendTask{EntryUUID: entry.EntryUUID, Recipient: entry.UserUUID, Necessary: entry.Necessary}
			if err := s.queue.Enqueue(tx, notifications.SendTaskType, task, sendAt, entry.EntryUUID); err != nil {
				return notifications.Acceptance{}, err
			}
		}
		return notifications.Acceptance{Outcome: notifications.Accepted, EntryUUID: entry.EntryUUID, Email: decision.Email}, nil
	})
}

func duplicate(existing *internalrepo.Entry) notifications.Acceptance {
	return notifications.Acceptance{Outcome: notifications.AcceptedBefore, EntryUUID: existing.EntryUUID, Email: existing.EmailChannel}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
