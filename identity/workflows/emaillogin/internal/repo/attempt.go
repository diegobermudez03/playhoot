package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/internal/business/emailcode"
)

// Intent is what an attempt's code is for.
type Intent string

const (
	// IntentSignIn signs the person in, registering them if the address is
	// new.
	IntentSignIn Intent = "sign-in"
	// IntentLink adds the address as another way into an account that is
	// already signed in.
	IntentLink Intent = "link"
)

// Event names one step of an attempt's history.
type Event string

const (
	EventSent         Event = "sent"
	EventWrongCode    Event = "wrong_code"
	EventSuperseded   Event = "superseded"
	EventExhausted    Event = "exhausted"
	EventCompleted    Event = "completed"
	EventRefusedInUse Event = "refused_in_use"
)

// NewAttempt is a code just asked for. GuestUserUUID, when set, is the guest
// to upgrade; LinkUserUUID, when set, is the account adding the address.
type NewAttempt struct {
	AttemptUUID   string
	Email         string
	Intent        Intent
	CodeHash      string
	Language      string
	GuestUserUUID string
	LinkUserUUID  string
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

// SendsSince counts, within tx, the codes sent to email since each of the
// two moments bounding how often one address may be asked for one.
func (r *Repo) SendsSince(ctx context.Context, tx *gorm.DB, email string, burstSince time.Time, daySince time.Time) (burst int, day int, err error) {
	var counts []struct {
		Burst int
		Day   int
	}
	err = tx.WithContext(ctx).Raw(`
		SELECT count(*) FILTER (WHERE created_at > ?) AS burst, count(*) FILTER (WHERE created_at > ?) AS day
		FROM identity_email_codes WHERE email = ?`,
		burstSince, daySince, email,
	).Scan(&counts).Error
	if err != nil {
		return 0, 0, fmt.Errorf("counting codes sent to an address: %s", err)
	}
	if len(counts) == 0 {
		return 0, 0, nil
	}
	return counts[0].Burst, counts[0].Day, nil
}

// SupersedeLive marks superseded, within tx, every attempt of email still
// waiting for its code, so one address has one live attempt.
func (r *Repo) SupersedeLive(ctx context.Context, tx *gorm.DB, email string, at time.Time) error {
	var ids []int64
	err := tx.WithContext(ctx).Raw(`
		UPDATE identity_email_codes SET superseded_at = ?
		WHERE email = ? AND completed_at IS NULL AND superseded_at IS NULL
		RETURNING id`,
		at, email,
	).Scan(&ids).Error
	if err != nil {
		return fmt.Errorf("superseding live email codes: %s", err)
	}
	for _, id := range ids {
		if err := r.RecordEvent(ctx, tx, id, EventSuperseded, at); err != nil {
			return err
		}
	}
	return nil
}

// CreateAttempt records, within tx, a sent code with its first event. A
// guest that no longer exists is recorded as no guest.
func (r *Repo) CreateAttempt(ctx context.Context, tx *gorm.DB, attempt NewAttempt) error {
	created := emailCode{
		AttemptUUID: attempt.AttemptUUID,
		Email:       attempt.Email,
		Intent:      string(attempt.Intent),
		CodeSha256:  attempt.CodeHash,
		Language:    attempt.Language,
		ExpiresAt:   attempt.ExpiresAt,
		CreatedAt:   attempt.CreatedAt,
	}
	if attempt.GuestUserUUID != "" {
		guestID, err := r.UserIDOf(ctx, tx, attempt.GuestUserUUID)
		if err != nil {
			return err
		}
		created.GuestUserID = optional(guestID)
	}
	if attempt.LinkUserUUID != "" {
		linkID, err := r.UserIDOf(ctx, tx, attempt.LinkUserUUID)
		if err != nil {
			return err
		}
		created.LinkUserID = optional(linkID)
	}
	if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
		return fmt.Errorf("recording email code attempt: %s", err)
	}
	return r.RecordEvent(ctx, tx, created.ID, EventSent, attempt.CreatedAt)
}

