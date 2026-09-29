package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// runtimeFailureInsert is the persisted shape of one session_runtime_failures
// row - the durable fatal-diagnostic record for a RuntimeTurn attempt that
// never committed. There is no failed_step_index column: the Executor's
// current contract exposes no per-step failure position, and that field is
// explicitly optional.
type runtimeFailureInsert struct {
	ID                      uint   `gorm:"column:id"`
	SessionID               uint   `gorm:"column:session_id"`
	FailureKind             string `gorm:"column:failure_kind"`
	ErrorCode               string `gorm:"column:error_code"`
	ErrorMessage            string `gorm:"column:error_message"`
	BaseTurnID              *uint  `gorm:"column:base_turn_id"`
	AttemptedSequence       uint64 `gorm:"column:attempted_sequence"`
	SourceKind              string `gorm:"column:source_kind"`
	SourceTimerObligationID *uint  `gorm:"column:source_timer_obligation_id"`
	ActorID                 *uint  `gorm:"column:actor_id"`
	DiagnosticPayload       []byte `gorm:"column:diagnostic_payload"`
}

func (runtimeFailureInsert) TableName() string { return "session_runtime_failures" }

// CreateRuntimeFailure persists one fatal RuntimeTurn attempt's diagnostic
// record, as part of the same atomic materialization that terminalizes the
// Session - baseTurnID/attemptedSequence identify the last
// committed Turn this attempt was made against (baseTurnID nil,
// attemptedSequence 1, for a pre-first-Turn Start failure), and
// sourceKind/sourceTimerObligationID/actorID identify the driving cause,
// mirroring the same fields CreateRuntimeTurn records for a successful Turn.
func (r *Repo) CreateRuntimeFailure(ctx context.Context, tx *gorm.DB, sessionID uint, failureKind string, errorCode string, errorMessage string, baseTurnID *uint, attemptedSequence uint64, sourceKind string, sourceTimerObligationID *uint, actorID *uint, diagnosticPayload []byte) error {
	row := runtimeFailureInsert{
		SessionID:               sessionID,
		FailureKind:             failureKind,
		ErrorCode:               errorCode,
		ErrorMessage:            errorMessage,
		BaseTurnID:              baseTurnID,
		AttemptedSequence:       attemptedSequence,
		SourceKind:              sourceKind,
		SourceTimerObligationID: sourceTimerObligationID,
		ActorID:                 actorID,
		DiagnosticPayload:       diagnosticPayload,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating runtime failure: %s", err)
	}
	return nil
}
