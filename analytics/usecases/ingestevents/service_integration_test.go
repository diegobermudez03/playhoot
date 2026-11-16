package ingestevents

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/analytics"
	"github.com/diegobermudez03/playhoot/analytics/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestIngestEvents(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenAnalyticsDB(t)
	service := New(db)

	event := func(key string) analytics.Event {
		return analytics.Event{
			EventType: analytics.EventGameCreated, UserRef: uuid.NewString(), AccountKind: analytics.AccountPersonal,
			PlanKind: analytics.PlanFree, EntityRef: uuid.NewString(), Properties: map[string]any{"a": 1}, IdempotencyKey: key,
			OccurredAt: time.Now().Add(-time.Minute),
		}
	}
	count := func(t *testing.T, source string) int {
		var n int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM analytics_events WHERE source_domain = ?`, source).Scan(&n).Error)
		return n
	}

	tests := map[string]func(t *testing.T){
		"a retried event is stored once": func(t *testing.T) {
			e := event("k1")
			first, err := service.IngestEvents(ctx, "src-retry", []analytics.Event{e})
			require.NoError(t, err)
			require.Equal(t, analytics.IngestResult{Stored: 1}, first)
			again, err := service.IngestEvents(ctx, "src-retry", []analytics.Event{e})
			require.NoError(t, err)
			require.Equal(t, analytics.IngestResult{Duplicates: 1}, again)
			require.Equal(t, 1, count(t, "src-retry"))
		},
		"the same key from another domain is another event": func(t *testing.T) {
			e := event("shared")
			_, err := service.IngestEvents(ctx, "src-a", []analytics.Event{e})
			require.NoError(t, err)
			result, err := service.IngestEvents(ctx, "src-b", []analytics.Event{e})
			require.NoError(t, err)
			require.Equal(t, 1, result.Stored)
		},
		"a batch stores the valid events and counts the rest": func(t *testing.T) {
			bad := event("bad")
			bad.EventType = "unknown"
			noUser := event("no-user")
			noUser.UserRef = "student-7"
			dup := event("dup-in-batch")
			result, err := service.IngestEvents(ctx, "src-batch", []analytics.Event{event("b1"), bad, noUser, dup, dup, event("b2")})
			require.NoError(t, err)
			require.Equal(t, analytics.IngestResult{Stored: 3, Duplicates: 1, Invalid: 2}, result)
			require.Equal(t, 3, count(t, "src-batch"))
		},
		"a future date is stored as now and properties stay small": func(t *testing.T) {
			future := event("future")
			future.OccurredAt = time.Now().Add(48 * time.Hour)
			big := event("big")
			big.Properties = map[string]any{"x": strings.Repeat("a", 3000)}
			result, err := service.IngestEvents(ctx, "src-edge", []analytics.Event{future, big})
			require.NoError(t, err)
			require.Equal(t, analytics.IngestResult{Stored: 1, Invalid: 1}, result)
			var occurred time.Time
			require.NoError(t, db.Raw(`SELECT occurred_at FROM analytics_events WHERE source_domain = ?`, "src-edge").Scan(&occurred).Error)
			require.WithinDuration(t, time.Now(), occurred, time.Minute)
		},
		"a missing source domain stores nothing": func(t *testing.T) {
			result, err := service.IngestEvents(ctx, "", []analytics.Event{event("k")})
			require.NoError(t, err)
			require.Equal(t, 1, result.Invalid)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
