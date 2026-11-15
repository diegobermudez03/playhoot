// Package grantadmin makes one account the administrator: the single admin
// of phase 1, named by the server's configuration and applied at startup.
package grantadmin

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils"
)

// Note is why a grant seeded from configuration exists.
const Note = "named by IDENTITY_ADMIN_USER_UUID"

type grantRow struct {
	ID        int64
	UserID    int64
	GrantedAt time.Time
	RevokedAt *time.Time
	Note      string
}

func (grantRow) TableName() string { return "identity_admin_grants" }

// Service seeds the administrator grant.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// GetDB returns the database, for transactions.
func (s *Service) GetDB() *gorm.DB {
	return s.db
}

// EnsureAdmin makes the account userUUID the only administrator, revoking
// any other active grant. A guest or unknown User gets nothing.
func (s *Service) EnsureAdmin(ctx context.Context, userUUID string) (identity.AdminGrantOutcome, error) {
	now := s.now().UTC().Truncate(time.Microsecond)
	outcome, err := utils.RunInDBTransaction(ctx, s, func(ctx context.Context, tx *gorm.DB) (identity.AdminGrantOutcome, error) {
		var ids []int64
		err := tx.WithContext(ctx).Raw(`SELECT u.id FROM identity_users u JOIN identity_accounts a ON a.user_id = u.id
			WHERE u.user_uuid = ? FOR UPDATE OF u`, userUUID).Scan(&ids).Error
		if err != nil {
			return "", fmt.Errorf("locking admin account: %s", err)
		}
		if len(ids) == 0 {
			return identity.AdminGrantNotAccount, nil
		}
		userID := ids[0]
		if err := tx.WithContext(ctx).Exec(`UPDATE identity_admin_grants SET revoked_at = ? WHERE revoked_at IS NULL AND user_id <> ?`, now, userID).Error; err != nil {
			return "", fmt.Errorf("revoking other admin grants: %s", err)
		}
		var active int64
		if err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM identity_admin_grants WHERE user_id = ? AND revoked_at IS NULL`, userID).Scan(&active).Error; err != nil {
			return "", fmt.Errorf("reading admin grant: %s", err)
		}
		if active > 0 {
			return identity.AdminGrantKept, nil
		}
		if err := tx.WithContext(ctx).Create(&grantRow{UserID: userID, GrantedAt: now, Note: Note}).Error; err != nil {
			return "", fmt.Errorf("creating admin grant: %s", err)
		}
		return identity.AdminGrantCreated, nil
	})
	if err != nil {
		return "", err
	}
	logging.LogFields(ctx, logging.Field("admin_grant", string(outcome)))
	return outcome, nil
}
