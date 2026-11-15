package spendingestion

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
)

type fakeRecorder struct {
	outcome budget.RecordSpendOutcome
	err     error
	got     budget.SpendEvent
}

func (f *fakeRecorder) RecordSpend(_ context.Context, event budget.SpendEvent) (budget.RecordSpendOutcome, error) {
	f.got = event
	return f.outcome, f.err
}

func TestDeliver(t *testing.T) {
	ctx := context.Background()
	payload, err := json.Marshal(Payload{Category: "email", Service: "ses-outbound", Quantity: 1, Unit: "emails", Measured: true, IdempotencyKey: "k"})
	require.NoError(t, err)

	tests := map[string]func(t *testing.T){
		"the queue's domain is the source": func(t *testing.T) {
			f := &fakeRecorder{outcome: budget.SpendRecorded}
			require.NoError(t, New(f).Deliver(ctx, "notifications", payload))
			require.Equal(t, "notifications", f.got.SourceDomain)
			require.Equal(t, budget.CategoryEmail, f.got.Category)
		},
		"a duplicate is delivered": func(t *testing.T) {
			require.NoError(t, New(&fakeRecorder{outcome: budget.SpendDuplicate}).Deliver(ctx, "x", payload))
		},
		"an unreadable or invalid event fails permanently": func(t *testing.T) {
			err := New(&fakeRecorder{}).Deliver(ctx, "x", json.RawMessage(`{`))
			require.Error(t, err)
			require.Contains(t, err.Error(), "reading spend event")
			err = New(&fakeRecorder{outcome: budget.SpendInvalid}).Deliver(ctx, "x", payload)
			require.Error(t, err)
		},
		"a storage failure retries": func(t *testing.T) {
			boom := errors.New("down")
			require.ErrorIs(t, New(&fakeRecorder{err: boom}).Deliver(ctx, "x", payload), boom)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
