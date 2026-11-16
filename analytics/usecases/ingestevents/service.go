// Package ingestevents stores the analytics events domains report: a batch
// at a time, each event once per source domain and idempotency key.
package ingestevents

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/analytics"
	internalrepo "github.com/diegobermudez03/playhoot/analytics/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

const (
	// maxKeyLen bounds identifiers coming from other domains.
	maxKeyLen = 200
	// maxPropertiesLen bounds an event's encoded properties: small values only.
	maxPropertiesLen = 2048
	// MaxBatch is the most events one call stores.
	MaxBatch = 500
)

var (
	outcomeLabel = metrics.NewLabel("outcome", "stored", "duplicate", "invalid")
	eventsTotal  = metrics.NewCounter(
		"analytics_events_total",
		"Analytics events ingested by what was decided.",
		outcomeLabel,
	)
)

// Service is the event ingestion.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// IngestEvents stores events from sourceDomain. An invalid event, or any
// event beyond MaxBatch, is skipped and counted as invalid; the rest are
// stored. An event dated in the future is stored as occurring now.
func (s *Service) IngestEvents(ctx context.Context, sourceDomain string, events []analytics.Event) (analytics.IngestResult, error) {
	defer logging.Step(ctx, "analytics.ingest_events").Close()
	var result analytics.IngestResult
	if sourceDomain == "" || len(sourceDomain) > maxKeyLen {
		result.Invalid = len(events)
		eventsTotal.Add(ctx, float64(result.Invalid), outcomeLabel.Value("invalid"))
		return result, nil
	}
	now := s.now().UTC()
	rows := make([]internalrepo.Event, 0, len(events))
	for i, event := range events {
		row, ok := toRow(sourceDomain, event, now)
		if !ok || i >= MaxBatch {
			result.Invalid++
			continue
		}
		rows = append(rows, row)
	}
	stored, err := s.repo.InsertEvents(ctx, rows)
	if err != nil {
		return analytics.IngestResult{}, err
	}
	result.Stored = stored
	result.Duplicates = len(rows) - stored
	eventsTotal.Add(ctx, float64(result.Stored), outcomeLabel.Value("stored"))
	eventsTotal.Add(ctx, float64(result.Duplicates), outcomeLabel.Value("duplicate"))
	eventsTotal.Add(ctx, float64(result.Invalid), outcomeLabel.Value("invalid"))
	return result, nil
}

func toRow(source string, e analytics.Event, now time.Time) (internalrepo.Event, bool) {
	user, err := uuid.Parse(e.UserRef)
	if err != nil ||
		!analytics.ValidEventType(e.EventType) ||
		!analytics.ValidAccountKind(e.AccountKind) || !analytics.ValidPlanKind(e.PlanKind) ||
		e.IdempotencyKey == "" || len(e.IdempotencyKey) > maxKeyLen ||
		e.EntityRef == "" || len(e.EntityRef) > maxKeyLen {
		return internalrepo.Event{}, false
	}
	properties := e.Properties
	if properties == nil {
		properties = map[string]any{}
	}
	encoded, err := json.Marshal(properties)
	if err != nil || len(encoded) > maxPropertiesLen {
		return internalrepo.Event{}, false
	}
	occurred := e.OccurredAt.UTC()
	if occurred.IsZero() || occurred.After(now) {
		occurred = now
	}
	return internalrepo.Event{
		EventType: e.EventType, UserRef: user.String(), AccountKind: e.AccountKind, PlanKind: e.PlanKind,
		EntityRef: e.EntityRef, Properties: string(encoded), SourceDomain: source, IdempotencyKey: e.IdempotencyKey,
		OccurredAt: occurred, CreatedAt: now,
	}, true
}
