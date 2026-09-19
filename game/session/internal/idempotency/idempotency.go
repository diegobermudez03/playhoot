// Package idempotency implements the shared session_requests claim
// mechanics reused by the Session lifecycle workflow's Create/Join/Leave
// steps: atomically claiming a (user_uuid, operation, idempotency_key)
// identity before any mutating effect happens, and completing a claim with
// its logical outcome. It owns no per-operation business policy
// (meaningful-field comparison, conflict vs. replay interpretation): that
// stays with each step, which understands its own request/response payload
// shapes.
package idempotency

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Status values for a session_requests row.
const (
	StatusPending   = "PENDING"
	StatusCompleted = "COMPLETED"
)

// Request is a persisted session_requests record, as seen by the workflow
// code that claims/replays it.
type Request struct {
	ID              uint
	Operation       string
	UserUUID        string
	IdempotencyKey  string
	SessionID       *uint
	RequestPayload  string
	Outcome         string
	ResponsePayload *string
	Status          string
}

// ClaimInput describes the identity and request being claimed.
type ClaimInput struct {
	Operation      string
	UserUUID       string
	IdempotencyKey string
	SessionID      *uint
	RequestPayload string
}

// fetchExisting returns the session_requests row already owning input's
// (user_uuid, operation, idempotency_key) identity, or nil if none exists
// yet.
func fetchExisting(ctx context.Context, tx *gorm.DB, input ClaimInput) (*Request, error) {
	var row Request
	result := tx.WithContext(ctx).Raw(`
		SELECT id, operation, user_uuid, idempotency_key, session_id, request_payload, outcome, response_payload, status
		FROM session_requests
		WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
	`, input.UserUUID, input.Operation, input.IdempotencyKey).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("fetching idempotency request: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// Claim attempts to atomically own (user_uuid, operation, idempotency_key).
//
// If requestID != 0, the caller acquired a fresh claim and must later call
// Complete inside the same transaction. If existing != nil, the identity was
// already owned by a prior request, for the caller to interpret as a replay
// or a conflict. These two outcomes are mutually exclusive
// (`docs/engineering/standards/idempotency.md`'s Claim Mechanism Contract).
//
// Claim first fetches by identity; only if nothing is found does it insert a
// fresh row. Claim does not itself resolve a race between its own fetch and
// its own insert: it relies on its callers to already serialize concurrent
// claims for the same identity before ever reaching Claim - Join/Leave do
// this by locking the owning Session's row first. A caller with no such
// serialization (Create has no pre-existing Session row to lock) can, under
// real concurrent load, have its insert lose a race against another
// concurrent claim of the same identity; that surfaces as an ordinary
// unique-constraint-violation error from the INSERT below, not a graceful
// replay.
func Claim(ctx context.Context, tx *gorm.DB, input ClaimInput) (requestID uint, existing *Request, err error) {
	existingRequest, err := fetchExisting(ctx, tx, input)
	if err != nil {
		return 0, nil, err
	}
	if existingRequest != nil {
		return 0, existingRequest, nil
	}

	var row Request
	result := tx.WithContext(ctx).Raw(`
		INSERT INTO session_requests (operation, idempotency_key, user_uuid, session_id, request_payload, outcome, status)
		VALUES (?, ?, ?, ?, ?, '', ?)
		RETURNING id, operation, user_uuid, idempotency_key, session_id, request_payload, outcome, response_payload, status
	`, input.Operation, input.IdempotencyKey, input.UserUUID, input.SessionID, input.RequestPayload, StatusPending).Scan(&row)
	if result.Error != nil {
		return 0, nil, fmt.Errorf("claiming idempotency identity: %s", result.Error)
	}
	return row.ID, nil, nil
}

// Complete marks a previously claimed request row as COMPLETED with its
// resulting outcome/response payload, and records the Session it ended up
// associated with (nil if none, e.g. a rejected Create). responsePayload is
// empty for a decline outcome that has nothing to replay (e.g. AlreadyJoined,
// LobbyFull, ActorNotFound) - response_payload is a nullable JSONB column, so
// an empty Go string (not valid JSON) is stored as SQL NULL rather than bound
// literally, which Postgres would otherwise reject.
func Complete(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error {
	var responsePayloadArg any
	if responsePayload != "" {
		responsePayloadArg = responsePayload
	}
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_requests
		SET status = ?, outcome = ?, response_payload = ?, session_id = ?
		WHERE id = ?
	`, StatusCompleted, outcome, responsePayloadArg, sessionID, requestID).Error; err != nil {
		return fmt.Errorf("completing idempotency claim: %s", err)
	}
	return nil
}
