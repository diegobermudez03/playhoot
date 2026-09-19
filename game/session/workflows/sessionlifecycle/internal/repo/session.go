package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SetSessionTerminal persists an already-decided terminal transition
// (expiration ownership - deciding *whether* now is at/after
// lobby_expires_at - belongs to the Manager, not this method). updated_at is
// an audit timestamp the repository stamps itself
// (`docs/engineering/standards/repositories.md`'s Timestamp Ownership).
func (r *Repo) SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE sessions
		SET phase = ?, terminal_at = ?, terminal_reason = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, session.PhaseTerminal, terminalAt, terminalReason, sessionID).Error; err != nil {
		return fmt.Errorf("setting session terminal: %s", err)
	}
	return nil
}

// CreatedSession is the persisted shape of a freshly created Session and its
// mandatory host relationship - the Host/SessionActor Creation Cycle's
// result (WORK-0001's Host/SessionActor Creation Cycle).
type CreatedSession struct {
	SessionID      uint
	SessionUUID    string
	HostActorID    uint
	LobbyExpiresAt time.Time
}

type sessionInsert struct {
	ID                 uint      `gorm:"column:id"`
	UUID               string    `gorm:"column:uuid"`
	GameDefinitionUUID string    `gorm:"column:game_definition_uuid"`
	Phase              string    `gorm:"column:phase"`
	LobbyExpiresAt     time.Time `gorm:"column:lobby_expires_at"`
}

func (sessionInsert) TableName() string { return "sessions" }

// CreateSessionWithHost persists the Host/SessionActor Creation Cycle
// (WORK-0001): inserts the sessions row with host_actor_id NULL, inserts the
// host session_actors row, then assigns host_actor_id - one cohesive
// structural invariant (a validly persisted Session always has its mandatory
// host relationship), not application-level orchestration (see
// `docs/engineering/standards/repositories.md`'s Multi-Table Persistence
// Encapsulation). It does not create a JoinCode; that remains a separate,
// Manager-orchestrated step within the same transaction. created_at/
// updated_at are audit timestamps the DB stamps itself
// (`repositories.md`'s Timestamp Ownership); lobbyExpiresAt is semantic
// lifecycle state and remains an explicit input.
func (r *Repo) CreateSessionWithHost(ctx context.Context, tx *gorm.DB, gameDefinitionUUID string, hostUserUUID string, lobbyExpiresAt time.Time) (CreatedSession, error) {
	sessionRow := sessionInsert{
		UUID:               uuid.NewString(),
		GameDefinitionUUID: gameDefinitionUUID,
		Phase:              session.PhaseLobby,
		LobbyExpiresAt:     lobbyExpiresAt,
	}
	if err := tx.WithContext(ctx).Create(&sessionRow).Error; err != nil {
		return CreatedSession{}, fmt.Errorf("creating session: %s", err)
	}

	actorID, err := createActorRow(ctx, tx, sessionRow.ID, hostUserUUID)
	if err != nil {
		return CreatedSession{}, fmt.Errorf("creating host session actor: %s", err)
	}

	if err := tx.WithContext(ctx).Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, actorID, sessionRow.ID).Error; err != nil {
		return CreatedSession{}, fmt.Errorf("assigning host actor to session: %s", err)
	}

	return CreatedSession{
		SessionID:      sessionRow.ID,
		SessionUUID:    sessionRow.UUID,
		HostActorID:    actorID,
		LobbyExpiresAt: sessionRow.LobbyExpiresAt,
	}, nil
}
