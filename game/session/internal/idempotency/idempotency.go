// Package idempotency implements the shared session_requests claim
// mechanics reused by CreateSession/JoinSession/LeaveSession: atomically
// claiming a (user_uuid, operation, idempotency_key) identity before any
// mutating effect happens, and completing a claim with its logical outcome.
// It owns no per-operation business policy (meaningful-field comparison,
// conflict vs. replay interpretation): that stays with each use case, which
// understands its own request/response payload shapes.
package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/session/internal/pgerrs"
	"gorm.io/gorm"
)

// Status values for a session_requests row.
const (
	StatusPending   = "PENDING"
	StatusCompleted = "COMPLETED"
)

// Row is a persisted session_requests record.
type Row struct {
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

type claimInsert struct {
	ID             uint   `gorm:"column:id"`
	Operation      string `gorm:"column:operation"`
	IdempotencyKey string `gorm:"column:idempotency_key"`
	UserUUID       string `gorm:"column:user_uuid"`
	SessionID      *uint  `gorm:"column:session_id"`
	RequestPayload string `gorm:"column:request_payload"`
	Outcome        string `gorm:"column:outcome"`
	Status         string `gorm:"column:status"`
}

func (claimInsert) TableName() string { return "session_requests" }

// Claim attempts to atomically own (user_uuid, operation, idempotency_key)
// by inserting a PENDING session_requests row.
//
// If claimed is true, requestID identifies the freshly inserted row; the
// caller owns it and must later call Complete inside the same transaction.
// If claimed is false, existing holds the already-owned row found via the
// same unique identity, for the caller to interpret as a replay or a
// conflict.
func Claim(ctx context.Context, tx *gorm.DB, input ClaimInput) (requestID uint, claimed bool, existing *Row, err error) {
	row := claimInsert{
		Operation:      input.Operation,
		IdempotencyKey: input.IdempotencyKey,
		UserUUID:       input.UserUUID,
		SessionID:      input.SessionID,
		RequestPayload: input.RequestPayload,
		Outcome:        "",
		Status:         StatusPending,
	}

	if createErr := tx.WithContext(ctx).Create(&row).Error; createErr != nil {
		if !pgerrs.IsUniqueViolation(createErr) {
			return 0, false, nil, fmt.Errorf("inserting idempotency claim: %s", createErr)
		}

		existingRow, findErr := find(ctx, tx, input.Operation, input.UserUUID, input.IdempotencyKey)
		if findErr != nil {
			return 0, false, nil, findErr
		}
		if existingRow == nil {
			return 0, false, nil, errors.New("idempotency claim conflicted but no existing row was found")
		}
		return 0, false, existingRow, nil
	}

	return row.ID, true, nil, nil
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

func find(ctx context.Context, tx *gorm.DB, operation, userUUID, idempotencyKey string) (*Row, error) {
	var row Row
	result := tx.WithContext(ctx).Raw(`
		SELECT id, operation, user_uuid, idempotency_key, session_id, request_payload, outcome, response_payload, status
		FROM session_requests
		WHERE user_uuid = ? AND operation = ? AND idempotency_key = ?
	`, userUUID, operation, idempotencyKey).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("reading existing idempotency claim: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}
