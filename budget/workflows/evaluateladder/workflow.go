// Package evaluateladder moves the restriction ladder from the numbers:
// escalation on pressure and automatic de-escalation below shutdown. It runs
// after spend ingestion and as a periodic task; shutdown never lifts here.
package evaluateladder

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/ladder"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/budget/usecases/recordspend"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// PeriodicTaskType is the periodic evaluation on Budget's queue, run Every.
const (
	PeriodicTaskType = "evaluate-ladder"
	Every            = time.Minute
)

// Workflow is the ladder evaluation.
type Workflow struct {
	repo  *internalrepo.Repo
	queue ladder.Enqueuer
	now   func() time.Time
}

// New constructs a Workflow on db that leaves notify tasks in queue.
func New(db *gorm.DB, queue ladder.Enqueuer) *Workflow {
	return &Workflow{repo: internalrepo.New(db), queue: queue, now: time.Now}
}

// Register makes the evaluation a periodic task of Budget's queue and the
// handler of the evaluation each recorded spend asks for.
func (w *Workflow) Register(queue *txqueue.Queue) {
	run := func(ctx context.Context, _ txqueue.Task) error {
		_, err := w.Evaluate(ctx)
		return err
	}
	queue.HandlePeriodic(PeriodicTaskType, Every, run)
	queue.Handle(recordspend.EvaluateAfterSpendTask, run)
}

// Evaluate reads the numbers, decides the step and appends it when it
// differs from the current one. It answers the state in force afterwards.
func (w *Workflow) Evaluate(ctx context.Context) (budget.LadderState, error) {
	defer logging.Step(ctx, "budget.evaluate_ladder").Close()
	state, err := utils.RunInDBTransaction(ctx, w.repo, func(ctx context.Context, tx *gorm.DB) (budget.LadderState, error) {
		if err := w.repo.Lock(ctx, tx); err != nil {
			return budget.LadderState{}, err
		}
		now := w.now().UTC()
		snap, err := stepSum(ctx, w.repo, tx, now)
		if err != nil {
			return budget.LadderState{}, err
		}
		next, changed := stepCompare(snap)
		if !changed {
			return snap.Ladder, nil
		}
		if err := stepAppendAndNotify(ctx, w.repo, w.queue, tx, next, now); err != nil {
			return budget.LadderState{}, err
		}
		return next, nil
	})
	if err != nil {
		return budget.LadderState{}, err
	}
	logging.LogFields(ctx, logging.Field("budget_level", string(state.Level)))
	return state, nil
}
