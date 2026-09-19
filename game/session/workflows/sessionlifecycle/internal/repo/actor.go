package repo

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/session"
	"gorm.io/gorm"
)

// Actor is the persisted session_actors identity for a (Session, User).
type Actor struct {
	ID        uint
	SessionID uint
	UserUUID  string
}

// FindActor returns the SessionActor for (sessionID, userUUID), or nil, nil
// if none exists yet.
func (r *Repo) FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*Actor, error) {
	var row Actor
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_id, user_uuid
		FROM session_actors
		WHERE session_id = ? AND user_uuid = ?
	`, sessionID, userUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding session actor: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

type actorInsert struct {
	ID               uint   `gorm:"column:id"`
	SessionID        uint   `gorm:"column:session_id"`
	UserUUID         string `gorm:"column:user_uuid"`
	SemanticPresence string `gorm:"column:semantic_presence"`
}

func (actorInsert) TableName() string { return "session_actors" }

func createActorRow(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (uint, error) {
	row := actorInsert{
		SessionID:        sessionID,
		UserUUID:         userUUID,
		SemanticPresence: session.PresenceConnected,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

// CreateActor inserts a new SessionActor for (sessionID, userUUID), starting
// semantic_presence CONNECTED, and returns its id. created_at is an audit
// timestamp the DB stamps itself
// (`docs/engineering/standards/repositories.md`'s Timestamp Ownership).
func (r *Repo) CreateActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (uint, error) {
	id, err := createActorRow(ctx, tx, sessionID, userUUID)
	if err != nil {
		return 0, fmt.Errorf("creating session actor: %s", err)
	}
	return id, nil
}