// LiveAttempt is the attempt an address is currently waiting on.
type LiveAttempt struct {
	ID          int64
	AttemptUUID string
	Email       string
	Intent      string
	CodeSha256  string `gorm:"column:code_sha256"`
	GuestUserID *int64
	LinkUserID  *int64
	WrongCodes  int
}

// LockLiveAttempt returns, within tx, the attempt of email that is still
// waiting for its code at now, locked for the rest of tx: not completed, not
// superseded, not expired and not exhausted by wrong codes. Nil when there
// is none.
func (r *Repo) LockLiveAttempt(ctx context.Context, tx *gorm.DB, email string, now time.Time) (*LiveAttempt, error) {
	var attempts []LiveAttempt
	err := tx.WithContext(ctx).Raw(`
		SELECT code.id, code.attempt_uuid, code.email, code.intent, code.code_sha256, code.guest_user_id, code.link_user_id,
			(SELECT count(*) FROM identity_email_code_events event WHERE event.code_id = code.id AND event.event = ?) AS wrong_codes
		FROM identity_email_codes code
		WHERE code.email = ? AND code.completed_at IS NULL AND code.superseded_at IS NULL AND code.expires_at > ?
		ORDER BY code.id DESC LIMIT 1
		FOR UPDATE OF code`,
		string(EventWrongCode), email, now,
	).Scan(&attempts).Error
	if err != nil {
		return nil, fmt.Errorf("locking email code attempt: %s", err)
	}
	if len(attempts) == 0 || attempts[0].WrongCodes >= emailcode.MaxWrongCodes {
		return nil, nil
	}
	return &attempts[0], nil
}

// ReadAttemptContact returns the address and language of the attempt
// attemptUUID, so the code's email can be addressed. found is false when no
// attempt has that identifier.
func (r *Repo) ReadAttemptContact(ctx context.Context, db *gorm.DB, attemptUUID string) (email string, language string, found bool, err error) {
	var rows []struct {
		Email    string
		Language string
	}
	err = db.WithContext(ctx).Raw(`SELECT email, language FROM identity_email_codes WHERE attempt_uuid = ?`, attemptUUID).
		Scan(&rows).Error
	if err != nil {
		return "", "", false, fmt.Errorf("reading an email code attempt: %s", err)
	}
	if len(rows) == 0 {
		return "", "", false, nil
	}
	return rows[0].Email, rows[0].Language, true, nil
}

// CloseAttempt uses the attempt up within tx, recording why.
func (r *Repo) CloseAttempt(ctx context.Context, tx *gorm.DB, attemptID int64, event Event, at time.Time) error {
	err := tx.WithContext(ctx).Exec(`
		UPDATE identity_email_codes SET completed_at = COALESCE(completed_at, ?) WHERE id = ?`,
		at, attemptID,
	).Error
	if err != nil {
		return fmt.Errorf("closing email code attempt: %s", err)
	}
	return r.RecordEvent(ctx, tx, attemptID, event, at)
}

// RecordEvent appends event to the attempt's history within tx.
func (r *Repo) RecordEvent(ctx context.Context, tx *gorm.DB, attemptID int64, event Event, at time.Time) error {
	recorded := emailCodeEvent{CodeID: attemptID, Event: string(event), CreatedAt: at}
	if err := tx.WithContext(ctx).Create(&recorded).Error; err != nil {
		return fmt.Errorf("recording email code event: %s", err)
	}
	return nil
}

// UserIDOf returns the internal identifier of userUUID within tx, or zero
// when no User has it.
func (r *Repo) UserIDOf(ctx context.Context, tx *gorm.DB, userUUID string) (int64, error) {
	var ids []int64
	err := tx.WithContext(ctx).Raw(`SELECT id FROM identity_users WHERE user_uuid = ?`, userUUID).Scan(&ids).Error
	if err != nil {
		return 0, fmt.Errorf("finding a user: %s", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}
