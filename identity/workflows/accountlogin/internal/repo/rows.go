package repo

import "time"

// The rows this repository inserts, one struct per table.

type loginAttempt struct {
	ID            int64
	State         string
	Provider      string
	CodeChallenge string
	Nonce         string
	GuestUserID   *int64
	StartedAt     time.Time
}

func (loginAttempt) TableName() string { return "identity_login_attempts" }

type loginAttemptEvent struct {
	ID            int64
	AttemptID     int64
	Event         string
	Reason        *string
	UserID        *int64
	ObservedEmail *string
	ObservedName  *string
	OccurredAt    time.Time
}

func (loginAttemptEvent) TableName() string { return "identity_login_attempt_events" }

type user struct {
	ID          int64
	UserUUID    string
	DisplayName *string
}

func (user) TableName() string { return "identity_users" }

type account struct {
	ID     int64
	UserID int64
}

func (account) TableName() string { return "identity_accounts" }

type externalIdentity struct {
	ID        int64
	Issuer    string
	Subject   string
	UserID    int64
	Email     *string
	Name      *string
	CreatedAt time.Time
}

func (externalIdentity) TableName() string { return "identity_external_identities" }

type session struct {
	ID           int64
	SessionUUID  string
	UserID       int64
	RefreshToken string
	IssuedAt     time.Time
	StartedAt    time.Time
	ExpiresAt    *time.Time
}

func (session) TableName() string { return "identity_sessions" }

// optional is nil for an empty value, so it is stored as NULL.
func optional[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}
