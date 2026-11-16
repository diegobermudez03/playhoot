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

// Intent is what a login attempt is for.
type Intent string

const (
	// IntentSignIn opens a session for whoever the provider vouches for.
	IntentSignIn Intent = "sign-in"
	// IntentLink adds the provider identity to an account already signed in.
	IntentLink Intent = "link"
)

// Reasons an issued code carries for a link attempt.
const (
	ReasonLinked      = "linked"
	ReasonMethodInUse = "method_in_use"
)

// NewAttempt is a started login. GuestUserUUID, when set, is the guest who
// started it; LinkUserUUID, when set, is the account the provider identity
// will be added to.
type NewAttempt struct {
	State         string
	Provider      string
	Intent        Intent
	CodeChallenge string
	Nonce         string
	// ProviderCodeVerifier is the PKCE verifier toward the provider.
	ProviderCodeVerifier string
	GuestUserUUID        string
	LinkUserUUID         string
	StartedAt            time.Time
}

// CreateAttempt records, within tx, a started login with its first event. A
// guest that no longer exists is recorded as no guest.
func (r *Repo) CreateAttempt(ctx context.Context, tx *gorm.DB, attempt NewAttempt) error {
	created := loginAttempt{
		State:                attempt.State,
		Provider:             attempt.Provider,
		Intent:               string(attempt.Intent),
		CodeChallenge:        attempt.CodeChallenge,
		Nonce:                attempt.Nonce,
		ProviderCodeVerifier: attempt.ProviderCodeVerifier,
		StartedAt:            attempt.StartedAt,
	}
	if attempt.GuestUserUUID != "" {
		guestID, err := userIDOf(ctx, tx, attempt.GuestUserUUID)
		if err != nil {
			return err
		}
		created.GuestUserID = optional(guestID)
	}
	if attempt.LinkUserUUID != "" {
		linkID, err := userIDOf(ctx, tx, attempt.LinkUserUUID)
		if err != nil {
			return err
		}
		created.LinkUserID = optional(linkID)
	}
	if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
		return fmt.Errorf("recording login attempt: %s", err)
	}
	return r.RecordAttemptEvent(ctx, tx, created.ID, AttemptEvent{Event: EventStarted, OccurredAt: attempt.StartedAt})
}

// AnsweredAttempt is a login attempt that was just answered, with what it
// is for and the guest or account that started it, if any.
type AnsweredAttempt struct {
	ID                   int64
	Intent               string
	Nonce                string
	ProviderCodeVerifier string
	GuestUserID          *int64
	LinkUserID           *int64
}

// userIDOf returns the internal identifier of userUUID within tx, or zero
// when no User has it.
func userIDOf(ctx context.Context, tx *gorm.DB, userUUID string) (int64, error) {
	var ids []int64
	if err := tx.WithContext(ctx).Raw(`SELECT id FROM identity_users WHERE user_uuid = ?`, userUUID).Scan(&ids).Error; err != nil {
		return 0, fmt.Errorf("finding the user of a login attempt: %s", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

// AnswerAttempt marks answered within tx, at answeredAt, the attempt of
// provider with state (unique) started after startedAfter and not answered
// yet, so each attempt is answered once. Nil when there is none.
func (r *Repo) AnswerAttempt(ctx context.Context, tx *gorm.DB, provider string, state string, answeredAt time.Time, startedAfter time.Time) (*AnsweredAttempt, error) {
	var attempts []AnsweredAttempt
	err := tx.WithContext(ctx).Raw(`
		UPDATE identity_login_attempts SET answered_at = ?
		WHERE state = ? AND provider = ? AND answered_at IS NULL AND started_at > ?
		RETURNING id, intent, nonce, provider_code_verifier, guest_user_id, link_user_id`,
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
	Intent        string
	CodeChallenge string
	ExpiresAt     time.Time
	CompletedAt   *time.Time

	// IssuedReason is what the callback decided for a link attempt: linked,
	// or the identity belongs to someone else.
	IssuedReason string

	UserID      int64
	UserUUID    string
	DisplayName string
}

// LockAttemptByCode returns the attempt the one-time code belongs to, locked
// for the rest of tx, or nil when there is none.
func (r *Repo) LockAttemptByCode(ctx context.Context, tx *gorm.DB, code string) (*CodeAttempt, error) {
	var attempts []CodeAttempt
	err := tx.WithContext(ctx).Raw(`
		SELECT attempt.id, attempt.intent, attempt.code_challenge, attempt.login_code_expires_at AS expires_at, attempt.completed_at,
			attempt.user_id, users.user_uuid, COALESCE(users.display_name, '') AS display_name,
			COALESCE((SELECT event.reason FROM identity_login_attempt_events event
				WHERE event.attempt_id = attempt.id AND event.event = ? ORDER BY event.id DESC LIMIT 1), '') AS issued_reason
		FROM identity_login_attempts attempt
		JOIN identity_users users ON users.id = attempt.user_id
		WHERE attempt.login_code = ?
		FOR UPDATE OF attempt`,
		string(EventCodeIssued), code,
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
	recorded := loginAttemptEvent{
		AttemptID:     attemptID,
		Event:         string(event.Event),
		Reason:        optional(event.Reason),
		UserID:        optional(event.UserID),
		ObservedEmail: optional(event.ObservedEmail),
		ObservedName:  optional(event.ObservedName),
		OccurredAt:    event.OccurredAt,
	}
	if err := tx.WithContext(ctx).Create(&recorded).Error; err != nil {
		return fmt.Errorf("recording login attempt event: %s", err)
	}
	return nil
}
