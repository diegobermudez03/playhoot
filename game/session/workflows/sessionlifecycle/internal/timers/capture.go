// Package timers durably records a committed RuntimeTurn's
// engine.ScheduleTimerOutput/CancelTimerOutput/ScheduleKeyedTimerOutput/
// CancelKeyedTimerOutput values as session_timer_obligations rows, shared by
// every sessionlifecycle Turn-producing step so every path captures timer
// obligations the same way - mirroring internal/interactions' role for
// questions exactly.
package timers

import (
	"context"
	"fmt"
	"math"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"gorm.io/gorm"
)

// CaptureRepo is the narrow persistence contract Capture needs.
type CaptureRepo interface {
	CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, engineSlot string, engineKey []byte, delayMs int64, createdByTurnID uint) (uint, error)
	CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, engineSlot string, engineKey []byte, closedByTurnID uint) error
}

// Capture durably records every engine.ScheduleTimerOutput/CancelTimerOutput/
// ScheduleKeyedTimerOutput/CancelKeyedTimerOutput produced by a just-processed
// RuntimeTurn as session_timer_obligations rows created/closed by turnID.
// Every step that persists a RuntimeTurn must call this, alongside
// interactions.Capture, so a timer a Turn schedules or cancels is never left
// unrecorded.
//
// outputs is the flat, ordered Output list engineservice.StartTurn/
// AdvanceTurn already return for one Turn.
func Capture(ctx context.Context, tx *gorm.DB, repo CaptureRepo, sessionID uint, turnID uint, outputs []engine.Output) error {
	for _, output := range outputs {
		switch o := output.(type) {
		case engine.ScheduleTimerOutput:
			delayMs, err := encodeDelay(o.DelayMilliseconds)
			if err != nil {
				return err
			}
			if _, err := repo.CreateTimerObligation(ctx, tx, sessionID, o.Slot, nil, delayMs, turnID); err != nil {
				return err
			}
		case engine.ScheduleKeyedTimerOutput:
			delayMs, err := encodeDelay(o.DelayMilliseconds)
			if err != nil {
				return err
			}
			encodedKey, err := engineservice.EncodeValue(o.Key)
			if err != nil {
				return fmt.Errorf("encoding keyed timer key: %s", err)
			}
			if _, err := repo.CreateTimerObligation(ctx, tx, sessionID, o.Slot, encodedKey, delayMs, turnID); err != nil {
				return err
			}
		case engine.CancelTimerOutput:
			// Only ever produced when a pending timer actually existed to
			// cancel - CancelTimerOperation is idempotent when the slot is
			// already empty and produces no Output in that case, so
			// CancelActiveTimerObligation's own no-op-if-absent behavior is
			// defense-in-depth, not the primary correctness mechanism here.
			if err := repo.CancelActiveTimerObligation(ctx, tx, sessionID, o.Slot, nil, turnID); err != nil {
				return err
			}
		case engine.CancelKeyedTimerOutput:
			encodedKey, err := engineservice.EncodeValue(o.Key)
			if err != nil {
				return fmt.Errorf("encoding keyed timer key: %s", err)
			}
			if err := repo.CancelActiveTimerObligation(ctx, tx, sessionID, o.Slot, encodedKey, turnID); err != nil {
				return err
			}
		}
	}
	return nil
}

// encodeDelay converts the engine's own DelayMilliseconds (a float64 the
// compiler/engine already guarantee is a finite, non-negative integer number
// of milliseconds - see program.ScheduleTimerOperation's doc comment) into
// session_timer_obligations.delay_ms's integer representation.
func encodeDelay(delayMilliseconds float64) (int64, error) {
	if delayMilliseconds < 0 || math.Trunc(delayMilliseconds) != delayMilliseconds {
		return 0, fmt.Errorf("timer delay %v is not a non-negative integer number of milliseconds", delayMilliseconds)
	}
	return int64(delayMilliseconds), nil
}
