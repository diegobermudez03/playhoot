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
	tx := dbServicer.GetDB().Begin(&sql.TxOptions{})
	if tx.Error != nil {
		return resultZeroValue, fmt.Errorf("openning DB transaction: %s", tx.Error)
	}

	defer tx.Rollback()
	result, err := callback(ctx, tx)
	if err != nil {
		return resultZeroValue, err
	}
	if err := tx.Commit().Error; err != nil {
		return resultZeroValue, fmt.Errorf("commiting transaction: %s", err)
	}

	return result, nil
}
