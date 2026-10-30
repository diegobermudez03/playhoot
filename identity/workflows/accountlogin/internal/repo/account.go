package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ExternalIdentity is an identity a provider vouched for: Issuer and Subject
// identify it, Email and Name are what the provider said about the person.
type ExternalIdentity struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
}

// FindBoundUser returns, within tx, the User the external identity is bound
// to, or zero when it is bound to nobody yet.
func (r *Repo) FindBoundUser(ctx context.Context, tx *gorm.DB, issuer string, subject string) (int64, error) {
	var userIDs []int64
	err := tx.WithContext(ctx).Raw(`
		SELECT user_id FROM identity_external_identities WHERE issuer = ? AND subject = ?`,
		issuer, subject,
	).Scan(&userIDs).Error
	if err != nil {
		return 0, fmt.Errorf("finding external identity: %s", err)
	}
	if len(userIDs) == 0 {
		return 0, nil
	}
	return userIDs[0], nil
}

// CreateAccount creates, within tx, a User without a display name, their
// account, and its binding to the external identity (keeping what the
// provider said when it was bound). False when another sign-in bound it
// first: the caller rolls tx back, leaving no User behind.
func (r *Repo) CreateAccount(ctx context.Context, tx *gorm.DB, userUUID string, identity ExternalIdentity, createdAt time.Time) (int64, bool, error) {
	var userID int64
	err := tx.WithContext(ctx).Raw(`
		INSERT INTO identity_users (user_uuid, display_name) VALUES (?, NULL) RETURNING id`,
		userUUID,
	).Scan(&userID).Error
	if err != nil {
		return 0, false, fmt.Errorf("creating account user: %s", err)
	}
	if err := tx.WithContext(ctx).Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error; err != nil {
		return 0, false, fmt.Errorf("creating account: %s", err)
	}
	var bound []int64
	err = tx.WithContext(ctx).Raw(`
		INSERT INTO identity_external_identities (issuer, subject, user_id, email, name, created_at)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)
		ON CONFLICT (issuer, subject) DO NOTHING
		RETURNING id`,
		identity.Issuer, identity.Subject, userID, identity.Email, identity.Name, createdAt,
	).Scan(&bound).Error
	if err != nil {
		return 0, false, fmt.Errorf("binding external identity: %s", err)
	}
	return userID, len(bound) > 0, nil
}
