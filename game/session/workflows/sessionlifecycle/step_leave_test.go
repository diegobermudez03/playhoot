package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
)

// TestManagerLeave covers Leave's pre-transaction validation - the only part
// of Leave mockable without a real DB transaction: the lock, lazy-expiration
// materialization, idempotency claim, and actor/participant lookup further
// down this path call the shared sessionlock/idempotency mechanism packages
// directly, which require a real Postgres connection to run their SQL. That
// business logic (replay/conflict, NotInLobby, ActorNotFound, deactivation)
// is proven instead by this package's TestManagerLeave_Integration tests
// against a real disposable database.
func TestManagerLeave(t *testing.T) {
	m := &Manager{}

	_, err := m.Leave(context.Background(), "session-uuid", "user-uuid", "")
	require.ErrorIs(t, err, session.ErrIdempotencyKeyRequired)
}
