// Package deadtasks is admin's view of every domain's dead letter queue,
// read through the queue registry: the queue tables are infrastructure, so
// no domain operation is involved.
package deadtasks

import (
	"context"

	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// registry lists the dead tasks of every domain queue.
type registry interface {
	ListDeadTasks(ctx context.Context) (txqueue.DeadTasks, error)
}

// Reader is the admin dead letter queues read.
type Reader struct {
	queues registry
}

// New constructs a Reader over queues.
func New(queues registry) *Reader {
	return &Reader{queues: queues}
}

// ListDeadTasks lists every domain's dead tasks; a queue that cannot be read
// is named in Unavailable while the others still show.
func (r *Reader) ListDeadTasks(ctx context.Context) (txqueue.DeadTasks, error) {
	return r.queues.ListDeadTasks(ctx)
}
