// Package timers persists the durable timer-obligation consequences of a
// committed RuntimeTurn's parsed platform Commands, shared by every
// RuntimeTurn-producing sessionlifecycle step so each reacts to a
// ScheduleTimer/CancelTimer the same way.
package timers

import (
	"context"

	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	"gorm.io/gorm"
)

// repoAPI is Apply's own narrow persistence contract.
type repoAPI interface {
	CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, data []byte, delayMs int64, createdByTurnID uint) (uint, error)
	CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, timer string, closedByTurnID uint) error
}

// Apply scans commands (already parsed via platform.ParseCommand) for every
// ScheduleTimer/CancelTimer and persists its effect against sessionID,
// stamped with the already-committed RuntimeTurn turnID.
//
// A ScheduleTimer naming a Timer identifier that already has an ACTIVE
// obligation replaces it (the existing one is cancelled, then the new one
// is created) rather than being rejected: the platform keeps no semantic
// memory of what a timer means, so it cannot tell an intentional
// reschedule (a renewable timeout) from an authoring mistake, and picks
// the generally useful behavior. A CancelTimer naming a Timer identifier
// with no matching ACTIVE obligation is an ordinary no-op, mirroring that
// command's own documented contract.
func Apply(ctx context.Context, tx *gorm.DB, repo repoAPI, sessionID uint, turnID uint, commands []platform.Command) error {
	for _, command := range commands {
		switch c := command.(type) {
		case *platform.ScheduleTimer:
			if err := repo.CancelActiveTimerObligation(ctx, tx, sessionID, c.Timer, turnID); err != nil {
				return err
			}
			if _, err := repo.CreateTimerObligation(ctx, tx, sessionID, c.Timer, c.Data, c.DelayMS, turnID); err != nil {
				return err
			}
		case *platform.CancelTimer:
			if err := repo.CancelActiveTimerObligation(ctx, tx, sessionID, c.Timer, turnID); err != nil {
				return err
			}
		}
	}
	return nil
}
