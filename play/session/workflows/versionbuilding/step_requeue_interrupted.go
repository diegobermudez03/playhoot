package versionbuilding

import (
	"context"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versionbuilding -destination=step_requeue_interrupted_mock_test.go . requeueRepo

// requeueRepo is RequeueInterrupted's own narrow persistence contract.
type requeueRepo interface {
	RequeueAll(ctx context.Context, tx *gorm.DB, from string, to string) (int, error)
}

// RequeueInterrupted puts back in the queue every build left running, which
// can only be one an instance was running when it stopped: the instance
// starting is the only one (a single instance until launch).
func (m *Manager) RequeueInterrupted(ctx context.Context) (int, error) {
	defer logging.Step(ctx, "versionbuilding.requeue_interrupted").Close()
	requeued, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (int, error) {
		return m.requeueRepo.RequeueAll(ctx, tx, string(build.Running), string(build.Queued))
	})
	if err != nil {
		return 0, err
	}
	logging.LogFields(ctx, logging.Field("builds_requeued", requeued))
	return requeued, nil
}
