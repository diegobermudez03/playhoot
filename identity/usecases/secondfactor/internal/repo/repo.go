// Package repo persists the second factor: grants read, TOTP credentials,
// and the trail of code checks with the step-up proofs among them.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is the second factor's persistence.
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

// User is the caller, locked for the rest of a transaction.
type User struct {
	ID          int64
	DisplayName *string
}

// LockUser returns the User with userUUID, locked for the rest of tx so
// their checks happen one at a time, or nil when there is none.
func (r *Repo) LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*User, error) {
	var users []User
	err := tx.WithContext(ctx).Raw(`SELECT id, display_name FROM identity_users WHERE user_uuid = ? FOR UPDATE`, userUUID).Scan(&users).Error
	if err != nil {
		return nil, fmt.Errorf("locking user: %s", err)
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

// HasActiveGrant reports whether the User holds an unrevoked admin grant.
func (r *Repo) HasActiveGrant(ctx context.Context, tx *gorm.DB, userID int64) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM identity_admin_grants WHERE user_id = ? AND revoked_at IS NULL`, userID).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("reading admin grant: %s", err)
	}
	return count > 0, nil
}

// Credential is a stored TOTP credential.
type Credential struct {
	ID              int64
	SecretEncrypted string
	CreatedAt       time.Time
}

// ConfirmedCredential returns the User's confirmed, unrevoked credential, or
// nil.
func (r *Repo) ConfirmedCredential(ctx context.Context, tx *gorm.DB, userID int64) (*Credential, error) {
	return r.credential(ctx, tx, `SELECT id, secret_encrypted, created_at FROM identity_totp_credentials
		WHERE user_id = ? AND revoked_at IS NULL AND confirmed_at IS NOT NULL`, userID)
}

// PendingCredential returns the User's unconfirmed, unrevoked credential, or
// nil.
func (r *Repo) PendingCredential(ctx context.Context, tx *gorm.DB, userID int64) (*Credential, error) {
	return r.credential(ctx, tx, `SELECT id, secret_encrypted, created_at FROM identity_totp_credentials
		WHERE user_id = ? AND revoked_at IS NULL AND confirmed_at IS NULL`, userID)
}

func (r *Repo) credential(ctx context.Context, tx *gorm.DB, query string, userID int64) (*Credential, error) {
	var credentials []Credential
	if err := tx.WithContext(ctx).Raw(query, userID).Scan(&credentials).Error; err != nil {
		return nil, fmt.Errorf("reading totp credential: %s", err)
	}
	if len(credentials) == 0 {
		return nil, nil
	}
	return &credentials[0], nil
}

type credentialRow struct {
	ID              int64
	UserID          int64
	SecretEncrypted string
	CreatedAt       time.Time
	ConfirmedAt     *time.Time
	RevokedAt       *time.Time
}

func (credentialRow) TableName() string { return "identity_totp_credentials" }

// CreatePendingCredential stores a new unconfirmed credential.
func (r *Repo) CreatePendingCredential(ctx context.Context, tx *gorm.DB, userID int64, secretEncrypted string, now time.Time) error {
	row := credentialRow{UserID: userID, SecretEncrypted: secretEncrypted, CreatedAt: now}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating totp credential: %s", err)
	}
	return nil
}

// RevokePending revokes the User's unconfirmed credential, if any.
func (r *Repo) RevokePending(ctx context.Context, tx *gorm.DB, userID int64, now time.Time) error {
	err := tx.WithContext(ctx).Exec(`UPDATE identity_totp_credentials SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL AND confirmed_at IS NULL`, now, userID).Error
	if err != nil {
		return fmt.Errorf("revoking pending totp credential: %s", err)
	}
	return nil
}

// RevokeConfirmed revokes the User's confirmed credential, if any.
func (r *Repo) RevokeConfirmed(ctx context.Context, tx *gorm.DB, userID int64, now time.Time) error {
	err := tx.WithContext(ctx).Exec(`UPDATE identity_totp_credentials SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL AND confirmed_at IS NOT NULL`, now, userID).Error
	if err != nil {
		return fmt.Errorf("revoking confirmed totp credential: %s", err)
	}
	return nil
}

// Confirm marks an unconfirmed credential confirmed.
func (r *Repo) Confirm(ctx context.Context, tx *gorm.DB, credentialID int64, now time.Time) error {
	err := tx.WithContext(ctx).Exec(`UPDATE identity_totp_credentials SET confirmed_at = ?
		WHERE id = ? AND confirmed_at IS NULL AND revoked_at IS NULL`, now, credentialID).Error
	if err != nil {
		return fmt.Errorf("confirming totp credential: %s", err)
	}
	return nil
}

// RecentFailures returns when the User's checks with one of failureOutcomes
// happened since since, newest first, at most limit of them.
func (r *Repo) RecentFailures(ctx context.Context, tx *gorm.DB, userID int64, failureOutcomes []string, since time.Time, limit int) ([]time.Time, error) {
	var times []time.Time
	err := tx.WithContext(ctx).Raw(`SELECT created_at FROM identity_second_factor_checks
		WHERE user_id = ? AND created_at > ? AND outcome IN ? ORDER BY created_at DESC LIMIT ?`,
		userID, since, failureOutcomes, limit).Scan(&times).Error
	if err != nil {
		return nil, fmt.Errorf("reading second factor failures: %s", err)
	}
	return times, nil
}

// LastAcceptedStep returns the newest time step the User had accepted, or
// false when they never had one.
func (r *Repo) LastAcceptedStep(ctx context.Context, tx *gorm.DB, userID int64, acceptedOutcome string) (int64, bool, error) {
	var steps []int64
	err := tx.WithContext(ctx).Raw(`SELECT time_step FROM identity_second_factor_checks
		WHERE user_id = ? AND outcome = ? ORDER BY time_step DESC LIMIT 1`, userID, acceptedOutcome).Scan(&steps).Error
	if err != nil {
		return 0, false, fmt.Errorf("reading last accepted step: %s", err)
	}
	if len(steps) == 0 {
		return 0, false, nil
	}
	return steps[0], true, nil
}

// Check is one second-factor check to record.
type Check struct {
	ID          int64
	UserID      int64
	Purpose     string
	Outcome     string
	TimeStep    *int64
	ProofSHA256 *string `gorm:"column:proof_sha256"`
	ExpiresAt   *time.Time
	UsedFor     *string
	UsedAt      *time.Time
	CreatedAt   time.Time
}

func (Check) TableName() string { return "identity_second_factor_checks" }

// RecordCheck stores a check.
func (r *Repo) RecordCheck(ctx context.Context, tx *gorm.DB, check Check) error {
	if err := tx.WithContext(ctx).Create(&check).Error; err != nil {
		return fmt.Errorf("recording second factor check: %s", err)
	}
	return nil
}

// ConsumeProof spends the User's live proof hashing to proofSHA256 on
// action, reporting whether there was one to spend.
func (r *Repo) ConsumeProof(ctx context.Context, userUUID string, purpose string, proofSHA256 string, action string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`UPDATE identity_second_factor_checks c SET used_for = ?, used_at = ?
		FROM identity_users u
		WHERE c.proof_sha256 = ? AND c.purpose = ? AND c.user_id = u.id AND u.user_uuid = ?
			AND c.used_at IS NULL AND c.expires_at > ?`,
		action, now, proofSHA256, purpose, userUUID, now)
	if result.Error != nil {
		return false, fmt.Errorf("consuming step-up proof: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// Proof is a stored step-up proof.
type Proof struct {
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// FindProof returns the User's proof hashing to proofSHA256, or nil.
func (r *Repo) FindProof(ctx context.Context, userUUID string, purpose string, proofSHA256 string) (*Proof, error) {
	var proofs []Proof
	err := r.db.WithContext(ctx).Raw(`SELECT c.expires_at, c.used_at FROM identity_second_factor_checks c
		JOIN identity_users u ON u.id = c.user_id
		WHERE c.proof_sha256 = ? AND c.purpose = ? AND u.user_uuid = ?`, proofSHA256, purpose, userUUID).Scan(&proofs).Error
	if err != nil {
		return nil, fmt.Errorf("finding step-up proof: %s", err)
	}
	if len(proofs) == 0 {
		return nil, nil
	}
	return &proofs[0], nil
}
