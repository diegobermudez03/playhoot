package repo

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Interaction is the persisted session_interactions record for one opened
// question/ask-group instance.
type Interaction struct {
	ID              uint
	UUID            string
	SessionID       uint
	SessionActorID  uint
	Kind            string
	EnginePath      []byte
	EngineSlot      string
	ResponsePayload []byte
	State           string
	OpenedByTurnID  uint
	ClosedByTurnID  *uint
}

// ResolveSessionForInteraction is an unlocked, pre-transaction lookup: it
// lets the Manager learn which Session to lock before opening the mutation
// transaction, without itself committing to the interaction row's full
// current state - that is re-read under lock, by FindInteractionByUUID,
// once serialization is actually acquired.
func (r *Repo) ResolveSessionForInteraction(ctx context.Context, interactionUUID string) (*uint, error) {
	var sessionID uint
	result := r.db.WithContext(ctx).Raw(`
		SELECT session_id FROM session_interactions WHERE uuid = ?
	`, interactionUUID).Scan(&sessionID)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving session for interaction: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &sessionID, nil
}

// FindInteractionByUUID re-reads interactionUUID's full current row inside
// an already-open transaction. Safe as a plain (non-FOR-UPDATE) read once
// the owning Session's row lock is held: every writer of a session's
// session_interactions rows acquires that same Session-row lock first, so
// no concurrent writer can be mid-mutation against this row while the
// caller holds it.
func (r *Repo) FindInteractionByUUID(ctx context.Context, tx *gorm.DB, interactionUUID string) (*Interaction, error) {
	var row Interaction
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, session_id, session_actor_id, kind, engine_path, engine_slot,
			response_payload, state, opened_by_turn_id, closed_by_turn_id
		FROM session_interactions
		WHERE uuid = ?
	`, interactionUUID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("finding interaction: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

// GetInteractionByID loads interactionID's persisted facts by internal id -
// the id a RuntimeTurn's own source_interaction_id references - for
// rebuilding the engine.Signal that interaction's response drove, during
// replay reconstruction.
func (r *Repo) GetInteractionByID(ctx context.Context, tx *gorm.DB, interactionID uint) (*Interaction, error) {
	var row Interaction
	result := tx.WithContext(ctx).Raw(`
		SELECT id, uuid, session_id, session_actor_id, kind, engine_path, engine_slot,
			response_payload, state, opened_by_turn_id, closed_by_turn_id
		FROM session_interactions
		WHERE id = ?
	`, interactionID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("getting interaction: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

type interactionInsert struct {
	ID                 uint   `gorm:"column:id"`
	UUID               string `gorm:"column:uuid"`
	SessionID          uint   `gorm:"column:session_id"`
	SessionActorID     uint   `gorm:"column:session_actor_id"`
	Kind               string `gorm:"column:kind"`
	EnginePath         []byte `gorm:"column:engine_path"`
	EngineSlot         string `gorm:"column:engine_slot"`
	InteractionPayload []byte `gorm:"column:interaction_payload"`
	State              string `gorm:"column:state"`
	OpenedByTurnID     uint   `gorm:"column:opened_by_turn_id"`
}

func (interactionInsert) TableName() string { return "session_interactions" }

// CreateInteraction persists a newly opened ACTIVE interaction, captured
// from an engine.OpenQuestionOutput belonging to the committed RuntimeTurn
// openedByTurnID.
func (r *Repo) CreateInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, sessionActorID uint, kind string, enginePath []byte, engineSlot string, interactionPayload []byte, openedByTurnID uint) (uint, error) {
	row := interactionInsert{
		UUID:               uuid.NewString(),
		SessionID:          sessionID,
		SessionActorID:     sessionActorID,
		Kind:               kind,
		EnginePath:         enginePath,
		EngineSlot:         engineSlot,
		InteractionPayload: interactionPayload,
		State:              session.InteractionStateActive,
		OpenedByTurnID:     openedByTurnID,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating interaction: %s", err)
	}
	return row.ID, nil
}

// CloseActiveInteraction closes the currently ACTIVE interaction matching
// (sessionID, enginePath, engineSlot, sessionActorID) - the same key an
// engine.CloseQuestionOutput's (Path, Slot, Recipient) identifies - as
// Turn-produced closure. A CloseQuestionOutput with no currently-ACTIVE
// match is a no-op (CloseQuestionOperation's own documented "closing an
// already empty slot is an idempotent no-op").
func (r *Repo) CloseActiveInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, enginePath []byte, engineSlot string, sessionActorID uint, closedByTurnID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_interactions
		SET state = ?, closed_by_turn_id = ?
		WHERE session_id = ? AND engine_path = ?::jsonb AND engine_slot = ? AND session_actor_id = ? AND state = ?
	`, session.InteractionStateClosed, closedByTurnID, sessionID, enginePath, engineSlot, sessionActorID, session.InteractionStateActive).Error; err != nil {
		return fmt.Errorf("closing interaction: %s", err)
	}
	return nil
}

// CloseAnsweredInteraction closes interactionID directly by id and persists
// its accepted response payload, as Turn-produced closure. This is separate
// from CloseActiveInteraction's (Path, Slot, Recipient)-matched closure:
// engineservice.Step clears an accepted answer's own slot internally,
// before the transition's own authored operations run, and never produces a
// CloseQuestionOutput for that closure - so the one specific interaction
// AnswerInteraction targeted must be closed directly by its already-known
// id, not discovered via captured Outputs.
func (r *Repo) CloseAnsweredInteraction(ctx context.Context, tx *gorm.DB, interactionID uint, responsePayload []byte, closedByTurnID uint) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_interactions SET state = ?, response_payload = ?, closed_by_turn_id = ? WHERE id = ?
	`, session.InteractionStateClosed, responsePayload, closedByTurnID, interactionID).Error; err != nil {
		return fmt.Errorf("closing answered interaction: %s", err)
	}
	return nil
}

// CloseAllActiveInteractionsForSession atomically closes every still-ACTIVE
// interaction for sessionID as terminal cleanup: closed_by_turn_id stays
// NULL and closure_reason records reason, since this is Session lifecycle
// cleanup, not gameplay - no RuntimeTurn ever caused it.
func (r *Repo) CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error {
	if err := tx.WithContext(ctx).Exec(`
		UPDATE session_interactions
		SET state = ?, closure_reason = ?
		WHERE session_id = ? AND state = ?
	`, session.InteractionStateTerminated, reason, sessionID, session.InteractionStateActive).Error; err != nil {
		return fmt.Errorf("closing active interactions for session: %s", err)
	}
	return nil
}
