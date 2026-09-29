package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/stretchr/testify/require"
)

// TestManagerSubmitPlayerEvent covers SubmitPlayerEvent's pre-transaction
// validation - the only part mockable without a real DB transaction: the
// lock, idempotency claim, actor resolution, and Executor call further down
// this path call the shared sessionlock/idempotency mechanism packages and
// the injected executor.Executor directly, which require a real Postgres
// connection to exercise meaningfully. That business logic is proven
// instead by this package's TestManagerSubmitPlayerEvent_Integration* tests
// against a real disposable database.
func TestManagerSubmitPlayerEvent(t *testing.T) {
	m := &Manager{}

	_, err := m.SubmitPlayerEvent(context.Background(), "session-uuid", "user-uuid", "Guess", nil, "")
	require.ErrorIs(t, err, session.ErrIdempotencyKeyRequired)
}
