package evaluateladder

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/ladder"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
)

// stepAppendAndNotify appends the move with actor automatic and leaves the
// notify task for the administrator in the same transaction.
func stepAppendAndNotify(ctx context.Context, repo *internalrepo.Repo, queue ladder.Enqueuer, tx *gorm.DB, next budget.LadderState, now time.Time) error {
	return ladder.Append(ctx, repo, queue, tx, next, now)
}
