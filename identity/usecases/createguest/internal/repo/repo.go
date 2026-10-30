// Package repo persists guest creation: the idempotency claim, the guest User
// and their first session.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is guest creation's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB returns the database, for transactions.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// NewGuest is a guest User to create with their first session, for one
// idempotency key.
type NewGuest struct {
	IdempotencyKey string
	UserUUID       string
	DisplayName    string
	SessionUUID    string
	RefreshToken   string
	RequestedAt    time.Time
}

// ClaimGuestRequest claims guest.IdempotencyKey within tx. It returns the new
// request's ID, or the request that claimed the key before (ID 0).
func (r *Repo) ClaimGuestRequest(ctx context.Context, tx *gorm.DB, guest NewGuest) (int64, *GuestRequest, error) {
	var claimed []int64
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO identity_guest_requests (idempotency_key, display_name, user_uuid, session_uuid, requested_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`,
		guest.IdempotencyKey, guest.DisplayName, guest.UserUUID, guest.SessionUUID, guest.RequestedAt,
	).Scan(&claimed).Error
	if err != nil {
		return 0, nil, fmt.Errorf("claiming guest request: %s", err)
	}
	if len(claimed) > 0 {
		return claimed[0], nil, nil
	}
	existing, err := findGuestRequest(ctx, tx, guest.IdempotencyKey)
	if err != nil {
		return 0, nil, err
	}
	if existing == nil {
		return 0, nil, fmt.Errorf("guest request %s claimed but not found", guest.IdempotencyKey)
	}
	return 0, existing, nil
}

// CreateGuestUser creates, within tx, the guest User, the display name they
// start with in its history, and their first, never expiring session.
func (r *Repo) CreateGuestUser(ctx context.Context, tx *gorm.DB, guest NewGuest) error {
	var userID int64
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`,
		guest.UserUUID, guest.DisplayName,
	).Scan(&userID).Error
	if err != nil {
		return fmt.Errorf("creating guest user: %s", err)
	}
	err = tx.WithContext(ctx).Exec(`
		INSERT INTO identity_display_name_changes (user_id, from_name, to_name, changed_at)
		VALUES (?, NULL, ?, ?)`,
		userID, guest.DisplayName, guest.RequestedAt,
	).Error
	if err != nil {
		return fmt.Errorf("recording guest display name: %s", err)
	}
	err = tx.WithContext(ctx).Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at)
		VALUES (?, ?, ?, ?, ?)`,
		guest.SessionUUID, userID, guest.RefreshToken, guest.RequestedAt, guest.RequestedAt,
	).Error
	if err != nil {
		return fmt.Errorf("creating guest session: %s", err)
	}
	return nil
}

// GuestRequest is a claimed guest creation and where its session stands.
type GuestRequest struct {
	DisplayName string
	UserUUID    string
	RequestedAt time.Time

	// ActiveRefreshToken is the session's active refresh token, nil once the
	// session ended (signed out).
	ActiveRefreshToken *string
}

// findGuestRequest returns the guest creation claimed with idempotencyKey,
// or nil when there is none.
func findGuestRequest(ctx context.Context, db *gorm.DB, idempotencyKey string) (*GuestRequest, error) {
	var requests []GuestRequest
	err := db.WithContext(ctx).Raw(`
		SELECT request.display_name, request.user_uuid, request.requested_at,
			session.refresh_token AS active_refresh_token
		FROM identity_guest_requests request
		LEFT JOIN identity_sessions session
			ON session.session_uuid = request.session_uuid AND session.deactivated_at IS NULL
		WHERE request.idempotency_key = ?`,
		idempotencyKey,
	).Scan(&requests).Error
	if err != nil {
		return nil, fmt.Errorf("finding guest request: %s", err)
	}
	if len(requests) == 0 {
		return nil, nil
	}
	return &requests[0], nil
}
