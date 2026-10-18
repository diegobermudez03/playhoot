package repo

import (
	"context"
	"fmt"
)

// RunningSessionRecord is a Session with its game version's script locator
// and its latest checkpoint. Locator and checkpoint fields are nil when it
// has none.
type RunningSessionRecord struct {
	ID                 uint
	Status             string
	GameVersionUUID    string
	ScriptObjectKey    *string
	ScriptSHA256       *string
	ScriptSize         *int64
	CheckpointRevision *int64
	CheckpointContent  []byte
}

// FindRunningSession returns sessionUUID with its script locator and latest
// checkpoint, or nil when there is no such Session.
func (r *Repo) FindRunningSession(ctx context.Context, sessionUUID string) (*RunningSessionRecord, error) {
	var record RunningSessionRecord
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.id, s.status, v.uuid AS game_version_uuid,
			v.script_object_key, v.script_sha256, v.script_size,
			c.context_revision AS checkpoint_revision, c.content AS checkpoint_content
		FROM play_sessions s
		JOIN play_game_versions v ON v.id = s.game_version_id
		LEFT JOIN play_session_checkpoints c ON c.session_id = s.id
		WHERE s.uuid = ?
	`, sessionUUID).Scan(&record)
	if result.Error != nil {
		return nil, fmt.Errorf("finding running session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &record, nil
}

// FindParticipants returns sessionID's players in the order they were
// recorded.
func (r *Repo) FindParticipants(ctx context.Context, sessionID uint) ([]Participant, error) {
	var participants []Participant
	result := r.db.WithContext(ctx).Raw(`
		SELECT user_uuid, participant_id
		FROM play_session_participants
		WHERE session_id = ?
		ORDER BY id
	`, sessionID).Scan(&participants)
	if result.Error != nil {
		return nil, fmt.Errorf("finding participants: %w", result.Error)
	}
	return participants, nil
}
