package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
)

// TestManagerStart covers Start's pre-transaction validation - the only
// part of Start mockable without a real DB transaction: the lock,
// lazy-expiration materialization, idempotency claim, host/roster
// resolution, and engine execution further down this path call the shared
// sessionlock/idempotency mechanism packages and the real Game Language
// engine directly, which require a real Postgres connection to exercise
// meaningfully. That business logic is proven instead by this package's
// TestManagerStart_Integration* tests against a real disposable database.
//
// The Turn-draining logic itself (the Step-chain-bound/atomicity mechanism)
// is proven independently, without a real DB, by engineservice's own
// TestAdvanceTurn/TestStartTurn tests, since this package never
// implements any part of that mechanism itself - it only supplies
// engineservice with the durable signal log and reads back Outputs.
func TestManagerStart(t *testing.T) {
	m := &Manager{}

	_, err := m.Start(context.Background(), "session-uuid", "user-uuid", "")
	require.ErrorIs(t, err, session.ErrIdempotencyKeyRequired)
}
