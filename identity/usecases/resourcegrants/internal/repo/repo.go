// Package repo persists single-use grants: only their tokens' hashes, each
// used at most once at a time.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is single-use grants' persistence.
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

type grantRow struct {
	ID           int64
	UUID         string `gorm:"column:uuid"`
	UserID       int64
	ResourceKind string
	ResourceID   string
	Scope        string
	TokenSHA256  string `gorm:"column:token_sha256"`
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       *time.Time
}

func (grantRow) TableName() string { return "identity_resource_grants" }

// Grant is a stored grant with its User's uuid.
type Grant struct {
	UUID       string `gorm:"column:uuid"`
	UserUUID   string
	Kind       string
	ResourceID string
	Scope      string
	ExpiresAt  time.Time
	UsedAt     *time.Time
}

// LockUser returns the id of the User with userUUID, locked for the rest of
// tx so their grants are counted and created one at a time, or false when
// there is none.
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

// DeleteExpiredGrants deletes the User's grants that expired before before.
func (r *Repo) DeleteExpiredGrants(ctx context.Context, tx *gorm.DB, userID int64, before time.Time) error {
	if err := tx.WithContext(ctx).Exec(`DELETE FROM identity_resource_grants WHERE user_id = ? AND expires_at < ?`, userID, before).Error; err != nil {
		return fmt.Errorf("deleting expired resource grants: %s", err)
	}
	return nil
}

// CountLiveGrants counts the User's grants neither used nor expired at now.
func (r *Repo) CountLiveGrants(ctx context.Context, tx *gorm.DB, userID int64, now time.Time) (int64, error) {
	var count int64
	if err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM identity_resource_grants WHERE user_id = ? AND used_at IS NULL AND expires_at > ?`, userID, now).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("counting resource grants: %s", err)
	}
	return count, nil
}

// CreateGrant stores a new grant with its token's hash.
func (r *Repo) CreateGrant(ctx context.Context, tx *gorm.DB, grantUUID string, userID int64, kind string, resourceID string, scope string, tokenSHA256 string, createdAt time.Time, expiresAt time.Time) error {
	row := grantRow{UUID: grantUUID, UserID: userID, ResourceKind: kind, ResourceID: resourceID, Scope: scope, TokenSHA256: tokenSHA256, CreatedAt: createdAt, ExpiresAt: expiresAt}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating resource grant: %s", err)
	}
	return nil
}

const selectGrant = `
	SELECT g.uuid, u.user_uuid, g.resource_kind AS kind, g.resource_id, g.scope, g.expires_at, g.used_at
	FROM identity_resource_grants g
	JOIN identity_users u ON u.id = g.user_id
`

// UseGrant marks the grant whose token hashes to tokenSHA256 used at now,
// only if it is unused and unexpired, and returns it; nil when it was not.
func (r *Repo) UseGrant(ctx context.Context, tokenSHA256 string, now time.Time) (*Grant, error) {
	var grants []Grant
	err := r.db.WithContext(ctx).Raw(`
		WITH used AS (
			UPDATE identity_resource_grants SET used_at = ?
			WHERE token_sha256 = ? AND used_at IS NULL AND expires_at > ?
			RETURNING uuid, user_id, resource_kind, resource_id, scope, expires_at, used_at
		)
		SELECT used.uuid, u.user_uuid, used.resource_kind AS kind, used.resource_id, used.scope, used.expires_at, used.used_at
		FROM used JOIN identity_users u ON u.id = used.user_id`,
		now, tokenSHA256, now,
	).Scan(&grants).Error
	if err != nil {
		return nil, fmt.Errorf("using resource grant: %s", err)
	}
	if len(grants) == 0 {
		return nil, nil
	}
	return &grants[0], nil
}

// FindGrantByHash returns the grant whose token hashes to tokenSHA256, or
// nil when there is none.
func (r *Repo) FindGrantByHash(ctx context.Context, tokenSHA256 string) (*Grant, error) {
	var grants []Grant
	if err := r.db.WithContext(ctx).Raw(selectGrant+`WHERE g.token_sha256 = ?`, tokenSHA256).Scan(&grants).Error; err != nil {
		return nil, fmt.Errorf("finding resource grant: %s", err)
	}
	if len(grants) == 0 {
		return nil, nil
	}
	return &grants[0], nil
}

// ReleaseGrant marks a used grant unused again; it reports whether it was
// used.
func (r *Repo) ReleaseGrant(ctx context.Context, grantUUID string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`UPDATE identity_resource_grants SET used_at = NULL WHERE uuid = ? AND used_at IS NOT NULL`, grantUUID)
	if result.Error != nil {
		return false, fmt.Errorf("releasing resource grant: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}
