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
	require.JSONEq(t, `{"result":true}`, *existingFinal.ResponsePayload, "Postgres's JSONB storage reformats stored JSON text, so compare semantically rather than byte-for-byte")
}

// TestClaim_ConcurrentSameIdentityResolvesToExactlyOneFreshClaim proves -
// against a real Postgres database - that of N concurrent claims sharing the
// same identity, exactly one wins a fresh claim; Claim does not itself
// resolve the race between its own fetch and its own insert, so every other
// attempt either observes the existing identity (if its fetch ran after the
// winner committed) or fails with a unique-constraint-violation error (if it
// raced the winner's still-open insert) - callers with no other
// serialization of their own (unlike Join/Leave, which lock the owning
// Session's row before ever reaching Claim) must expect and handle this.
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
			errs[i] = tx.Commit().Error
		}(i)
	}
	close(start)
	wg.Wait()

	freshCount := 0
	for i := 0; i < attempts; i++ {
		switch {
		case requestIDs[i] != 0:
			freshCount++
			require.NoError(t, errs[i], "the winning fresh claim must not error, attempt %d", i)
			require.Nil(t, existings[i], "attempt %d", i)
		case errs[i] == nil:
			require.NotNil(t, existings[i], "a losing attempt with no error must observe the existing identity, attempt %d", i)
		}
		// A losing attempt that instead returned an error is expected: it
		// raced the winner's still-open insert (see the test's doc comment).
	}
	require.Equal(t, 1, freshCount, "exactly one concurrent claim attempt must win the fresh claim")

	var count int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM session_requests WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
	`, userUUID, "CONCURRENT_OP", key).Scan(&count).Error)
	require.Equal(t, int64(1), count)
}
