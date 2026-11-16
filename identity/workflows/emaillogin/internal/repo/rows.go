package repo

import "time"

// The rows this repository inserts, one struct per table.

type emailCode struct {
	ID          int64
	AttemptUUID string
	Email       string
	Intent      string
	CodeSha256  string `gorm:"column:code_sha256"`
	Language    string
	GuestUserID *int64
	LinkUserID  *int64
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

func (emailCode) TableName() string { return "identity_email_codes" }

type emailCodeEvent struct {
	ID        int64
	CodeID    int64
	Event     string
	CreatedAt time.Time
}

func (emailCodeEvent) TableName() string { return "identity_email_code_events" }

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
