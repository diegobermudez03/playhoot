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
	"errors"
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

// claimRow is the physical row shape returned by the claim upsert, including
// the xmax-derived Inserted flag distinguishing a fresh claim from an
// already-existing identity (see Claim's doc comment).
type claimRow struct {
	ID              uint    `gorm:"column:id"`
	Operation       string  `gorm:"column:operation"`
	UserUUID        string  `gorm:"column:user_uuid"`
	IdempotencyKey  string  `gorm:"column:idempotency_key"`
	SessionID       *uint   `gorm:"column:session_id"`
	RequestPayload  string  `gorm:"column:request_payload"`
	Outcome         string  `gorm:"column:outcome"`
	ResponsePayload *string `gorm:"column:response_payload"`
	Status          string  `gorm:"column:status"`
	Inserted        bool    `gorm:"column:inserted"`
}

// Claim attempts to atomically own (user_uuid, operation, idempotency_key).
//
// If requestID != 0, the caller acquired a fresh claim and must later call
// Complete inside the same transaction. If existing != nil, the identity was
// already owned by a prior request, for the caller to interpret as a replay
// or a conflict. These two outcomes are mutually exclusive
// (`docs/engineering/standards/idempotency.md`'s Claim Mechanism Contract).
//
// The claim is performed as a single INSERT ... ON CONFLICT DO UPDATE
// upsert, using Postgres's xmax system column to tell an insert from a
// conflict-resolution update, rather than INSERT ... ON CONFLICT DO NOTHING
// followed by a separate SELECT for the existing row. A separate
// snapshot-bound SELECT after a DO NOTHING no-op can fail to observe a
// concurrently committed row for the remainder of a REPEATABLE READ
// transaction (the isolation level `utils.RunInDBTransaction` runs under),
// even though the row now exists - the DO UPDATE variant instead always
// returns exactly one row, computed from current data by the same statement
// that resolved the conflict, so the caller reliably observes the winning
// claim regardless of when it committed relative to this transaction's
// snapshot.
func Claim(ctx context.Context, tx *gorm.DB, input ClaimInput) (requestID uint, existing *Request, err error) {
	var row claimRow
	result := tx.WithContext(ctx).Raw(`
		INSERT INTO session_requests (operation, idempotency_key, user_uuid, session_id, request_payload, outcome, status)
		VALUES (?, ?, ?, ?, ?, '', ?)
		ON CONFLICT (user_uuid, operation, idempotency_key)
		DO UPDATE SET operation = session_requests.operation
		RETURNING
			id, operation, user_uuid, idempotency_key, session_id, request_payload, outcome, response_payload, status,
			(xmax = 0) AS inserted
	`, input.Operation, input.IdempotencyKey, input.UserUUID, input.SessionID, input.RequestPayload, StatusPending).Scan(&row)
	if result.Error != nil {
		return 0, nil, fmt.Errorf("claiming idempotency identity: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return 0, nil, errors.New("idempotency claim upsert returned no row")
	}

	if row.Inserted {
		return row.ID, nil, nil
	}
	return 0, &Request{
		ID:              row.ID,
		Operation:       row.Operation,
		UserUUID:        row.UserUUID,
		IdempotencyKey:  row.IdempotencyKey,
		SessionID:       row.SessionID,
		RequestPayload:  row.RequestPayload,
		Outcome:         row.Outcome,
		ResponsePayload: row.ResponsePayload,
		Status:          row.Status,
	}, nil
}

// Complete marks a previously claimed request row as COMPLETED with its
// resulting outcome/response payload, and records the Session it ended up
// associated with (nil if none, e.g. a rejected Create).
func Complete(ctx context.Context, tx *gorm.DB, requestID uint, sessionID *uint, outcome string, responsePayload string) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_requests
		SET status = ?, outcome = ?, response_payload = ?, session_id = ?
		WHERE id = ?
	`, StatusCompleted, outcome, responsePayload, sessionID, requestID).Error; err != nil {
		return fmt.Errorf("completing idempotency claim: %s", err)
	}
	return nil
}
