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

func RunInDBTransaction[T any](ctx context.Context, dbServicer DBServicer, callback func(ctx context.Context, tx *gorm.DB) (T, error)) (T, error) {
	var resultZeroValue T
	tx := dbServicer.GetDB().WithContext(ctx).Begin(&sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
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
