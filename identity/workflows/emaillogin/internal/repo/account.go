package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BoundIdentity is the live binding of an address or provider subject, with
// what was observed about the person when it was bound.
type BoundIdentity struct {
	ID        int64
	UserID    int64
	UserUUID  string
	Email     string
	Name      string
	CreatedAt time.Time
}

// FindBoundUser returns, within tx, the User the live binding of (issuer,
// subject) belongs to, or nil when it is bound to nobody.
func (r *Repo) FindBoundUser(ctx context.Context, tx *gorm.DB, issuer string, subject string) (*BoundIdentity, error) {
	var rows []BoundIdentity
	err := tx.WithContext(ctx).Raw(`
		SELECT bound.id, bound.user_id, users.user_uuid,
			COALESCE(bound.email, '') AS email, COALESCE(bound.name, '') AS name, bound.created_at
		FROM identity_external_identities bound JOIN identity_users users ON users.id = bound.user_id
		WHERE bound.issuer = ? AND bound.subject = ? AND bound.removed_at IS NULL`,
		issuer, subject,
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("finding external identity: %s", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// Identified is who a User is once a code resolved them.
type Identified struct {
	UserID      int64
	UserUUID    string
	DisplayName string
}

// ReadUser returns who userID is within tx.
func (r *Repo) ReadUser(ctx context.Context, tx *gorm.DB, userID int64) (Identified, error) {
	var rows []Identified
	err := tx.WithContext(ctx).Raw(`
		SELECT id AS user_id, user_uuid, COALESCE(display_name, '') AS display_name FROM identity_users WHERE id = ?`,
		userID,
	).Scan(&rows).Error
	if err != nil {
		return Identified{}, fmt.Errorf("reading a user: %s", err)
	}
	if len(rows) == 0 {
		return Identified{}, fmt.Errorf("reading a user: %d does not exist", userID)
	}
	return rows[0], nil
}

// ExternalIdentity is a way into an account: Issuer and Subject identify it,
// Email and Name are what was observed about the person.
type ExternalIdentity struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
}

// LinkAccount makes the guest guestUserID an account within tx, keeping
// their User and display name, and binds the identity to it. linked is false
// when that User already has an account; bound is false when the identity
// was bound meanwhile, and the caller rolls tx back.
func (r *Repo) LinkAccount(ctx context.Context, tx *gorm.DB, guestUserID int64, identity ExternalIdentity, linkedAt time.Time) (linked bool, bound bool, err error) {
	made := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&account{UserID: guestUserID})
	if made.Error != nil {
		return false, false, fmt.Errorf("making the guest an account: %s", made.Error)
	}
	if made.RowsAffected == 0 {
		return false, true, nil
	}
	bound, err = r.BindIdentity(ctx, tx, guestUserID, identity, linkedAt)
	if err != nil || !bound {
		return true, bound, err
	}
	return true, true, r.EndSessionsOf(ctx, tx, guestUserID, linkedAt)
}

// CreateAccount creates, within tx, a User without a display name, their
// account, and the binding of the identity. bound is false when the identity
// was bound meanwhile: the caller rolls tx back, leaving no User behind.
func (r *Repo) CreateAccount(ctx context.Context, tx *gorm.DB, userUUID string, identity ExternalIdentity, createdAt time.Time) (int64, bool, error) {
	created := user{UserUUID: userUUID}
	if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
		return 0, false, fmt.Errorf("creating account user: %s", err)
	}
	if err := tx.WithContext(ctx).Create(&account{UserID: created.ID}).Error; err != nil {
		return 0, false, fmt.Errorf("creating account: %s", err)
	}
	bound, err := r.BindIdentity(ctx, tx, created.ID, identity, createdAt)
	return created.ID, bound, err
}

// BindIdentity binds the identity to userID within tx, keeping what was
// observed when it was bound. False when it is bound to someone already.
func (r *Repo) BindIdentity(ctx context.Context, tx *gorm.DB, userID int64, identity ExternalIdentity, boundAt time.Time) (bool, error) {
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
