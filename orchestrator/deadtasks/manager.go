// Package deadtasks is admin's retry or cancel of a dead task on its owning
// domain queue, through the queue registry.
package deadtasks

import (
	"context"

	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// registry changes dead tasks on whichever domain queue holds them.
type registry interface {
	RetryDeadTask(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error)
	CancelDeadTask(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error)
}

// Manager is the retry-or-cancel flow.
type Manager struct {
	queues registry
}

// New constructs a Manager over queues.
func New(queues registry) *Manager {
	return &Manager{queues: queues}
}

// RetryDeadTask puts a dead task back to pending with fresh attempts. A task
// that is no longer dead is left as it is (DeadTaskNotDead).
func (m *Manager) RetryDeadTask(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error) {
	return m.queues.RetryDeadTask(ctx, taskUUID)
}

// CancelDeadTask cancels a dead task for good. A task that is no longer dead
// is left as it is (DeadTaskNotDead).
func (m *Manager) CancelDeadTask(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error) {
	return m.queues.CancelDeadTask(ctx, taskUUID)
}
