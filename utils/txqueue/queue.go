// Package txqueue is the one way to do asynchronous work: each domain keeps a
// queue table it enqueues into inside the transaction of the write that caused
// the work, and a worker that runs the domain's handlers with retries. A task
// that keeps failing turns dead, which is the dead letter queue.
package txqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Statuses of a task. A completed task is deleted, not kept.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusDead      = "dead"
	StatusCancelled = "cancelled"
)

// Cross-domain delivery types: a domain enqueues them to tell another domain
// something, and only the orchestrator registers their handlers.
const (
	TypeNotify         = "notify"
	TypeAnalyticsEvent = "analytics-event"
	TypeSpendEvent     = "spend-event"
)

var deliveryTypes = map[string]bool{TypeNotify: true, TypeAnalyticsEvent: true, TypeSpendEvent: true}

// IsDeliveryType reports whether taskType is a cross-domain delivery type.
func IsDeliveryType(taskType string) bool {
	return deliveryTypes[taskType]
}

// Handler runs one task. It must be idempotent: a task may run again after a
// crash or a lost completion. An error retries the task with backoff; an
// error wrapped by Permanent turns it dead at once.
type Handler func(ctx context.Context, task Task) error

// Task is a claimed task as its handler sees it.
type Task struct {
	UUID    string
	Type    string
	Payload json.RawMessage
	// Attempt counts runs including this one, from 1.
	Attempt int
}

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as one retrying cannot fix, such as a payload the
// handler cannot read: the task turns dead without further attempts.
func Permanent(err error) error {
	return permanentError{err: err}
}

var domainName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

// TableName is the queue table of domain.
func TableName(domain string) string {
	return domain + "_queue_tasks"
}

type registration struct {
	handler Handler
	// every is set for a periodic task: it runs again that long after it
	// completes.
	every time.Duration
}

// Queue is one domain's queue: enqueueing, handler registration and its
// worker. Build it with Registry.Queue.
type Queue struct {
	domain   string
	table    string
	store    *store
	config   WorkerConfig
	now      func() time.Time
	newUUID  func() string
	mu       sync.Mutex
	handlers map[string]registration
}

func newQueue(db *gorm.DB, domain string, config WorkerConfig) *Queue {
	if !domainName.MatchString(domain) {
		panic(fmt.Sprintf("txqueue: invalid domain name %q", domain))
	}
	table := TableName(domain)
	return &Queue{
		domain:   domain,
		table:    table,
		store:    &store{db: db, table: table},
		config:   config,
		now:      time.Now,
		newUUID:  uuid.NewString,
		handlers: map[string]registration{},
	}
}

// Domain is the domain owning the queue.
func (q *Queue) Domain() string {
	return q.domain
}

// Enqueue adds a task to the queue inside tx, the transaction of the write
// that caused it, due at runAt (now when zero). A non-empty idempotencyKey
// makes the call a no-op while another task with the same type and key is
// pending, running or dead.
func (q *Queue) Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error {
	if taskType == "" {
		return errors.New("txqueue: empty task type")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("txqueue: encoding %s payload: %w", taskType, err)
	}
	now := q.now().UTC()
	if runAt.IsZero() {
		runAt = now
	}
	row := taskRow{UUID: q.newUUID(), Type: taskType, Payload: string(body), RunAt: runAt.UTC(), Status: StatusPending, CreatedAt: now, UpdatedAt: now}
	if idempotencyKey != "" {
		row.IdempotencyKey = &idempotencyKey
	}
	if err := tx.Table(q.table).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("txqueue: enqueueing %s on %s: %w", taskType, q.domain, err)
	}
	return nil
}

// Handle registers the owning domain's handler for taskType. Delivery types
// are refused: the orchestrator registers those with HandleDelivery.
func (q *Queue) Handle(taskType string, handler Handler) {
	if IsDeliveryType(taskType) {
		panic(fmt.Sprintf("txqueue: %s is a delivery type; the orchestrator registers it", taskType))
	}
	q.register(taskType, registration{handler: handler})
}

// HandlePeriodic registers the owning domain's handler for a task that runs
// every interval: the worker schedules it when it starts and again each time
// it completes. Only one task of the type is ever scheduled.
func (q *Queue) HandlePeriodic(taskType string, every time.Duration, handler Handler) {
	if IsDeliveryType(taskType) || every <= 0 {
		panic(fmt.Sprintf("txqueue: invalid periodic task %s every %s", taskType, every))
	}
	q.register(taskType, registration{handler: handler, every: every})
}

// HandleDelivery is the orchestrator's registration point for the
// cross-domain delivery types; it refuses any other type.
func (q *Queue) HandleDelivery(taskType string, handler Handler) {
	if !IsDeliveryType(taskType) {
		panic(fmt.Sprintf("txqueue: %s is not a delivery type", taskType))
	}
	q.register(taskType, registration{handler: handler})
}

func (q *Queue) register(taskType string, r registration) {
	if taskType == "" || r.handler == nil {
		panic("txqueue: a handler needs a task type and a function")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, taken := q.handlers[taskType]; taken {
		panic(fmt.Sprintf("txqueue: %s already has a handler on %s", taskType, q.domain))
	}
	q.handlers[taskType] = r
}

func (q *Queue) registered() map[string]registration {
	q.mu.Lock()
	defer q.mu.Unlock()
	handlers := make(map[string]registration, len(q.handlers))
	for taskType, r := range q.handlers {
		handlers[taskType] = r
	}
	return handlers
}
