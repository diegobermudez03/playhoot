// Package markread marks one of a user's entries read, once: on opening its
// content, on seeing its update, or explicitly. Sending email never does.
package markread

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/notifications"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
)

var (
	outcomeLabel = metrics.NewLabel("outcome", string(notifications.MarkedRead), string(notifications.AlreadyRead), string(notifications.UnknownEntry))
	readsTotal   = metrics.NewCounter("notifications_mark_read_total", "Read marks by what was found.", outcomeLabel)
)

// Service marks entries read.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// MarkRead marks userUUID's entry read. Another user's entry is unknown.
func (s *Service) MarkRead(ctx context.Context, userUUID, entryUUID string) (notifications.MarkReadOutcome, error) {
	defer logging.Step(ctx, "notifications.mark_read").Close()
	outcome, err := s.mark(ctx, userUUID, entryUUID)
	if err != nil {
		return "", err
	}
	readsTotal.Inc(ctx, outcomeLabel.Value(string(outcome)))
	return outcome, nil
}

func (s *Service) mark(ctx context.Context, userUUID, entryUUID string) (notifications.MarkReadOutcome, error) {
	if _, err := uuid.Parse(entryUUID); err != nil {
		return notifications.UnknownEntry, nil
	}
	db := s.repo.GetDB()
	marked, err := s.repo.MarkRead(ctx, db, userUUID, entryUUID, s.now().UTC())
	if err != nil || marked {
		return notifications.MarkedRead, err
	}
	entry, err := s.repo.EntryByUUID(ctx, db, entryUUID)
	if err != nil {
		return "", err
	}
	if entry == nil || entry.UserUUID != userUUID || !entry.InApp {
		return notifications.UnknownEntry, nil
	}
	return notifications.AlreadyRead, nil
}
