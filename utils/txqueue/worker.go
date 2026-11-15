package txqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// WorkerConfig tunes a queue's worker and retry policy.
type WorkerConfig struct {
	// PollInterval is how long the worker waits when nothing is due.
	PollInterval time.Duration
	// Batch is how many tasks one claim takes.
	Batch int
	// Lease is how long a claimed task is held; a worker that dies leaves it
	// to be claimed again when the lease ends. It also bounds a handler run.
	Lease time.Duration
	// MaxAttempts is how many runs a task gets before it turns dead.
	MaxAttempts int
	// BaseBackoff is the wait after the first failure; it doubles per
	// attempt up to MaxBackoff, with jitter.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
}

// DefaultWorkerConfig is the retry policy every domain queue uses: 10
// attempts, backoff from 10 s doubling up to 1 h, with jitter.
var DefaultWorkerConfig = WorkerConfig{
	PollInterval: 2 * time.Second,
	Batch:        10,
	Lease:        5 * time.Minute,
	MaxAttempts:  10,
	BaseBackoff:  10 * time.Second,
	MaxBackoff:   time.Hour,
}

var (
	outcomeLabel = metrics.NewLabel("outcome", "completed", "retried", "dead", "lease_lost")
	tasksTotal   = metrics.NewCounter(
		"txqueue_tasks_total",
		"Queue task runs by what came of them: completed (deleted), retried later, dead, or finished after their lease was lost to another worker.",
		outcomeLabel,
	)
	taskDuration = metrics.NewHistogram(
		"txqueue_task_duration_seconds",
		"How long a queue task's handler ran.",
		metrics.LatencyBuckets(),
	)
	claimFailures = metrics.NewCounter(
		"txqueue_claim_failures_total",
		"Polls that could not claim due tasks; the next poll tries again.",
	)
)

// backoff is the wait before the run after attempt failed: BaseBackoff
// doubled per earlier attempt, capped at MaxBackoff, of which a random half
// is taken off so failing tasks spread out.
func (c WorkerConfig) backoff(attempt int, random func() float64) time.Duration {
	wait := c.BaseBackoff
	for i := 1; i < attempt && wait < c.MaxBackoff; i++ {
		wait *= 2
	}
	if wait > c.MaxBackoff {
		wait = c.MaxBackoff
	}
	half := wait / 2
	return half + time.Duration(random()*float64(half))
}

// Run works the queue until ctx is done: it schedules the periodic tasks,
// then claims and runs due tasks, polling when none are left. Every instance
// runs it; a task is claimed by one worker at a time.
func (q *Queue) Run(ctx context.Context) {
	logger := slog.Default()
	scheduled := false
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if !scheduled {
			if err := q.SchedulePeriodic(ctx); err != nil {
				logging.LogStandalone(logger, []string{"txqueue"}, "scheduling periodic tasks failed", logging.Field("domain", q.domain), logging.Field("error", err.Error()))
			} else {
				scheduled = true
			}
		}
		claimed, err := q.ProcessDue(ctx)
		if err != nil {
			claimFailures.Inc(ctx)
			logging.LogStandalone(logger, []string{"txqueue"}, "claiming tasks failed", logging.Field("domain", q.domain), logging.Field("error", err.Error()))
		}
		wait := q.config.PollInterval
		if err == nil && claimed == q.config.Batch {
			wait = 0
		}
		timer.Reset(wait)
	}
}

// SchedulePeriodic makes sure every periodic task type has its task; one
// already pending, running or dead is left as it is.
func (q *Queue) SchedulePeriodic(ctx context.Context) error {
	handlers := q.registered()
	types := make([]string, 0, len(handlers))
	for taskType, r := range handlers {
		if r.every > 0 {
			types = append(types, taskType)
		}
	}
	sort.Strings(types)
	if len(types) == 0 {
		return nil
	}
	_, err := utils.RunInDBTransaction(ctx, q.store, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		for _, taskType := range types {
			if err := q.Enqueue(tx.WithContext(ctx), taskType, struct{}{}, time.Time{}, taskType); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	})
	return err
}

