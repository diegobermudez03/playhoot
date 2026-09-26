package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
)

// TestManagerCancelSession covers CancelSession's pre-transaction
// validation - the only part mockable without a real DB transaction: the
// lock, idempotency claim, host-authorization check, and engine execution
// further down this path call the shared sessionlock/idempotency mechanism
// packages and the real Game Language engine directly, which require a
// real Postgres connection to exercise meaningfully. That business logic is
// proven instead by this package's TestManagerCancelSession_Integration*
// tests against a real disposable database.
func TestManagerCancelSession(t *testing.T) {
	m := &Manager{}

	_, err := m.CancelSession(context.Background(), "session-uuid", "user-uuid", "")
	require.ErrorIs(t, err, session.ErrIdempotencyKeyRequired)
}
