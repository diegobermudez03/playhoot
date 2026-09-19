package utils

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"
)

type DBServicer interface {
	GetDB() *gorm.DB
}

// RunInDBTransaction runs callback inside one atomic transaction at Read
// Committed isolation. Read Committed is the deliberate choice for callers
// that serialize concurrent mutations by explicitly locking a row (e.g.
// SELECT ... FOR UPDATE) and then reading/writing other tables based on that
// lock: Read Committed gives every statement a fresh per-statement snapshot,
// so a statement that runs after acquiring a lock actually observes what
// every prior holder of that lock committed. A stricter level (Repeatable
// Read/Serializable) instead pins one snapshot for the whole transaction,
// which a lock wait does not advance for any table other than the specific
// row being locked - silently reintroducing the exact race the lock was
// meant to prevent for any other table the transaction reads afterward.
func RunInDBTransaction[T any](ctx context.Context, dbServicer DBServicer, callback func(ctx context.Context, tx *gorm.DB) (T, error)) (T, error) {
	var resultZeroValue T
	tx := dbServicer.GetDB().WithContext(ctx).Begin(&sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if tx.Error != nil {
		return resultZeroValue, fmt.Errorf("opening DB transaction: %w", tx.Error)
	}

	defer tx.Rollback()
	result, err := callback(ctx, tx)
	if err != nil {
		return resultZeroValue, err
	}
	if err := tx.Commit().Error; err != nil {
		return resultZeroValue, fmt.Errorf("committing transaction: %w", err)
	}

	return result, nil
}
