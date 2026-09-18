package repo

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRepoClaimAndCompleteSessionRequest(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)
	userUUID := uuid.NewString()
	key := uuid.NewString()

	requestID, claimed, existing, err := r.ClaimSessionRequest(context.Background(), db, "TEST_OP", userUUID, key, nil, `{"a":1}`)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Nil(t, existing)
	require.NotZero(t, requestID)

	// A second claim attempt against the same identity, before completion,
	// finds the existing (still-PENDING) row rather than claiming again.
	_, claimedAgain, existingAgain, err := r.ClaimSessionRequest(context.Background(), db, "TEST_OP", userUUID, key, nil, `{"a":1}`)
	require.NoError(t, err)
	require.False(t, claimedAgain)
	require.NotNil(t, existingAgain)
	require.Equal(t, RequestStatusPending, existingAgain.Status)

	require.NoError(t, r.CompleteSessionRequest(context.Background(), db, requestID, nil, "TEST_OUTCOME", `{"result":true}`))

	_, claimedFinal, existingFinal, err := r.ClaimSessionRequest(context.Background(), db, "TEST_OP", userUUID, key, nil, `{"a":1}`)
	require.NoError(t, err)
	require.False(t, claimedFinal)
	require.NotNil(t, existingFinal)
	require.Equal(t, RequestStatusCompleted, existingFinal.Status)
	require.Equal(t, "TEST_OUTCOME", existingFinal.Outcome)
	require.NotNil(t, existingFinal.ResponsePayload)
	require.Equal(t, `{"result":true}`, *existingFinal.ResponsePayload)
}
