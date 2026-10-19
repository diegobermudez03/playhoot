// Package repo is storereplaychunk's persistence: the Session's replay
// manifest and the chunks registered in it.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is storereplaychunk's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Chunk is a stored replay chunk's manifest entry.
type Chunk struct {
	RecordingUUID        string
	RecordingStartedAt   time.Time
	Index                int
	FormatVersion        int
	FromSessionTimeMs    int64
	ThroughSessionTimeMs int64
	Final                bool
	ObjectKey            string
	SHA256               string
	Size                 int64
}

// RegisterChunk adds chunk to sessionUUID's replay manifest, creating it with
// the Session's pinned version; a final chunk finalizes it once. found is
// false when there is no such Session. Registering a chunk again changes nothing.
func (r *Repo) RegisterChunk(ctx context.Context, sessionUUID string, chunk Chunk, registeredAt time.Time) (found bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session struct {
			ID            uint
			GameVersionID uint
		}
		result := tx.Raw(`SELECT id, game_version_id FROM play_sessions WHERE uuid = ?`, sessionUUID).Scan(&session)
		if result.Error != nil {
			return fmt.Errorf("finding session: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		found = true
		if err := tx.Exec(`
			INSERT INTO play_replays (session_id, game_version_id, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT (session_id) DO NOTHING
		`, session.ID, session.GameVersionID, registeredAt).Error; err != nil {
			return fmt.Errorf("creating replay manifest: %w", err)
		}
		if err := tx.Exec(`
			INSERT INTO play_replay_chunks (session_id, recording_uuid, recording_started_at, chunk_index, format_version,
				from_session_time_ms, through_session_time_ms, final, object_key, sha256, size, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (recording_uuid, chunk_index) DO NOTHING
		`, session.ID, chunk.RecordingUUID, chunk.RecordingStartedAt, chunk.Index, chunk.FormatVersion,
			chunk.FromSessionTimeMs, chunk.ThroughSessionTimeMs, chunk.Final, chunk.ObjectKey, chunk.SHA256, chunk.Size, registeredAt).Error; err != nil {
			return fmt.Errorf("registering replay chunk: %w", err)
		}
		if !chunk.Final {
			return nil
		}
		if err := tx.Exec(`
			UPDATE play_replays SET final_session_time_ms = ?, finalized_at = ?
			WHERE session_id = ? AND finalized_at IS NULL
		`, chunk.ThroughSessionTimeMs, registeredAt, session.ID).Error; err != nil {
			return fmt.Errorf("finalizing replay manifest: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return found, nil
}
