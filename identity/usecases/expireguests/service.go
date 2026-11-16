// Package expireguests ends the guests nobody has used for the term
// disclosed where a guest is created. They go through the same deletion a
// person asks for, so the games they played stay readable afterwards.
package expireguests

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/expireguests/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

//go:generate mockgen -package=expireguests -destination=service_mock_test.go . guestFinder,userDeleter

// TaskType is the sweep's task on Identity's queue, Every how often it runs,
// Term how long a guest may go unused and Batch how many one pass takes.
const (
	TaskType = "expire-guests"
	Every    = 24 * time.Hour
	Term     = 90 * 24 * time.Hour
	Batch    = 500
)

// guestFinder finds the guests whose term ran out.
type guestFinder interface {
	InactiveGuests(ctx context.Context, before time.Time, limit int) ([]string, error)
}

// userDeleter is Identity's deletion.
type userDeleter interface {
	DeleteUser(ctx context.Context, userUUID string, origin identity.DeleteOrigin) (identity.DeleteUserOutcome, error)
}

var expiredTotal = metrics.NewCounter(
	"identity_guests_expired_total",
	"Guests deleted for having gone the whole term without being used.",
)

// Service is Identity's guest sweep.
type Service struct {
	guests  guestFinder
	deleter userDeleter
	now     func() time.Time
}

// New constructs a Service on db, deleting each expired guest with deleter.
func New(db *gorm.DB, deleter userDeleter) *Service {
	return &Service{guests: internalrepo.New(db), deleter: deleter, now: time.Now}
}

// Register makes the sweep a periodic task of Identity's queue.
func (s *Service) Register(queue *txqueue.Queue) {
	queue.HandlePeriodic(TaskType, Every, func(ctx context.Context, _ txqueue.Task) error {
		_, err := s.Expire(ctx)
		return err
	})
}

// Expire deletes every guest whose term ran out, in batches, answering how
// many. A guest deleted between being found and being deleted is skipped;
// nothing is expired twice.
func (s *Service) Expire(ctx context.Context) (int64, error) {
	defer logging.Step(ctx, "identity.expire_guests").Close()

	before := s.now().UTC().Add(-Term)
	var total int64
	for {
		uuids, err := s.guests.InactiveGuests(ctx, before, Batch)
		if err != nil {
			return total, err
		}
		expired := 0
		for _, userUUID := range uuids {
			outcome, err := s.deleter.DeleteUser(ctx, userUUID, identity.DeleteExpiry)
			if err != nil {
				if errors.Is(err, identity.ErrUserNotFound) {
					continue
				}
				return total, err
			}
			if outcome == identity.UserDeleted {
				expired++
			}
		}
		total += int64(expired)
		if len(uuids) < Batch {
			break
		}
		// Every guest found is deleted, so the next pass starts after
		// them; a pass that deleted none of its batch would not move.
		if expired == 0 {
			break
		}
	}
	if total > 0 {
		expiredTotal.Add(ctx, float64(total))
	}
	logging.LogFields(ctx, logging.Field("guests_expired", total))
	return total, nil
}
