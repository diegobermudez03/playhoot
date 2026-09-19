package idempotency

import (
	"context"
	"sync"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestClaimAndComplete(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	userUUID := uuid.NewString()
	key := uuid.NewString()

	requestID, existing, err := Claim(context.Background(), db, ClaimInput{
		Operation: "TEST_OP", UserUUID: userUUID, IdempotencyKey: key, RequestPayload: `{"a":1}`,
	})
	require.NoError(t, err)
	require.NotZero(t, requestID)
	require.Nil(t, existing)

	// A second claim attempt against the same identity, before completion,
	// finds the existing (still-PENDING) request rather than claiming again.
	requestIDAgain, existingAgain, err := Claim(context.Background(), db, ClaimInput{
		Operation: "TEST_OP", UserUUID: userUUID, IdempotencyKey: key, RequestPayload: `{"a":1}`,
	})
	require.NoError(t, err)
	require.Zero(t, requestIDAgain)
	require.NotNil(t, existingAgain)
	require.Equal(t, StatusPending, existingAgain.Status)

	require.NoError(t, Complete(context.Background(), db, requestID, nil, "TEST_OUTCOME", `{"result":true}`))

	requestIDFinal, existingFinal, err := Claim(context.Background(), db, ClaimInput{
		Operation: "TEST_OP", UserUUID: userUUID, IdempotencyKey: key, RequestPayload: `{"a":1}`,
	})
	require.NoError(t, err)
	require.Zero(t, requestIDFinal)
	require.NotNil(t, existingFinal)
	require.Equal(t, StatusCompleted, existingFinal.Status)
	require.Equal(t, "TEST_OUTCOME", existingFinal.Outcome)
	require.NotNil(t, existingFinal.ResponsePayload)
	require.Equal(t, `{"result":true}`, *existingFinal.ResponsePayload)
}

// TestClaim_ConcurrentSameIdentityResolvesToExactlyOneFreshClaim proves -
// against a real Postgres database - that the claim mechanism's
// non-error conflict path resolves N concurrent claims sharing the same
// identity to exactly one fresh claim, with every other attempt observing
// the existing identity (never an internal-invariant error), and each
// attempt's own transaction remaining usable afterward either way
// (`docs/engineering/standards/idempotency.md`'s Claim Mechanism Contract;
// WORK-0001's Claim Mechanism And Step-Local Interpretation).
func TestClaim_ConcurrentSameIdentityResolvesToExactlyOneFreshClaim(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	userUUID := uuid.NewString()
	key := uuid.NewString()

	const attempts = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	requestIDs := make([]uint, attempts)
	existings := make([]*Request, attempts)
	errs := make([]error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start

			tx := db.Begin()
			defer tx.Rollback()

			requestID, existing, err := Claim(context.Background(), tx, ClaimInput{
				Operation: "CONCURRENT_OP", UserUUID: userUUID, IdempotencyKey: key, RequestPayload: `{"a":1}`,
			})
			requestIDs[i], existings[i], errs[i] = requestID, existing, err
			if err != nil {
				return
			}

			// The transaction must remain fully usable afterward either
			// way - a losing attempt must not have been left aborted by
			// the conflict.
			errs[i] = tx.Exec(`SELECT 1`).Error
			if errs[i] != nil {
				return
			}
			errs[i] = tx.Commit().Error
		}(i)
	}
	close(start)
	wg.Wait()

	freshCount := 0
	for i := 0; i < attempts; i++ {
		require.NoError(t, errs[i], "attempt %d", i)
		if requestIDs[i] != 0 {
			freshCount++
			require.Nil(t, existings[i], "attempt %d", i)
		} else {
			require.NotNil(t, existings[i], "attempt %d must observe the existing identity", i)
		}
	}
	require.Equal(t, 1, freshCount, "exactly one concurrent claim attempt must win the fresh claim")

	var count int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_requests WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
	`, userUUID, "CONCURRENT_OP", key).Scan(&count).Error)
	require.Equal(t, int64(1), count)
}
