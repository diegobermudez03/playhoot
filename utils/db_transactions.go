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

// RunInDBTransaction runs callback in one transaction at Read Committed. Each
// statement sees a fresh snapshot, so reads after acquiring a row lock see
// what earlier lock holders committed; a stricter level would pin the snapshot
// and reintroduce the race the lock prevents.
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
