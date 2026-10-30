package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// LinkAccount makes the guest guestUserID an account within tx, keeping their
// User and display name: their account, its binding to the external identity,
// and the end of their guest sessions (which never expire). linked is false
// when that User already has an account; bound is false when another sign-in
// bound the identity first, and the caller rolls tx back.
func (r *Repo) LinkAccount(ctx context.Context, tx *gorm.DB, guestUserID int64, identity ExternalIdentity, linkedAt time.Time) (bool, bool, error) {
	made := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&account{UserID: guestUserID})
	if made.Error != nil {
		return false, false, fmt.Errorf("making the guest an account: %s", made.Error)
	}
	if made.RowsAffected == 0 {
		return false, true, nil
	}
	bound, err := bindIdentity(ctx, tx, guestUserID, identity, linkedAt)
	if err != nil || !bound {
		return true, bound, err
	}
	// A rotation committing while the update waits on its row leaves a new
	// active row the update could not see: repeat until none is left.
	for {
		result := tx.WithContext(ctx).Exec(`
			UPDATE identity_sessions SET deactivated_at = ?
			WHERE user_id = ? AND deactivated_at IS NULL`,
			linkedAt, guestUserID,
		)
		if result.Error != nil {
			return true, true, fmt.Errorf("ending the guest's sessions: %s", result.Error)
		}
		var active int64
		err := tx.WithContext(ctx).Raw(`
			SELECT count(*) FROM identity_sessions WHERE user_id = ? AND deactivated_at IS NULL`, guestUserID).
			Scan(&active).Error
		if err != nil {
			return true, true, fmt.Errorf("checking the guest's sessions ended: %s", err)
		}
		if active == 0 {
			return true, true, nil
		}
	}
}

// CreateAccount creates, within tx, a User without a display name, their
// account, and its binding to the external identity (keeping what the
// provider said when it was bound). False when another sign-in bound it
// first: the caller rolls tx back, leaving no User behind.
func (r *Repo) CreateAccount(ctx context.Context, tx *gorm.DB, userUUID string, identity ExternalIdentity, createdAt time.Time) (int64, bool, error) {
	created := user{UserUUID: userUUID}
	if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
		return 0, false, fmt.Errorf("creating account user: %s", err)
	}
	if err := tx.WithContext(ctx).Create(&account{UserID: created.ID}).Error; err != nil {
		return 0, false, fmt.Errorf("creating account: %s", err)
	}
	bound, err := bindIdentity(ctx, tx, created.ID, identity, createdAt)
	return created.ID, bound, err
}

// bindIdentity binds the external identity to userID within tx, keeping what
// the provider said when it was bound. False when it was bound already.
func bindIdentity(ctx context.Context, tx *gorm.DB, userID int64, identity ExternalIdentity, boundAt time.Time) (bool, error) {
	binding := externalIdentity{
		Issuer:    identity.Issuer,
		Subject:   identity.Subject,
		UserID:    userID,
		Email:     optional(identity.Email),
		Name:      optional(identity.Name),
		CreatedAt: boundAt,
	}
	bound := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&binding)
	if bound.Error != nil {
		return false, fmt.Errorf("binding external identity: %s", bound.Error)
	}
	return bound.RowsAffected > 0, nil
}
