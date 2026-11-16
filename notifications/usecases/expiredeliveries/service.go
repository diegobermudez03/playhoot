// Package expiredeliveries deletes email delivery records older than 30
// days, as a periodic task of Notifications' queue.
package expiredeliveries

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// TaskType runs every Every and deletes Retention-old records, Batch per
// statement.
const (
	TaskType  = "expire-email-deliveries"
	Every     = time.Hour
	Retention = 30 * 24 * time.Hour
	Batch     = 500
)

var expiredTotal = metrics.NewCounter("notifications_deliveries_expired_total", "Email delivery records deleted at 30 days.")

// Service expires delivery records.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// Register makes the expiry a periodic task of queue.
func (s *Service) Register(queue *txqueue.Queue) {
	queue.HandlePeriodic(TaskType, Every, func(ctx context.Context, _ txqueue.Task) error {
		_, err := s.Expire(ctx)
		return err
	})
}

// Expire deletes every record older than Retention, answering how many.
func (s *Service) Expire(ctx context.Context) (int64, error) {
	defer logging.Step(ctx, "notifications.expire_deliveries").Close()
	cutoff := s.now().UTC().Add(-Retention)
	var total int64
	for {
		deleted, err := s.repo.DeleteDeliveriesBefore(ctx, s.repo.GetDB(), cutoff, Batch)
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < Batch {
			break
		}
	}
	if total > 0 {
		expiredTotal.Add(ctx, float64(total))
	}
	return total, nil
}
