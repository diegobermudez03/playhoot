package repo

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"
)

// maxJoinCodeAttempts bounds retries generating a numeric JoinCode that does
// not collide with another currently-active code.
const maxJoinCodeAttempts = 10

// JoinCodeResolution identifies which Session a JoinCode currently names,
// and that Session's pinned Definition/Version UUID. RevokedAt is non-nil
// when the resolved join_codes row is itself already revoked - the caller
// still re-validates the Session's admissibility under lock afterward, using
// RevokedAt to distinguish a code revoked independently of the Session it
// names still being open (genuinely invalid) from one revoked concurrently
// by that same Session's own lazy lobby-expiration materialization (a value
// outcome, not an error).
type JoinCodeResolution struct {
	SessionID          uint
	GameDefinitionUUID string
	RevokedAt          *time.Time
}

// ResolveSessionForJoinCode is an unlocked, pre-transaction lookup: it lets
// the Manager read the Game Management pinned-definition capability before
// opening the mutation transaction. It resolves the most recently issued
// join_codes row for joinCode regardless of revocation
// status (never filtered to revoked_at IS NULL) - a code number is reused
// over time as old assignments are revoked, so the most recent row is always
// the currently-relevant assignment, active or not. The Session's
// admissibility itself is always re-validated under lock afterward.
func (r *Repo) ResolveSessionForJoinCode(ctx context.Context, joinCode uint) (*JoinCodeResolution, error) {
	var resolution JoinCodeResolution
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, s.game_definition_uuid, jc.revoked_at
		FROM sessions s
		INNER JOIN join_codes jc ON jc.session_id = s.id
		WHERE jc.code = ?
		ORDER BY jc.id DESC
		LIMIT 1
	`, joinCode).Scan(&resolution)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving join code: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &resolution, nil
}

// CreateJoinCode persists a fresh active JoinCode for sessionID, retrying on
// a collision with another currently-active code. created_at is an audit
// timestamp the DB stamps itself.
//
// A collision is detected via a non-error ON CONFLICT DO NOTHING path
// against the active-code partial unique index, rather than a real
// unique-violation error: an actual Postgres error here would abort the
// caller's whole Create transaction, leaving no usable transaction to retry
// a fresh random code against.
func (r *Repo) CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint) (uint, error) {
	for attempt := 0; attempt < maxJoinCodeAttempts; attempt++ {
		code := uint(rand.Intn(9000) + 1000) //nolint:gosec // not security sensitive, just a human-facing lobby code
		var inserted struct {
			ID uint `gorm:"column:id"`
		}
		result := tx.WithContext(ctx).Raw(`
			INSERT INTO join_codes (session_id, code)
			VALUES (?, ?)
			ON CONFLICT (code) WHERE revoked_at IS NULL DO NOTHING
			RETURNING id
		`, sessionID, code).Scan(&inserted)
		if result.Error != nil {
			return 0, fmt.Errorf("creating join code: %s", result.Error)
		}
		if result.RowsAffected > 0 {
			return code, nil
		}
	}
	return 0, fmt.Errorf("creating join code: exhausted %d attempts generating a unique active code", maxJoinCodeAttempts)
}

// RevokeActiveJoinCode revokes sessionID's currently active JoinCode, if
// any. revokedAt is the semantic revocation event time, explicit workflow
// input.
func (r *Repo) RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, revokedAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE join_codes
		SET revoked_at = ?
		WHERE session_id = ? AND revoked_at IS NULL
	`, revokedAt, sessionID).Error; err != nil {
		return fmt.Errorf("revoking join code: %s", err)
	}
	return nil
}
