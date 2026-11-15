package txqueue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/utils"
)

var testDomains = []string{"qalpha", "qbeta"}

var queueDB = utils.DisposableTestDB{
	Key:        "txqueue",
	NameSuffix: "txqueue_pkg_test",
	Migrate: func(db *gorm.DB) error {
		migrations := make([]*gormigrate.Migration, len(testDomains))
		for i, domain := range testDomains {
			migrations[i] = Migration(fmt.Sprintf("test_%s_queue", domain), domain)
		}
		return gormigrate.New(db, gormigrate.DefaultOptions, migrations).Migrate()
	},
}

func TestMain(m *testing.M) {
	os.Exit(queueDB.Run(m))
}

func TestBackoff(t *testing.T) {
	config := DefaultWorkerConfig
	tests := map[string]func(t *testing.T){
		"doubles from the base and takes up to half off": func(t *testing.T) {
			require.Equal(t, 5*time.Second, config.backoff(1, func() float64 { return 0 }))
			require.Equal(t, 10*time.Second, config.backoff(2, func() float64 { return 0 }))
			require.Equal(t, 40*time.Second, config.backoff(3, func() float64 { return 1 }))
		},
		"is capped at the maximum": func(t *testing.T) {
			require.Equal(t, time.Hour, config.backoff(30, func() float64 { return 1 }))
			require.Equal(t, 30*time.Minute, config.backoff(30, func() float64 { return 0 }))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestRegistration(t *testing.T) {
	noop := func(context.Context, Task) error { return nil }
	tests := map[string]func(t *testing.T){
		"a domain cannot handle a delivery type": func(t *testing.T) {
			q := newQueue(nil, "qalpha", DefaultWorkerConfig)
			require.Panics(t, func() { q.Handle(TypeNotify, noop) })
		},
		"the delivery registration takes only delivery types": func(t *testing.T) {
			q := newQueue(nil, "qalpha", DefaultWorkerConfig)
			require.Panics(t, func() { q.HandleDelivery("sweep", noop) })
			for _, taskType := range []string{TypeNotify, TypeAnalyticsEvent, TypeSpendEvent} {
				q.HandleDelivery(taskType, noop)
			}
		},
		"a type has one handler": func(t *testing.T) {
			q := newQueue(nil, "qalpha", DefaultWorkerConfig)
			q.Handle("sweep", noop)
			require.Panics(t, func() { q.HandlePeriodic("sweep", time.Minute, noop) })
		},
		"a domain name must be a plain identifier": func(t *testing.T) {
			require.Panics(t, func() { newQueue(nil, "x; DROP TABLE y", DefaultWorkerConfig) })
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

// clock is a settable time shared by a test's queues.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestQueue(t *testing.T) {
	ctx := context.Background()
	db := queueDB.Open(t)
	config := WorkerConfig{PollInterval: time.Millisecond, Batch: 10, Lease: time.Minute, MaxAttempts: 3, BaseBackoff: 10 * time.Second, MaxBackoff: time.Hour}

	setup := func(t *testing.T) (*Registry, *clock) {
		for _, domain := range testDomains {
			require.NoError(t, db.Exec(fmt.Sprintf(`TRUNCATE %s`, TableName(domain))).Error)
		}
		c := &clock{now: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)}
		registry := NewRegistry(db, config)
		registry.now = c.Now
		for _, domain := range testDomains {
			registry.Queue(domain).now = c.Now
		}
		return registry, c
	}
	enqueue := func(t *testing.T, q *Queue, taskType string, payload any, runAt time.Time, key string) {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return q.Enqueue(tx, taskType, payload, runAt, key)
		}))
	}
	statusOf := func(t *testing.T, q *Queue, taskType string) []string {
		var statuses []string
		require.NoError(t, db.Raw(fmt.Sprintf(`SELECT status FROM %s WHERE type = ? ORDER BY id`, q.table), taskType).Scan(&statuses).Error)
		return statuses
	}

	tests := map[string]func(t *testing.T){
		"a completed task is deleted and its handler saw the payload": func(t *testing.T) {
			registry, _ := setup(t)
			q := registry.Queue("qalpha")
			var seen Task
			q.Handle("greet", func(_ context.Context, task Task) error { seen = task; return nil })
			enqueue(t, q, "greet", map[string]string{"name": "Ana"}, time.Time{}, "")

			claimed, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, claimed)
			require.JSONEq(t, `{"name":"Ana"}`, string(seen.Payload))
			require.Equal(t, 1, seen.Attempt)
			require.Empty(t, statusOf(t, q, "greet"))
		},
		"an enqueue rolled back with its transaction leaves nothing": func(t *testing.T) {
			registry, _ := setup(t)
			q := registry.Queue("qalpha")
			_ = db.Transaction(func(tx *gorm.DB) error {
				require.NoError(t, q.Enqueue(tx, "greet", nil, time.Time{}, ""))
				return errors.New("the write failed")
			})
			require.Empty(t, statusOf(t, q, "greet"))
		},
		"a failed task retries after backoff, then turns dead after max attempts": func(t *testing.T) {
			registry, c := setup(t)
			q := registry.Queue("qalpha")
			runs := 0
			q.Handle("flaky", func(context.Context, Task) error { runs++; return errors.New("provider down") })
			enqueue(t, q, "flaky", nil, time.Time{}, "")

			_, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{StatusPending}, statusOf(t, q, "flaky"))
			var runAt time.Time
			require.NoError(t, db.Raw(fmt.Sprintf(`SELECT run_at FROM %s WHERE type = ?`, q.table), "flaky").Scan(&runAt).Error)
			wait := runAt.Sub(c.Now())
			require.GreaterOrEqual(t, wait, 5*time.Second)
			require.LessOrEqual(t, wait, 10*time.Second)

			claimed, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Zero(t, claimed, "not due before its backoff")

			for range config.MaxAttempts - 1 {
				c.Advance(time.Hour)
				_, err := q.ProcessDue(ctx)
				require.NoError(t, err)
			}
			require.Equal(t, config.MaxAttempts, runs)
			require.Equal(t, []string{StatusDead}, statusOf(t, q, "flaky"))
			c.Advance(24 * time.Hour)
			claimed, err = q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Zero(t, claimed, "a dead task never runs on its own")
		},
		"a permanent failure turns dead at once, and so does a panic at the last attempt": func(t *testing.T) {
			registry, c := setup(t)
			q := registry.Queue("qalpha")
			q.Handle("unreadable", func(context.Context, Task) error { return Permanent(errors.New("bad payload")) })
			q.Handle("panics", func(context.Context, Task) error { panic("boom") })
			enqueue(t, q, "unreadable", nil, time.Time{}, "")
			enqueue(t, q, "panics", nil, time.Time{}, "")
			for range config.MaxAttempts {
				_, err := q.ProcessDue(ctx)
				require.NoError(t, err)
				c.Advance(time.Hour)
			}
			require.Equal(t, []string{StatusDead}, statusOf(t, q, "unreadable"))
			require.Equal(t, []string{StatusDead}, statusOf(t, q, "panics"))
		},
		"admin lists dead tasks of every queue, retries one and cancels another": func(t *testing.T) {
			registry, c := setup(t)
			alpha, beta := registry.Queue("qalpha"), registry.Queue("qbeta")
			fixed := false
			alpha.Handle("job", func(context.Context, Task) error {
				if fixed {
					return nil
				}
				return Permanent(errors.New("broken"))
			})
			beta.Handle("job", func(context.Context, Task) error { return Permanent(errors.New("broken")) })
			enqueue(t, alpha, "job", nil, time.Time{}, "")
			enqueue(t, beta, "job", nil, time.Time{}, "")
			_, err := alpha.ProcessDue(ctx)
			require.NoError(t, err)
			_, err = beta.ProcessDue(ctx)
			require.NoError(t, err)

			dead, err := registry.ListDeadTasks(ctx)
			require.NoError(t, err)
			require.Empty(t, dead.Unavailable)
			require.Len(t, dead.Tasks, 2)
			require.Equal(t, "qalpha", dead.Tasks[0].Domain)
			require.Equal(t, "broken", dead.Tasks[0].LastError)
			require.Equal(t, "qbeta", dead.Tasks[1].Domain)

			outcome, err := registry.RetryDeadTask(ctx, dead.Tasks[0].UUID)
			require.NoError(t, err)
			require.Equal(t, DeadTaskRetried, outcome)
			outcome, err = registry.RetryDeadTask(ctx, dead.Tasks[0].UUID)
			require.NoError(t, err)
			require.Equal(t, DeadTaskNotDead, outcome, "retrying twice changes nothing")
			fixed = true
			c.Advance(time.Second)
			claimed, err := alpha.ProcessDue(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, claimed)
			require.Empty(t, statusOf(t, alpha, "job"))

			outcome, err = registry.CancelDeadTask(ctx, dead.Tasks[1].UUID)
			require.NoError(t, err)
			require.Equal(t, DeadTaskCancelled, outcome)
			outcome, err = registry.CancelDeadTask(ctx, dead.Tasks[1].UUID)
			require.NoError(t, err)
			require.Equal(t, DeadTaskNotDead, outcome, "cancelling twice changes nothing")
			require.Equal(t, []string{StatusCancelled}, statusOf(t, beta, "job"))
			c.Advance(48 * time.Hour)
			claimed, err = beta.ProcessDue(ctx)
			require.NoError(t, err)
			require.Zero(t, claimed, "a cancelled task never runs")

			outcome, err = registry.CancelDeadTask(ctx, "00000000-0000-0000-0000-000000000000")
			require.NoError(t, err)
			require.Equal(t, DeadTaskNotFound, outcome)
		},
		"a queue that cannot be read is reported and the others still show": func(t *testing.T) {
			registry, _ := setup(t)
			registry.Queue("qmissing")
			dead, err := registry.ListDeadTasks(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"qmissing"}, dead.Unavailable)
		},
		"an idempotency key keeps one live task, and a re-run after a lost lease changes nothing": func(t *testing.T) {
			registry, c := setup(t)
			q := registry.Queue("qalpha")
			enqueue(t, q, "once", nil, time.Time{}, "order-7")
			enqueue(t, q, "once", nil, time.Time{}, "order-7")
			require.Equal(t, []string{StatusPending}, statusOf(t, q, "once"))

			// A worker claims it and stalls past its lease; another one runs it.
			rows, err := q.store.claim(ctx, []string{"once"}, c.Now(), c.Now().Add(config.Lease), 1)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			c.Advance(config.Lease + time.Second)
			runs := 0
			q.Handle("once", func(context.Context, Task) error { runs++; return nil })
			claimed, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, claimed)
			require.Empty(t, statusOf(t, q, "once"))

			// The stalled worker finishes late: its completion is a no-op.
			q.completed(ctx, registration{handler: func(context.Context, Task) error { return nil }}, rows[0])
			require.Equal(t, 1, runs)
			require.Empty(t, statusOf(t, q, "once"))
		},
		"two workers never claim the same task": func(t *testing.T) {
			registry, _ := setup(t)
			q := registry.Queue("qalpha")
			const tasks = 60
			for i := range tasks {
				enqueue(t, q, "count", i, time.Time{}, "")
			}
			var runs sync.Map
			var total atomic.Int64
			handler := func(_ context.Context, task Task) error {
				if _, again := runs.LoadOrStore(task.UUID, true); again {
					return errors.New("claimed twice")
				}
				total.Add(1)
				return nil
			}
			other := newQueue(db, "qalpha", config)
			other.now = q.now
			q.Handle("count", handler)
			other.Handle("count", handler)
			var wg sync.WaitGroup
			for _, worker := range []*Queue{q, other} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						claimed, err := worker.ProcessDue(ctx)
						if err != nil || claimed == 0 {
							return
						}
					}
				}()
			}
			wg.Wait()
			require.EqualValues(t, tasks, total.Load())
			require.Empty(t, statusOf(t, q, "count"))
		},
		"a periodic task is scheduled once and re-enqueues itself on completion": func(t *testing.T) {
			registry, c := setup(t)
			q := registry.Queue("qalpha")
			runs := 0
			q.HandlePeriodic("sweep", time.Hour, func(context.Context, Task) error { runs++; return nil })
			require.NoError(t, q.SchedulePeriodic(ctx))
			require.NoError(t, q.SchedulePeriodic(ctx), "another instance starting")
			require.Equal(t, []string{StatusPending}, statusOf(t, q, "sweep"))

			_, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, runs)
			require.Equal(t, []string{StatusPending}, statusOf(t, q, "sweep"))
			claimed, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Zero(t, claimed, "the next run waits its interval")
			c.Advance(time.Hour)
			_, err = q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Equal(t, 2, runs)
		},
		"a task nobody handles yet waits in the queue": func(t *testing.T) {
			registry, _ := setup(t)
			q := registry.Queue("qalpha")
			q.Handle("other", func(context.Context, Task) error { return nil })
			enqueue(t, q, TypeNotify, map[string]string{"to": "admin"}, time.Time{}, "")
			claimed, err := q.ProcessDue(ctx)
			require.NoError(t, err)
			require.Zero(t, claimed)
			require.Equal(t, []string{StatusPending}, statusOf(t, q, TypeNotify))
		},
		"the worker runs until stopped": func(t *testing.T) {
			registry, _ := setup(t)
			q := registry.Queue("qalpha")
			done := make(chan struct{})
			q.Handle("ping", func(context.Context, Task) error { close(done); return nil })
			enqueue(t, q, "ping", nil, time.Time{}, "")
			runCtx, stop := context.WithCancel(ctx)
			stopped := make(chan struct{})
			go func() { registry.Run(runCtx); close(stopped) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("the worker did not run the task")
			}
			stop()
			<-stopped
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
