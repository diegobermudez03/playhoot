package txqueue

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
)

// DeadListLimit is how many dead tasks admin sees per domain queue, newest
// first.
const DeadListLimit = 100

// DeadTask is a task that ran out of attempts, as admin sees it.
type DeadTask struct {
	Domain       string
	UUID         string
	Type         string
	Payload      json.RawMessage
	Attempts     int
	LastError    string
	AdminRetries int
	CreatedAt    time.Time
	DiedAt       time.Time
}

// DeadTasks is every domain's dead letter queue. Unavailable names the
// domains whose queue could not be read; the others are still listed.
type DeadTasks struct {
	Tasks       []DeadTask
	Unavailable []string
}

// DeadTaskOutcome is what an admin retry or cancel did.
type DeadTaskOutcome string

const (
	// DeadTaskRetried means the task is pending again with fresh attempts.
	DeadTaskRetried DeadTaskOutcome = "retried"
	// DeadTaskCancelled means the task will never run again.
	DeadTaskCancelled DeadTaskOutcome = "cancelled"
	// DeadTaskNotDead means the task exists but is not dead: nothing changed.
	DeadTaskNotDead DeadTaskOutcome = "not_dead"
	// DeadTaskNotFound means no queue holds the task.
	DeadTaskNotFound DeadTaskOutcome = "not_found"
)

// Registry holds every domain queue of the process, for their workers and for
// admin.
type Registry struct {
	db     *gorm.DB
	config WorkerConfig
	now    func() time.Time
	mu     sync.Mutex
	queues map[string]*Queue
}

// NewRegistry constructs a Registry whose queues live in db and work with
// config.
func NewRegistry(db *gorm.DB, config WorkerConfig) *Registry {
	return &Registry{db: db, config: config, now: time.Now, queues: map[string]*Queue{}}
}

// Queue returns domain's queue, creating it on first use. Its table comes
// from the domain's migration (Migration).
func (r *Registry) Queue(domain string) *Queue {
	r.mu.Lock()
	defer r.mu.Unlock()
	if q, ok := r.queues[domain]; ok {
		return q
	}
	q := newQueue(r.db, domain, r.config)
	r.queues[domain] = q
	return q
}

func (r *Registry) sorted() []*Queue {
	r.mu.Lock()
	defer r.mu.Unlock()
	queues := make([]*Queue, 0, len(r.queues))
	for _, q := range r.queues {
		queues = append(queues, q)
	}
	sort.Slice(queues, func(i, j int) bool { return queues[i].domain < queues[j].domain })
	return queues
}

// Run runs every queue's worker until ctx is done, and returns once they all
// stopped.
func (r *Registry) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, q := range r.sorted() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Run(ctx)
		}()
	}
	wg.Wait()
}

// ListDeadTasks lists the dead tasks of every queue.
func (r *Registry) ListDeadTasks(ctx context.Context) (DeadTasks, error) {
	defer logging.Step(ctx, "txqueue.list-dead").Close()
	result := DeadTasks{Tasks: []DeadTask{}, Unavailable: []string{}}
	for _, q := range r.sorted() {
		rows, err := q.store.listDead(ctx, DeadListLimit)
		if err != nil {
			logging.LogError(ctx, err)
			result.Unavailable = append(result.Unavailable, q.domain)
			continue
		}
		for _, row := range rows {
			task := DeadTask{Domain: q.domain, UUID: row.UUID, Type: row.Type, Payload: json.RawMessage(row.Payload), Attempts: row.Attempts, AdminRetries: row.AdminRetries, CreatedAt: row.CreatedAt, DiedAt: row.DiedAt}
			if row.LastError != nil {
				task.LastError = *row.LastError
			}
			result.Tasks = append(result.Tasks, task)
		}
	}
	return result, nil
}

// RetryDeadTask puts a dead task back to pending on its queue. A task that is
// no longer dead is left as it is.
func (r *Registry) RetryDeadTask(ctx context.Context, taskUUID string) (DeadTaskOutcome, error) {
	defer logging.Step(ctx, "txqueue.retry-dead").Close()
	return r.applyToDead(ctx, taskUUID, DeadTaskRetried, func(s *store, now time.Time) (bool, error) {
		return s.retryDead(ctx, taskUUID, now)
	})
}

// CancelDeadTask cancels a dead task on its queue. A task that is no longer
// dead, cancelled ones included, is left as it is.
func (r *Registry) CancelDeadTask(ctx context.Context, taskUUID string) (DeadTaskOutcome, error) {
	defer logging.Step(ctx, "txqueue.cancel-dead").Close()
	return r.applyToDead(ctx, taskUUID, DeadTaskCancelled, func(s *store, now time.Time) (bool, error) {
		return s.cancelDead(ctx, taskUUID, now)
	})
}

func (r *Registry) applyToDead(ctx context.Context, taskUUID string, done DeadTaskOutcome, apply func(s *store, now time.Time) (bool, error)) (DeadTaskOutcome, error) {
	now := r.now().UTC()
	for _, q := range r.sorted() {
		changed, err := apply(q.store, now)
		if err != nil {
			return "", err
		}
		if changed {
			logging.LogFields(ctx, logging.Field("domain", q.domain), logging.Field("task_uuid", taskUUID), logging.Field("dead_task", string(done)))
			return done, nil
		}
		held, err := q.store.has(ctx, taskUUID)
		if err != nil {
			return "", err
		}
		if held {
			logging.LogFields(ctx, logging.Field("domain", q.domain), logging.Field("task_uuid", taskUUID), logging.Field("dead_task", string(DeadTaskNotDead)))
			return DeadTaskNotDead, nil
		}
	}
	return DeadTaskNotFound, nil
}
