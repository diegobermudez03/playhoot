// Package repo is recordgamefault's persistence: it reads who reports and
// stores fault reports, and decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is recordgamefault's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Reporter is the Session a report is about, and the reporting user's
// participant id in it (nil when they are not its participant).
type Reporter struct {
	SessionID     int64
	GameVersionID int64
	Status        string
	ParticipantID *string
}

// FindReporter returns the Session with sessionUUID and userUUID's place in
// it, or nil when there is no such Session.
func (r *Repo) FindReporter(ctx context.Context, sessionUUID string, userUUID string) (*Reporter, error) {
	var reporters []Reporter
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.id AS session_id, s.game_version_id, s.status, p.participant_id
		FROM play_sessions s
		LEFT JOIN play_session_participants p ON p.session_id = s.id AND p.user_uuid = ?
		WHERE s.uuid = ?`,
		userUUID, sessionUUID,
	).Scan(&reporters).Error
	if err != nil {
		return nil, fmt.Errorf("finding the reporter of a game fault: %w", err)
	}
	if len(reporters) == 0 {
		return nil, nil
	}
	return &reporters[0], nil
}

// Report is one game fault report, as stored.
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

// CreateReport stores report.
func (r *Repo) CreateReport(ctx context.Context, report Report) error {
	if err := r.db.WithContext(ctx).Create(&report).Error; err != nil {
		return fmt.Errorf("recording game fault: %w", err)
	}
	return nil
}