// ProcessDue claims one batch of due tasks with a registered handler and runs
// them, answering how many it claimed. Tasks of a type nobody handles yet
// wait in the queue.
func (q *Queue) ProcessDue(ctx context.Context) (int, error) {
	handlers := q.registered()
	if len(handlers) == 0 {
		return 0, nil
	}
	types := make([]string, 0, len(handlers))
	for taskType := range handlers {
		types = append(types, taskType)
	}
	sort.Strings(types)
	now := q.now().UTC()
	rows, err := q.store.claim(ctx, types, now, now.Add(q.config.Lease), q.config.Batch)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		q.run(ctx, handlers[row.Type], row)
	}
	return len(rows), nil
}

// run runs one claimed task and records what came of it: deleted when it
// completed (scheduling the next run of a periodic one), back to pending with
// backoff when it failed, dead when it failed for good.
func (q *Queue) run(ctx context.Context, r registration, row claimedRow) {
	taskCtx := logging.Start(ctx)
	defer logging.FinishRequestLog(taskCtx, slog.Default(), "queue task")
	logging.LogFields(taskCtx, logging.Field("domain", q.domain), logging.Field("task_type", row.Type), logging.Field("task_uuid", row.UUID), logging.Field("attempt", row.Attempts))

	task := Task{UUID: row.UUID, Type: row.Type, Payload: json.RawMessage(row.Payload), Attempt: row.Attempts}
	handlerCtx, cancel := context.WithTimeout(taskCtx, q.config.Lease)
	started := time.Now()
	err := callHandler(handlerCtx, r.handler, task)
	cancel()
	taskDuration.Observe(taskCtx, time.Since(started).Seconds())

	if err == nil {
		q.completed(taskCtx, r, row)
		return
	}
	logging.LogError(taskCtx, err)
	now := q.now().UTC()
	var permanent permanentError
	if errors.As(err, &permanent) || row.Attempts >= q.config.MaxAttempts {
		if failErr := q.store.fail(taskCtx, row.UUID, row.Attempts, StatusDead, now, err.Error(), now); failErr != nil {
			logging.LogError(taskCtx, failErr)
			return
		}
		tasksTotal.Inc(taskCtx, outcomeLabel.Value("dead"))
		logging.LogFields(taskCtx, logging.Field("task_outcome", "dead"))
		return
	}
	retryAt := now.Add(q.config.backoff(row.Attempts, rand.Float64))
	if failErr := q.store.fail(taskCtx, row.UUID, row.Attempts, StatusPending, retryAt, err.Error(), now); failErr != nil {
		logging.LogError(taskCtx, failErr)
		return
	}
	tasksTotal.Inc(taskCtx, outcomeLabel.Value("retried"))
	logging.LogFields(taskCtx, logging.Field("task_outcome", "retried"), logging.Field("retry_at", retryAt))
}

func (q *Queue) completed(ctx context.Context, r registration, row claimedRow) {
	deleted, err := utils.RunInDBTransaction(ctx, q.store, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		deleted, err := q.store.complete(ctx, tx, row.UUID, row.Attempts)
		if err != nil || !deleted || r.every <= 0 {
			return deleted, err
		}
		return true, q.Enqueue(tx.WithContext(ctx), row.Type, struct{}{}, q.now().UTC().Add(r.every), row.Type)
	})
	if err != nil {
		// The task stays running until its lease ends, then runs again.
		logging.LogError(ctx, err)
		return
	}
	outcome := "completed"
	if !deleted {
		outcome = "lease_lost"
	}
	tasksTotal.Inc(ctx, outcomeLabel.Value(outcome))
	logging.LogFields(ctx, logging.Field("task_outcome", outcome))
}

// callHandler runs handler, turning a panic into an error so one bad task
// does not stop the worker.
func callHandler(ctx context.Context, handler Handler, task Task) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("handler panicked: %v", recovered)
		}
	}()
	return handler(ctx, task)
}
