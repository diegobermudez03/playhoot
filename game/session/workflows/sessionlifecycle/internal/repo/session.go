package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SessionRow is the locked snapshot of a sessions row needed by lobby
// mutations - a thin alias over sessionlock.Row, the shared per-Session
// locking primitive's own fact-reporting shape.
type SessionRow = sessionlock.Row

// LockSessionByID selects the sessions row FOR UPDATE by its internal id via
// the shared sessionlock primitive, serializing every other lobby mutation
// attempted against the same Session. Callers must invoke this from within
// an already-open DB transaction. Returns nil, nil if no such Session
// exists.
func (r *Repo) LockSessionByID(ctx context.Context, tx *gorm.DB, sessionID uint) (*SessionRow, error) {
	return sessionlock.LockByID(ctx, tx, sessionID)
}

// LockSessionByUUID selects the sessions row FOR UPDATE by its public uuid
// via the shared sessionlock primitive. Returns nil, nil if no such Session
// exists.
func (r *Repo) LockSessionByUUID(ctx context.Context, tx *gorm.DB, sessionUUID string) (*SessionRow, error) {
	return sessionlock.LockByUUID(ctx, tx, sessionUUID)
}

// SetSessionTerminal persists an already-decided terminal transition
// (expiration ownership - deciding *whether* now is at/after
// lobby_expires_at - belongs to the Manager, not this method).
func (r *Repo) SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string, now time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE sessions
		SET phase = ?, terminal_at = ?, terminal_reason = ?, updated_at = ?
		WHERE id = ?
	`, session.PhaseTerminal, terminalAt, terminalReason, now, sessionID).Error; err != nil {
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
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (sessionInsert) TableName() string { return "sessions" }

// CreateSessionWithHost persists the Host/SessionActor Creation Cycle
// (WORK-0001): inserts the sessions row with host_actor_id NULL, inserts the
// host session_actors row, then assigns host_actor_id - one cohesive
// structural invariant (a validly persisted Session always has its mandatory
// host relationship), not application-level orchestration (see
// `docs/engineering/standards/repositories.md`'s Multi-Table Persistence
// Encapsulation). It does not create a JoinCode; that remains a separate,
// Manager-orchestrated step within the same transaction.
func (r *Repo) CreateSessionWithHost(ctx context.Context, tx *gorm.DB, gameDefinitionUUID string, hostUserUUID string, lobbyExpiresAt time.Time, now time.Time) (CreatedSession, error) {
	sessionRow := sessionInsert{
		UUID:               uuid.NewString(),
		GameDefinitionUUID: gameDefinitionUUID,
		Phase:              session.PhaseLobby,
		LobbyExpiresAt:     lobbyExpiresAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := tx.WithContext(ctx).Create(&sessionRow).Error; err != nil {
		return CreatedSession{}, fmt.Errorf("creating session: %s", err)
	}

	actorID, err := createActorRow(ctx, tx, sessionRow.ID, hostUserUUID, now)
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
