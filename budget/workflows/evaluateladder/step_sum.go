package evaluateladder

import (
	"context"
	"time"

	"gorm.io/gorm"

	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/budget/internal/snapshot"
)

// stepSum sums each window per category and global: measured, estimated
// and reserved, plus the categories missing rates.
func stepSum(ctx context.Context, repo *internalrepo.Repo, tx *gorm.DB, now time.Time) (snapshot.Snapshot, error) {
	return snapshot.Load(ctx, repo, tx, now)
}
