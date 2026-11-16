// Package repo persists the guest sweep: finding the guests nobody has used
// for the whole term.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is the guest sweep's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// InactiveGuests returns the uuids of up to limit guests whose newest
// session use, or their creation when no session was ever used, is older
// than before. A User with an account is never one, whichever mode they are
// in, and neither is a User already deleted.
func (r *Repo) InactiveGuests(ctx context.Context, before time.Time, limit int) ([]string, error) {
	var uuids []string
	err := r.db.WithContext(ctx).Raw(`
		SELECT users.user_uuid
		FROM identity_users users
		LEFT JOIN identity_accounts account ON account.user_id = users.id
		WHERE users.deleted_at IS NULL AND account.id IS NULL
			AND COALESCE(
				(SELECT MAX(session.last_used_at) FROM identity_sessions session WHERE session.user_id = users.id),
				users.created_at
			) < ?
		ORDER BY users.created_at
		LIMIT ?`,
		before, limit,
	).Scan(&uuids).Error
	if err != nil {
		return nil, fmt.Errorf("finding the guests to expire: %s", err)
	}
	return uuids, nil
}
