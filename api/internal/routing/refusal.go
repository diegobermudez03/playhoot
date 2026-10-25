package routing

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// Reasons the edge's own limits refuse a request, as clients read them.
const (
	// RefusedRateLimited means the caller sent more requests to the route
	// than its limits allow.
	RefusedRateLimited = "rate_limited"
	// RefusedTooManyConnections means the caller has as many live
	// connections as it may.
	RefusedTooManyConnections = "too_many_connections"
	// RefusedServerBusy means the instance serves as many live connections
	// as it may.
	RefusedServerBusy = "server_busy"
)

// LimitRefusal is why the edge's limits refused a request.
type LimitRefusal struct {
	// Reason is one of the Refused reasons.
	Reason string
	// RetryAfter is when trying again may succeed; zero when unknown.
	RetryAfter time.Duration
}

// LimitRefuser answers r, refused by a limit, in the route's own way and
// reports true, or reports false without answering when it cannot, leaving
// the HTTP error to the caller.
type LimitRefuser func(w http.ResponseWriter, r *http.Request, refusal LimitRefusal) bool

type limitRefusalNoteKey struct{}

// WithLimitRefusalNote returns ctx able to remember which limit refused its
// request, and a function reporting that reason; empty when none did.
func WithLimitRefusalNote(ctx context.Context) (context.Context, func() string) {
	note := new(atomic.Pointer[string])
	return context.WithValue(ctx, limitRefusalNoteKey{}, note), func() string {
		if reason := note.Load(); reason != nil {
			return *reason
		}
		return ""
	}
}

// NoteLimitRefusal remembers that reason refused ctx's request, when ctx was
// prepared by WithLimitRefusalNote; otherwise it does nothing.
func NoteLimitRefusal(ctx context.Context, reason string) {
	if note, ok := ctx.Value(limitRefusalNoteKey{}).(*atomic.Pointer[string]); ok {
		note.Store(&reason)
	}
}
