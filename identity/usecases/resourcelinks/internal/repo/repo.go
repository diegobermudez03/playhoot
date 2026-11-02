// Package repo persists resource links: only their tokens' hashes, at most
// one open link per User and resource, revocation written once.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is resource links' persistence.
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

// Link is a stored resource link with its User's uuid.
type Link struct {
	ID         int64
	UserID     int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	UserUUID   string
	Kind       string
	ResourceID string
}

type linkRow struct {
	ID           int64
	UserID       int64
	ResourceKind string
	ResourceID   string
	TokenSHA256  string `gorm:"column:token_sha256"`
	CreatedAt    time.Time
	ExpiresAt    time.Time
	RevokedAt    *time.Time
}

func (linkRow) TableName() string { return "identity_resource_links" }

// LockUser returns the id of the User with userUUID, locked for the rest of
// tx so their links change one at a time, or false when there is none.
func (r *Repo) LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (int64, bool, error) {
	var ids []int64
	err := tx.WithContext(ctx).Raw(`SELECT id FROM identity_users WHERE user_uuid = ? FOR UPDATE`, userUUID).Scan(&ids).Error
	if err != nil {
		return 0, false, fmt.Errorf("locking user: %s", err)
	}
	if len(ids) == 0 {
		return 0, false, nil
	}
	return ids[0], true, nil
}

// RevokeOpenLink revokes the User's open link to the resource, if any.
func (r *Repo) RevokeOpenLink(ctx context.Context, tx *gorm.DB, userID int64, kind string, resourceID string, revokedAt time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE identity_resource_links SET revoked_at = ?
		WHERE user_id = ? AND resource_kind = ? AND resource_id = ? AND revoked_at IS NULL`,
		revokedAt, userID, kind, resourceID,
	)
	if result.Error != nil {
		return false, fmt.Errorf("revoking resource link: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// CreateLink stores a new open link with its token's hash.
func (r *Repo) CreateLink(ctx context.Context, tx *gorm.DB, userID int64, kind string, resourceID string, tokenSHA256 string, createdAt time.Time, expiresAt time.Time) error {
	row := linkRow{UserID: userID, ResourceKind: kind, ResourceID: resourceID, TokenSHA256: tokenSHA256, CreatedAt: createdAt, ExpiresAt: expiresAt}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating resource link: %s", err)
	}
	return nil
}

const selectLink = `
	SELECT l.id, l.user_id, l.created_at, l.expires_at, l.revoked_at,
		u.user_uuid, l.resource_kind AS kind, l.resource_id
	FROM identity_resource_links l
	JOIN identity_users u ON u.id = l.user_id
`

// FindOpenLink returns the User's open (not revoked) link to the resource,
// expired or not, or nil when there is none.
func (r *Repo) FindOpenLink(ctx context.Context, userUUID string, kind string, resourceID string) (*Link, error) {
	var links []Link
	err := r.db.WithContext(ctx).Raw(selectLink+`WHERE u.user_uuid = ? AND l.resource_kind = ? AND l.resource_id = ? AND l.revoked_at IS NULL`,
		userUUID, kind, resourceID,
	).Scan(&links).Error
	if err != nil {
		return nil, fmt.Errorf("finding open resource link: %s", err)
	}
	if len(links) == 0 {
		return nil, nil
	}
	return &links[0], nil
}

// FindLinkByHash returns the link whose token hashes to tokenSHA256, with its
// User's uuid, or nil when there is none.
func (r *Repo) FindLinkByHash(ctx context.Context, tokenSHA256 string) (*Link, error) {
	var links []Link
	err := r.db.WithContext(ctx).Raw(selectLink+`WHERE l.token_sha256 = ?`, tokenSHA256).Scan(&links).Error
	if err != nil {
		return nil, fmt.Errorf("finding resource link: %s", err)
	}
	if len(links) == 0 {
		return nil, nil
	}
	return &links[0], nil
}
