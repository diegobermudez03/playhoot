package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// runtimeStartInsert is the persisted shape of session_runtime_starts - the
// one-row-per-Session durable record of Start's deterministic
// initialization. Seed is stored as InitializationInput.Seed's uint64 bit
// pattern reinterpreted as int64 (see the owning migration's doc comment);
// the conversion is exact and lossless in both directions.
type runtimeStartInsert struct {
	ID             uint   `gorm:"column:id"`
	SessionID      uint   `gorm:"column:session_id"`
	Seed           int64  `gorm:"column:seed"`
	RootParameters []byte `gorm:"column:root_parameters"`
}

func (runtimeStartInsert) TableName() string { return "session_runtime_starts" }

// CreateRuntimeStart persists Start's Seed/RootParameters. Call this in the
// same transaction as Start's Turn 1, since this is the one moment the
// actual Start-time roster is known - a Participant's slot can later be
// released and reoccupied by a rejoin, so this durable record must not be
// reconstructed from current Participant state afterward. rootParameters is
// already encoded through engineservice.EncodeValue.
func (r *Repo) CreateRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint, seed uint64, rootParameters []byte) error {
	row := runtimeStartInsert{
		SessionID:      sessionID,
		Seed:           int64(seed),
		RootParameters: rootParameters,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating runtime start: %s", err)
	}
	return nil
}

// RuntimeStart is sessionID's persisted Start-initialization facts.
type RuntimeStart struct {
	Seed           uint64
	RootParameters []byte
}

// GetRuntimeStart loads sessionID's persisted Start-initialization facts,
// the deterministic replay origin every RuntimeTurn is folded onto.
func (r *Repo) GetRuntimeStart(ctx context.Context, tx *gorm.DB, sessionID uint) (*RuntimeStart, error) {
	var row struct {
		Seed           int64  `gorm:"column:seed"`
		RootParameters []byte `gorm:"column:root_parameters"`
	}
	result := tx.WithContext(ctx).Raw(`
		SELECT seed, root_parameters
		FROM session_runtime_starts
		WHERE session_id = ?
	`, sessionID).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("getting runtime start: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &RuntimeStart{Seed: uint64(row.Seed), RootParameters: row.RootParameters}, nil
}
