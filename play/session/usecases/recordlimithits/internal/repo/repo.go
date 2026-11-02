// Package repo is recordlimithits's persistence: it reads the Sessions limit
// hits belong to and stores their reports, and decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is recordlimithits's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB returns the database, for transactions.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// Target is the Session a hit belongs to, and the user's participant id in it
// (nil when they are not its participant).
type Target struct {
	SessionID     int64
	GameVersionID int64
	ParticipantID *string
}

// FindTarget returns, within tx, the Session with sessionUUID and userUUID's
// place in it, or nil when there is no such Session.
func (r *Repo) FindTarget(ctx context.Context, tx *gorm.DB, sessionUUID string, userUUID string) (*Target, error) {
	var targets []Target
	err := tx.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, s.game_version_id, p.participant_id
		FROM play_sessions s
		LEFT JOIN play_session_participants p ON p.session_id = s.id AND p.user_uuid = ?
		WHERE s.uuid = ?`,
		userUUID, sessionUUID,
	).Scan(&targets).Error
	if err != nil {
		return nil, fmt.Errorf("finding the Session of a limit hit: %w", err)
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return &targets[0], nil
}

// Report is one limit's hits in one audit flush, as stored.
type Report struct {
	ID            int64
	SessionID     int64
	GameVersionID int64
	UserUUID      string
	ParticipantID *string
	Source        string
	Kind          string
	Detail        string
	Occurrences   int
	ReportedAt    time.Time
}

func (Report) TableName() string { return "play_game_fault_reports" }

// CreateReport stores report within tx.
func (r *Repo) CreateReport(ctx context.Context, tx *gorm.DB, report Report) error {
	if err := tx.WithContext(ctx).Create(&report).Error; err != nil {
		return fmt.Errorf("recording limit hits: %w", err)
	}
	return nil
}
