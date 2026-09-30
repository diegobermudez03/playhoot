package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SetSessionTerminal persists an already-decided terminal transition
// (expiration ownership - deciding *whether* now is at/after
// lobby_expires_at - belongs to the Manager, not this method). updated_at is
// an audit timestamp the repository stamps itself.
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
// result.
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

// SetSessionRunning persists Start's successful LOBBY -> RUNNING transition,
// including the initial activity_expires_at deadline.
// Expiration/host/roster validation is Start's own business policy, decided
// before this call; this method only performs the already-decided mutation.
// updated_at is an audit timestamp the repository stamps itself; startedAt/
// activityExpiresAt are semantic lifecycle state and remain explicit inputs.
func (r *Repo) SetSessionRunning(ctx context.Context, tx *gorm.DB, sessionID uint, startedAt time.Time, activityExpiresAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE sessions
		SET phase = ?, started_at = ?, activity_expires_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, session.PhaseRunning, startedAt, activityExpiresAt, sessionID).Error; err != nil {
		return fmt.Errorf("setting session running: %s", err)
	}
	return nil
}

// RenewActivityDeadline extends sessionID's RUNNING-phase inactivity
// deadline, sessions.activity_expires_at, to activityExpiresAt. Called by
// every RuntimeTurn-producing RUNNING-phase
// operation once it actually commits a new Turn - deciding *whether* an
// operation counts as renewal-worthy activity is the Manager's own policy,
// not this method's.
func (r *Repo) RenewActivityDeadline(ctx context.Context, tx *gorm.DB, sessionID uint, activityExpiresAt time.Time) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE sessions
		SET activity_expires_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, activityExpiresAt, sessionID).Error; err != nil {
		return fmt.Errorf("renewing session activity deadline: %s", err)
	}
	return nil
}

// SetCurrentTurn advances sessionID's current-authoritative-RuntimeTurn
// pointer, sessions.current_turn_id. Start calls this once, creating the
// pointer for the Session's first Turn; a later RUNNING-phase caller
// advances the same column for its own committed Turn.
func (r *Repo) SetCurrentTurn(ctx context.Context, tx *gorm.DB, sessionID uint, currentTurnID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE sessions
		SET current_turn_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, currentTurnID, sessionID).Error; err != nil {
		return fmt.Errorf("setting session current turn: %s", err)
	}
	return nil
}

// ResolveSessionForClientState is an unlocked, pre-transaction read of
// sessionUUID's own sessions row - GetClientState's own read: a pure
// computation that never mutates Session state, so no row lock is needed
// the way every LOBBY/RUNNING-phase mutation's own LockSessionByUUID
// requires. Returns nil, nil if no such Session exists.
func (r *Repo) ResolveSessionForClientState(ctx context.Context, sessionUUID string) (*Session, error) {
	var row Session
	result := r.db.WithContext(ctx).Raw(`
		SELECT id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, activity_expires_at, current_turn_id, terminal_at, terminal_reason
		FROM sessions
		WHERE uuid = ?
	`, sessionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving session for client state: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// CreateSessionWithHost persists the Host/SessionActor Creation Cycle:
// inserts the sessions row with host_actor_id NULL, inserts the host
// session_actors row, then assigns host_actor_id - one cohesive structural
// invariant (a validly persisted Session always has its mandatory host
// relationship). It does not create a JoinCode; that remains a separate,
// Manager-orchestrated step within the same transaction. created_at/
// updated_at are audit timestamps the DB stamps itself; lobbyExpiresAt is
// semantic lifecycle state and remains an explicit input.
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
