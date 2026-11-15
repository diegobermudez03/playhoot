// Package repo persists Plan: the append-only free-limit configuration.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is Plan's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// FreeLimitConfig is a plan_free_limit_configs row.
type FreeLimitConfig struct {
	ID                 int64
	SavedGames         int
	CreationsPerWindow int
	SessionsPerWindow  int
	WindowDays         int
	PlayersPerSession  int
	SimultaneousRooms  int
	StorageMB          int
	EffectiveAt        time.Time
	CreatedByUserUUID  *string
	Reason             string
	CreatedAt          time.Time
}

func (FreeLimitConfig) TableName() string { return "plan_free_limit_configs" }

const configColumns = `id, saved_games, creations_per_window, sessions_per_window, window_days, players_per_session,
	simultaneous_rooms, storage_mb, effective_at, created_by_user_uuid, reason, created_at`

// CurrentFreeLimitConfig returns the configuration in force at now, or nil
// when none is.
func (r *Repo) CurrentFreeLimitConfig(ctx context.Context, now time.Time) (*FreeLimitConfig, error) {
	var rows []FreeLimitConfig
	err := r.db.WithContext(ctx).Raw(
		`SELECT `+configColumns+` FROM plan_free_limit_configs
		WHERE effective_at <= ? ORDER BY effective_at DESC, id DESC LIMIT 1`, now).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading current free limits: %s", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// FreeLimitConfigs returns every configuration, newest first.
func (r *Repo) FreeLimitConfigs(ctx context.Context) ([]FreeLimitConfig, error) {
	var rows []FreeLimitConfig
	err := r.db.WithContext(ctx).Raw(
		`SELECT ` + configColumns + ` FROM plan_free_limit_configs ORDER BY effective_at DESC, id DESC`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading free limit configs: %s", err)
	}
	return rows, nil
}

// InsertFreeLimitConfig appends a configuration.
func (r *Repo) InsertFreeLimitConfig(ctx context.Context, config FreeLimitConfig) error {
	if err := r.db.WithContext(ctx).Create(&config).Error; err != nil {
		return fmt.Errorf("inserting free limit config: %s", err)
	}
	return nil
}
