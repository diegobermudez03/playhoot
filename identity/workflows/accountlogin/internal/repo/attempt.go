package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Event names one step of a login attempt's history.
type Event string

const (
	EventStarted     Event = "started"
	EventAnswered    Event = "answered"
	EventCancelled   Event = "cancelled"
	EventFailed      Event = "failed"
	EventUnavailable Event = "unavailable"
	EventCodeIssued  Event = "code_issued"
	EventCompleted   Event = "completed"
	EventRejected    Event = "rejected"
)

// AttemptEvent is one entry of an attempt's history. Reason, UserID and the
// observed values are set only for the events they explain.
type AttemptEvent struct {
	Event         Event
	Reason        string
	UserID        int64
	ObservedEmail string
	ObservedName  string
	OccurredAt    time.Time
}

// NewAttempt is a started login.
type NewAttempt struct {
	State         string
	Provider      string
	CodeChallenge string
	Nonce         string
	StartedAt     time.Time
}

// CreateAttempt records, within tx, a started login with its first event.
func (r *Repo) CreateAttempt(ctx context.Context, tx *gorm.DB, attempt NewAttempt) error {
	var attemptID int64
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO identity_login_attempts (state, provider, code_challenge, nonce, started_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id`,
		attempt.State, attempt.Provider, attempt.CodeChallenge, attempt.Nonce, attempt.StartedAt,
	).Scan(&attemptID).Error
	if err != nil {
		return fmt.Errorf("recording login attempt: %s", err)
	}
	return r.RecordAttemptEvent(ctx, tx, attemptID, AttemptEvent{Event: EventStarted, OccurredAt: attempt.StartedAt})
}

// AnsweredAttempt is a login attempt that was just answered.
type AnsweredAttempt struct {
	ID    int64
	Nonce string
}

// AnswerAttempt marks answered within tx, at answeredAt, the attempt of
// provider with state (unique) started after startedAfter and not answered
// yet, so each attempt is answered once. Nil when there is none.
func (r *Repo) AnswerAttempt(ctx context.Context, tx *gorm.DB, provider string, state string, answeredAt time.Time, startedAfter time.Time) (*AnsweredAttempt, error) {
	var attempts []AnsweredAttempt
	err := tx.WithContext(ctx).Raw(`
		UPDATE identity_login_attempts SET answered_at = ?
		WHERE state = ? AND provider = ? AND answered_at IS NULL AND started_at > ?
		RETURNING id, nonce`,
		answeredAt, state, provider, startedAfter,
	).Scan(&attempts).Error
	if err != nil {
		return nil, fmt.Errorf("answering login attempt: %s", err)
	}
	if len(attempts) == 0 {
		return nil, nil
	}
	err = r.RecordAttemptEvent(ctx, tx, attempts[0].ID, AttemptEvent{Event: EventAnswered, OccurredAt: answeredAt})
	return &attempts[0], err
}

// IssueCode gives the answered attempt, within tx, its User and one-time
// code, recording what the provider said about the person at this login.
func (r *Repo) IssueCode(ctx context.Context, tx *gorm.DB, attemptID int64, code string, expiresAt time.Time, issued AttemptEvent) error {
	err := tx.WithContext(ctx).Exec(`
		UPDATE identity_login_attempts SET user_id = ?, login_code = ?, login_code_expires_at = ?
		WHERE id = ?`,
		issued.UserID, code, expiresAt, attemptID,
	).Error
	if err != nil {
		return fmt.Errorf("issuing login code: %s", err)
	}
	issued.Event = EventCodeIssued
	return r.RecordAttemptEvent(ctx, tx, attemptID, issued)
}

// CodeAttempt is a login attempt with a one-time code, and the User it signs
// in.
type CodeAttempt struct {
	ID            int64
	CodeChallenge string
	ExpiresAt     time.Time
	CompletedAt   *time.Time

	UserID      int64
	UserUUID    string
	DisplayName string
}

// LockAttemptByCode returns the attempt the one-time code belongs to, locked
// for the rest of tx, or nil when there is none.
func (r *Repo) LockAttemptByCode(ctx context.Context, tx *gorm.DB, code string) (*CodeAttempt, error) {
	var attempts []CodeAttempt
	err := tx.WithContext(ctx).Raw(`
		SELECT attempt.id, attempt.code_challenge, attempt.login_code_expires_at AS expires_at, attempt.completed_at,
			attempt.user_id, users.user_uuid, COALESCE(users.display_name, '') AS display_name
		FROM identity_login_attempts attempt
		JOIN identity_users users ON users.id = attempt.user_id
		WHERE attempt.login_code = ?
		FOR UPDATE OF attempt`,
		code,
	).Scan(&attempts).Error
	if err != nil {
		return nil, fmt.Errorf("locking login attempt: %s", err)
	}
	if len(attempts) == 0 {
		return nil, nil
	}
	return &attempts[0], nil
}

// CloseAttempt uses up the attempt's one-time code within tx, recording why:
// completed, or rejected with a reason.
func (r *Repo) CloseAttempt(ctx context.Context, tx *gorm.DB, attemptID int64, closed AttemptEvent) error {
	err := tx.WithContext(ctx).Exec(`
		UPDATE identity_login_attempts SET completed_at = COALESCE(completed_at, ?) WHERE id = ?`,
		closed.OccurredAt, attemptID,
	).Error
	if err != nil {
		return fmt.Errorf("closing login attempt: %s", err)
	}
	return r.RecordAttemptEvent(ctx, tx, attemptID, closed)
}

// RecordAttemptEvent appends event to the attempt's history within tx.
func (r *Repo) RecordAttemptEvent(ctx context.Context, tx *gorm.DB, attemptID int64, event AttemptEvent) error {
	err := tx.WithContext(ctx).Exec(`
		INSERT INTO identity_login_attempt_events (attempt_id, event, reason, user_id, observed_email, observed_name, occurred_at)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''), NULLIF(?, ''), ?)`,
		attemptID, string(event.Event), event.Reason, event.UserID, event.ObservedEmail, event.ObservedName, event.OccurredAt,
	).Error
	if err != nil {
		return fmt.Errorf("recording login attempt event: %s", err)
	}
	return nil
}
