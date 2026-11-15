// Package ladder appends a move of the restriction ladder and, in the same
// transaction, leaves the notify task that tells the administrator.
package ladder

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/budget/internal/snapshot"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// NotificationType is the notify type of a ladder move.
const NotificationType = "budget-level-changed"

// RecipientAdmin addresses a notification to the administrator.
const RecipientAdmin = "admin"

var (
	levelLabel = metrics.NewLabel("level",
		string(budget.LevelNormal), string(budget.LevelRestricted), string(budget.LevelPaused), string(budget.LevelShutdown))
	actorLabel = metrics.NewLabel("actor", budget.ActorAutomatic, "admin")
	movesTotal = metrics.NewCounter(
		"budget_ladder_moves_total",
		"Restriction ladder moves by the level reached and who moved it.",
		levelLabel, actorLabel,
	)
)

// Enqueuer leaves a task in Budget's queue inside a transaction.
type Enqueuer interface {
	Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error
}

// Notification is the notify task payload: one logical entry for the
// recipient, keyed so a retried task never duplicates it.
type Notification struct {
	Recipient      string         `json:"recipient"`
	Type           string         `json:"type"`
	IdempotencyKey string         `json:"idempotency_key"`
	ExternalToken  string         `json:"external_token"`
	Necessary      bool           `json:"necessary"`
	Skeleton       map[string]any `json:"skeleton"`
}

// Append records state as the ladder's new current row at at, inside tx
// (which must hold the budget lock), and leaves its notify task.
func Append(ctx context.Context, repo *internalrepo.Repo, queue Enqueuer, tx *gorm.DB, state budget.LadderState, at time.Time) error {
	id, err := repo.InsertChange(ctx, tx, snapshot.ChangeRow(state, at))
	if err != nil {
		return err
	}
	key := fmt.Sprintf("ladder-change:%d", id)
	payload := Notification{
		Recipient: RecipientAdmin, Type: NotificationType, IdempotencyKey: key, ExternalToken: key, Necessary: true,
		Skeleton: map[string]any{
			"level": state.Level, "paused_categories": state.PausedCategories, "reopen_closed": state.ReopenClosed,
			"reason": state.Reason, "actor": state.Actor,
		},
	}
	if err := queue.Enqueue(tx, txqueue.TypeNotify, payload, time.Time{}, key); err != nil {
		return err
	}
	actor := budget.ActorAutomatic
	if state.Actor != budget.ActorAutomatic {
		actor = "admin"
	}
	movesTotal.Inc(ctx, levelLabel.Value(string(state.Level)), actorLabel.Value(actor))
	return nil
}
