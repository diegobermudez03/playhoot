// Package repo is recordgamefault's persistence: it stores game fault reports
// and decides nothing itself.
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

// Fault is one game fault report.
type Fault struct {
	UserUUID      string
	ParticipantID string
	Kind          string
	Detail        []byte
}

// RecordFault stores fault for sessionUUID only while that Session is
// RUNNING and the reporter is its participant under that participant id; a
// report of a kind the reporter already made in the Session counts it again
// and keeps the newer detail. recorded is false when nothing was stored.
func (r *Repo) RecordFault(ctx context.Context, sessionUUID string, fault Fault, reportedAt time.Time) (recorded bool, err error) {
	result := r.db.WithContext(ctx).Exec(`
		INSERT INTO play_game_faults (session_id, game_version_id, user_uuid, participant_id, source, kind, detail,
			occurrences, first_reported_at, last_reported_at)
		SELECT s.id, s.game_version_id, p.user_uuid, p.participant_id, 'CLIENT', ?, ?, 1, ?, ?
		FROM play_sessions s
		JOIN play_session_participants p ON p.session_id = s.id
		WHERE s.uuid = ? AND s.status = 'RUNNING' AND p.user_uuid = ? AND p.participant_id = ?
		ON CONFLICT (session_id, user_uuid, kind) DO UPDATE SET
			occurrences = play_game_faults.occurrences + 1,
			detail = EXCLUDED.detail,
			last_reported_at = EXCLUDED.last_reported_at
	`, fault.Kind, string(fault.Detail), reportedAt, reportedAt, sessionUUID, fault.UserUUID, fault.ParticipantID)
	if result.Error != nil {
		return false, fmt.Errorf("recording game fault: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}
