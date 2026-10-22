// Package limitaudit gathers the platform limits live Session users hit and
// has them recorded for auditing in batches, best effort: hitting a limit
// never waits for storage, and a flood is one record a few seconds later.
package limitaudit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=limitaudit -destination=auditor_mock_test.go . hitStore

// hitStore records limit hits for auditing.
type hitStore interface {
	RecordLimitHits(ctx context.Context, hits []sessionpkg.LimitHit) (int, error)
}

const (
	// flushInterval is how often gathered hits are recorded.
	flushInterval = 5 * time.Second
	// finalFlushTimeout bounds the last recording when the instance stops.
	finalFlushTimeout = 5 * time.Second
	// maxPendingKeys bounds the distinct Session, user and limit triples
	// gathered between recordings; hits of new ones beyond it are dropped.
	maxPendingKeys = 10_000
)

const (
	flushRecorded = "recorded"
	flushFailed   = "failed"
	hitsDropped   = "dropped"
)

var (
	outcomeLabel = metrics.NewLabel("outcome", flushRecorded, flushFailed, hitsDropped)

	limitHitsAudited = metrics.NewCounter(
		"play_runtime_limit_hits_audited_total",
		"Limit hits gathered for the audit records, by outcome: recorded, failed (the batch could not be stored) or dropped (too many distinct hits waiting).",
		outcomeLabel,
	)
)

type hitKey struct {
	sessionUUID string
	userUUID    string
	limit       string
}

// Auditor gathers limit hits and records them every few seconds.
type Auditor struct {
	store    hitStore
	interval time.Duration

	mu      sync.Mutex
	pending map[hitKey]int
}

// New constructs an Auditor recording through store.
func New(store hitStore) *Auditor {
	return &Auditor{store: store, interval: flushInterval, pending: make(map[hitKey]int)}
}

// Hit counts one refusal of limit for userUUID in sessionUUID. It never
// blocks on storage.
func (a *Auditor) Hit(ctx context.Context, sessionUUID, userUUID, limit string) {
	key := hitKey{sessionUUID: sessionUUID, userUUID: userUUID, limit: limit}
	a.mu.Lock()
	_, known := a.pending[key]
	if !known && len(a.pending) >= maxPendingKeys {
		a.mu.Unlock()
		limitHitsAudited.Inc(ctx, outcomeLabel.Value(hitsDropped))
		return
	}
	a.pending[key]++
	a.mu.Unlock()
}

// Run records the gathered hits every interval until ctx is done, then once
// more within finalFlushTimeout.
func (a *Auditor) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			a.Flush(ctx)
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalFlushTimeout)
			a.Flush(final)
			cancel()
			return
		}
	}
}

// Flush records what was gathered so far. A failed batch is logged and
// dropped: the audit is best effort.
func (a *Auditor) Flush(ctx context.Context) {
	a.mu.Lock()
	if len(a.pending) == 0 {
		a.mu.Unlock()
		return
	}
	gathered := a.pending
	a.pending = make(map[hitKey]int)
	a.mu.Unlock()

	hits := make([]sessionpkg.LimitHit, 0, len(gathered))
	for key, count := range gathered {
		hits = append(hits, sessionpkg.LimitHit{SessionUUID: key.sessionUUID, UserUUID: key.userUUID, Limit: key.limit, Count: count})
	}
	if _, err := a.store.RecordLimitHits(ctx, hits); err != nil {
		logging.LogError(ctx, fmt.Errorf("recording %d limit hits for audit: %w", len(hits), err))
		limitHitsAudited.Add(ctx, float64(len(hits)), outcomeLabel.Value(flushFailed))
		return
	}
	limitHitsAudited.Add(ctx, float64(len(hits)), outcomeLabel.Value(flushRecorded))
}
