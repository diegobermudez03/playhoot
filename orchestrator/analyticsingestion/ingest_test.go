package analyticsingestion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/analytics"
)

type fakeIngester struct {
	result analytics.IngestResult
	err    error
	source string
	got    []analytics.Event
}

func (f *fakeIngester) IngestEvents(_ context.Context, source string, events []analytics.Event) (analytics.IngestResult, error) {
	f.source, f.got = source, events
	return f.result, f.err
}

func TestDeliver(t *testing.T) {
	ctx := context.Background()
	one := json.RawMessage(`{"event_type":"game-created","user_ref":"u","account_kind":"personal","plan_kind":"free","entity_ref":"g","properties":{"a":1},"idempotency_key":"k"}`)
	batch := json.RawMessage(`[` + string(one) + `,` + string(one) + `]`)

	tests := map[string]func(t *testing.T){
		"the queue's domain is the source": func(t *testing.T) {
			f := &fakeIngester{}
			require.NoError(t, New(f).Deliver(ctx, "game", one))
			require.Equal(t, "game", f.source)
			require.Len(t, f.got, 1)
			require.Equal(t, analytics.EventGameCreated, f.got[0].EventType)
		},
		"a task may carry a batch": func(t *testing.T) {
			f := &fakeIngester{}
			require.NoError(t, New(f).Deliver(ctx, "game", batch))
			require.Len(t, f.got, 2)
		},
		"duplicates are delivered": func(t *testing.T) {
			require.NoError(t, New(&fakeIngester{result: analytics.IngestResult{Duplicates: 1}}).Deliver(ctx, "game", one))
		},
		"an unreadable or invalid event fails permanently": func(t *testing.T) {
			err := New(&fakeIngester{}).Deliver(ctx, "game", json.RawMessage(`{`))
			require.ErrorContains(t, err, "reading analytics event")
			err = New(&fakeIngester{result: analytics.IngestResult{Stored: 1, Invalid: 1}}).Deliver(ctx, "game", batch)
			require.ErrorContains(t, err, "invalid analytics events")
		},
		"a storage failure retries": func(t *testing.T) {
			boom := errors.New("down")
			require.ErrorIs(t, New(&fakeIngester{err: boom}).Deliver(ctx, "game", one), boom)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
