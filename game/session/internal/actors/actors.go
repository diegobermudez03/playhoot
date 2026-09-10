// Package actors implements the SessionActor/Participant persistence
// mechanics shared by JoinSession and LeaveSession: finding/creating a
// SessionActor for (session, user_uuid), and finding/creating/reactivating/
// deactivating its at-most-one Participant row.
package actors

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"gorm.io/gorm"
)

// Row is a persisted session_actors record.
type Row struct {
	ID        uint
	SessionID uint
	UserUUID  string
}

// Find returns the SessionActor for (sessionID, userUUID), or nil, nil if
// none exists yet.
func Find(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*Row, error) {
	var row Row
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
	ID               uint      `gorm:"column:id"`
	SessionID        uint      `gorm:"column:session_id"`
	UserUUID         string    `gorm:"column:user_uuid"`
	SemanticPresence string    `gorm:"column:semantic_presence"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (actorInsert) TableName() string { return "session_actors" }

// Create inserts a new SessionActor for (sessionID, userUUID), starting
// semantic_presence CONNECTED, and returns its id.
func Create(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string, now time.Time) (uint, error) {
	row := actorInsert{
		SessionID:        sessionID,
		UserUUID:         userUUID,
		SemanticPresence: session.PresenceConnected,
		CreatedAt:        now,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating session actor: %s", err)
	}
	return row.ID, nil
}

// ParticipantRow is a persisted session_participants record.
type ParticipantRow struct {
	ID             uint
	SessionActorID uint
	DisplayName    string
	Active         bool
}

// FindParticipant returns the Participant owned by actorID, or nil, nil if
// none has ever been created for it.
func FindParticipant(ctx context.Context, tx *gorm.DB, actorID uint) (*ParticipantRow, error) {
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

type participantInsert struct {
	ID             uint      `gorm:"column:id"`
	SessionActorID uint      `gorm:"column:session_actor_id"`
	DisplayName    string    `gorm:"column:display_name"`
	Active         bool      `gorm:"column:active"`
	JoinedAt       time.Time `gorm:"column:joined_at"`
}

func (participantInsert) TableName() string { return "session_participants" }

// CreateParticipant inserts the first, active Participant for actorID.
func CreateParticipant(ctx context.Context, tx *gorm.DB, actorID uint, displayName string, now time.Time) error {
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

// ReactivateParticipant re-admits a previously deactivated Participant,
// refreshing its display-name snapshot and joined_at.
func ReactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, displayName string, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET active = TRUE, left_at = NULL, display_name = ?, joined_at = ?
		WHERE id = ?
	`, displayName, now, participantID).Error; err != nil {
		return fmt.Errorf("reactivating participant: %s", err)
	}
	return nil
}

// RefreshActiveDisplayName updates the display-name snapshot of an already
// active Participant, keeping a repeated identical Join idempotent without
// mutating joined_at/left_at.
func RefreshActiveDisplayName(ctx context.Context, tx *gorm.DB, participantID uint, displayName string) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET display_name = ?
		WHERE id = ?
	`, displayName, participantID).Error; err != nil {
		return fmt.Errorf("refreshing participant display name: %s", err)
	}
	return nil
}

// DeactivateParticipant releases participantID's lobby slot.
func DeactivateParticipant(ctx context.Context, tx *gorm.DB, participantID uint, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_participants
		SET active = FALSE, left_at = ?
		WHERE id = ?
	`, now, participantID).Error; err != nil {
		return fmt.Errorf("deactivating participant: %s", err)
	}
	return nil
}

// CountActive returns the number of currently active Participants for
// sessionID.
func CountActive(ctx context.Context, tx *gorm.DB, sessionID uint) (int, error) {
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
