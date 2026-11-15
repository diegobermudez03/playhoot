// Package sweepresourcegrants is Identity's periodic housekeeping of
// single-use grants: grants expired for more than a day are deleted. It runs
// as a periodic task on Identity's queue.
package sweepresourcegrants

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/usecases/sweepresourcegrants/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// TaskType is the sweep's task on Identity's queue.
const TaskType = "sweep-resource-grants"

// Every is how often the sweep runs.
const Every = time.Hour

// expiredRetention is how long an expired grant is kept, and batch how many
// one statement deletes.
const (
	expiredRetention = 24 * time.Hour
	batch            = 500
)

var sweptGrants = metrics.NewCounter(
	"identity_resource_grants_swept_total",
	"Single-use grants deleted for having expired more than a day ago.",
)

// grantSweeper deletes expired grants.
type grantSweeper interface {
	DeleteExpiredGrants(ctx context.Context, before time.Time, limit int) (int64, error)
}

// Service is the grant sweep.
type Service struct {
	grants grantSweeper
	now    func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{grants: repo.New(db), now: time.Now}
}

// Register makes the sweep a periodic task of Identity's queue.
func (s *Service) Register(queue *txqueue.Queue) {
	queue.HandlePeriodic(TaskType, Every, func(ctx context.Context, _ txqueue.Task) error {
		_, err := s.Sweep(ctx)
		return err
	})
}

// Sweep deletes every grant expired for longer than the retention, answering
// how many. Running it again deletes nothing new, so a repeated run is safe.
func (s *Service) Sweep(ctx context.Context) (int64, error) {
	defer logging.Step(ctx, "sweepresourcegrants.sweep").Close()
	before := s.now().UTC().Add(-expiredRetention)
	var total int64
	for {
		deleted, err := s.grants.DeleteExpiredGrants(ctx, before, batch)
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < batch {
			break
		}
	}
	if total > 0 {
		sweptGrants.Add(ctx, float64(total))
	}
	logging.LogFields(ctx, logging.Field("resource_grants_swept", total))
	return total, nil
}
