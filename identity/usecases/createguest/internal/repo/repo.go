// Package repo persists guest creation: the idempotency claim, the guest User
// and their first session.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	request := guestRequest{
		IdempotencyKey: guest.IdempotencyKey,
		DisplayName:    guest.DisplayName,
		UserUUID:       guest.UserUUID,
		SessionUUID:    guest.SessionUUID,
		RequestedAt:    guest.RequestedAt,
	}
	claimed := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&request)
	if claimed.Error != nil {
		return 0, nil, fmt.Errorf("claiming guest request: %s", claimed.Error)
	}
	if claimed.RowsAffected > 0 {
		return request.ID, nil, nil
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
	created := user{UserUUID: guest.UserUUID, DisplayName: guest.DisplayName}
	if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
		return fmt.Errorf("creating guest user: %s", err)
	}
	named := displayNameChange{UserID: created.ID, ToName: guest.DisplayName, ChangedAt: guest.RequestedAt}
	if err := tx.WithContext(ctx).Create(&named).Error; err != nil {
		return fmt.Errorf("recording guest display name: %s", err)
	}
	opened := session{
		SessionUUID:  guest.SessionUUID,
		UserID:       created.ID,
		RefreshToken: guest.RefreshToken,
		IssuedAt:     guest.RequestedAt,
		StartedAt:    guest.RequestedAt,
	}
	if err := tx.WithContext(ctx).Create(&opened).Error; err != nil {
		return fmt.Errorf("creating guest session: %s", err)
	}
	return nil
}

type guestRequest struct {
	ID             int64
	IdempotencyKey string
	DisplayName    string
	UserUUID       string
	SessionUUID    string
	RequestedAt    time.Time
}

func (guestRequest) TableName() string { return "identity_guest_requests" }

type user struct {
	ID          int64
	UserUUID    string
	DisplayName string
}

func (user) TableName() string { return "identity_users" }

type displayNameChange struct {
	ID        int64
	UserID    int64
	FromName  *string
	ToName    string
	ChangedAt time.Time
}

func (displayNameChange) TableName() string { return "identity_display_name_changes" }

type session struct {
	ID           int64
	SessionUUID  string
	UserID       int64
	RefreshToken string
	IssuedAt     time.Time
	StartedAt    time.Time
}

func (session) TableName() string { return "identity_sessions" }

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
