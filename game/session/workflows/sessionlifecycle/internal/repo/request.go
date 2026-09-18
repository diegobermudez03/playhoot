package repo

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/session/internal/idempotency"
	"gorm.io/gorm"
)

// RequestRow is a persisted session_requests record - a thin alias over
// idempotency.Row, the shared claim/replay mechanism's own shape.
type RequestRow = idempotency.Row

// Idempotency status values (thin aliases so consumers of this package do
// not need to import idempotency directly).
const (
	RequestStatusPending   = idempotency.StatusPending
	RequestStatusCompleted = idempotency.StatusCompleted
)

// ClaimSessionRequest attempts to atomically own (userUUID, operation,
// idempotencyKey) by inserting a PENDING session_requests row, via the
// shared idempotency claim mechanism. It stores the claim/request row and
// payload; it does not decide what a replay/conflict/new-command means -
// that is Manager policy (`docs/engineering/standards/idempotency.md`'s
// Idempotency Policy Ownership).
func (r *Repo) ClaimSessionRequest(ctx context.Context, tx *gorm.DB, operation, userUUID, idempotencyKey string, sessionID *uint, requestPayload string) (requestID uint, claimed bool, existing *RequestRow, err error) {
	requestID, claimed, existing, err = idempotency.Claim(ctx, tx, idempotency.ClaimInput{
		Operation:      operation,
		UserUUID:       userUUID,
		IdempotencyKey: idempotencyKey,
		SessionID:      sessionID,
		RequestPayload: requestPayload,
	})
	if err != nil {
		return 0, false, nil, fmt.Errorf("claiming session request: %s", err)
	}
	return requestID, claimed, existing, nil
}

// CompleteSessionRequest marks a previously claimed request row as COMPLETED
// with its resulting outcome/response payload.
func (r *Repo) CompleteSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error {
	return idempotency.Complete(ctx, tx, requestID, sessionID, outcome, responsePayload)
}
