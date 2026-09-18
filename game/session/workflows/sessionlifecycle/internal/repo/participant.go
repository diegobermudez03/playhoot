package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ParticipantRow is the persisted session_participants record owned by a
// SessionActor.
type ParticipantRow struct {
	ID             uint
	SessionActorID uint
	DisplayName    string
	Active         bool
}

// FindParticipant returns the Participant owned by actorID, or nil, nil if
// none has ever been created for it.
func (r *Repo) FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*ParticipantRow, error) {
	var row ParticipantRow
	result := tx.WithContext(ctx).Raw(`
		SELECT id, session_actor_id, display_name, active
		FROM session_participants
		WHERE session_actor_id = ?
	`, actorID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding participant: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// CountActiveParticipants returns the number of currently active
// Participants for sessionID.
func (r *Repo) CountActiveParticipants(ctx context.Context, tx *gorm.DB, sessionID uint) (int, error) {
	var count int
	if err := tx.WithContext(ctx).Raw(`
		SELECT COUNT(*)
		FROM session_participants p
		INNER JOIN session_actors a ON a.id = p.session_actor_id
		WHERE a.session_id = ? AND p.active = TRUE
	`, sessionID).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("counting active participants: %s", err)
	}
	return count, nil
}

type participantInsert struct {
	ID             uint      `gorm:"column:id"`
	SessionActorID uint      `gorm:"column:session_actor_id"`
	DisplayName    string    `gorm:"column:display_name"`
	Active         bool      `gorm:"column:active"`
	JoinedAt       time.Time `gorm:"column:joined_at"`
}

func (participantInsert) TableName() string { return "session_participants" }

// CreateParticipant inserts the first, active Participant for actorID.
func (r *Repo) CreateParticipant(ctx context.Context, tx *gorm.DB, actorID uint, displayName string, now time.Time) error {
	row := participantInsert{
		SessionActorID: actorID,
		DisplayName:    displayName,
		Active:         true,
		JoinedAt:       now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating participant: %s", err)
	}
	return nil
}

// ActivateParticipant re-admits a previously deactivated Participant,
// refreshing its display-name snapshot and joined_at.
func (r *Repo) ActivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, displayName string, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET active = TRUE, left_at = NULL, display_name = ?, joined_at = ?
		WHERE id = ?
	`, displayName, now, participantID).Error; err != nil {
		return fmt.Errorf("activating participant: %s", err)
	}
	return nil
}

// DeactivateParticipant releases participantID's lobby slot.
func (r *Repo) DeactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET active = FALSE, left_at = ?
		WHERE id = ?
	`, now, participantID).Error; err != nil {
		return fmt.Errorf("deactivating participant: %s", err)
	}
	return nil
}
