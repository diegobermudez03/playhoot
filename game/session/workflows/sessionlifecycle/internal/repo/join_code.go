package repo

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/diegobermudez03/playhoot/game/session/internal/pgerrs"
	"gorm.io/gorm"
)

// maxJoinCodeAttempts bounds retries generating a numeric JoinCode that does
// not collide with another currently-active code.
const maxJoinCodeAttempts = 10

// JoinCodeResolution identifies which Session an active JoinCode currently
// belongs to, and that Session's pinned Definition/Version UUID.
type JoinCodeResolution struct {
	SessionID          uint
	GameDefinitionUUID string
}

// ResolveActiveSessionForJoinCode is an unlocked, pre-transaction lookup: it
// lets the Manager read the Game Management pinned-definition capability
// before opening the mutation transaction (GAME-ADR-0001). The Session's
// admissibility itself is always re-validated under lock afterward.
func (r *Repo) ResolveActiveSessionForJoinCode(ctx context.Context, joinCode uint) (*JoinCodeResolution, error) {
	var resolution JoinCodeResolution
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, s.game_definition_uuid
		FROM sessions s
		INNER JOIN join_codes jc ON jc.session_id = s.id
		WHERE jc.code = ? AND jc.revoked_at IS NULL
	`, joinCode).Scan(&resolution)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving join code: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &resolution, nil
}

type joinCodeInsert struct {
	ID        uint      `gorm:"column:id"`
	SessionID uint      `gorm:"column:session_id"`
	Code      uint      `gorm:"column:code"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (joinCodeInsert) TableName() string { return "join_codes" }

// CreateJoinCode persists a fresh active JoinCode for sessionID, retrying on
// a collision with another currently-active code.
func (r *Repo) CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) (uint, error) {
	for attempt := 0; attempt < maxJoinCodeAttempts; attempt++ {
		code := uint(rand.Intn(9000) + 1000) //nolint:gosec // not security sensitive, just a human-facing lobby code
		row := joinCodeInsert{
			SessionID: sessionID,
			Code:      code,
			CreatedAt: now,
		}
		err := tx.WithContext(ctx).Create(&row).Error
		if err == nil {
			return code, nil
		}
		if !pgerrs.IsUniqueViolation(err) {
			return 0, fmt.Errorf("creating join code: %s", err)
		}
	}
	return 0, fmt.Errorf("creating join code: exhausted %d attempts generating a unique active code", maxJoinCodeAttempts)
}

// RevokeActiveJoinCode revokes sessionID's currently active JoinCode, if any.
func (r *Repo) RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE join_codes
		SET revoked_at = ?
		WHERE session_id = ? AND revoked_at IS NULL
	`, now, sessionID).Error; err != nil {
		return fmt.Errorf("revoking join code: %s", err)
	}
	return nil
}
