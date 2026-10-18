package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// StartCandidate is what deciding whether a Session may start needs: the
// Session and the game version it is pinned to. The script locator is nil
// when the version has none.
type StartCandidate struct {
	Status            string
	CreatedByUserUUID string
	LobbyExpiresAt    time.Time
	GameVersionUUID   string
	VersionStatus     string
	Manifest          []byte
	ScriptObjectKey   *string
	ScriptSHA256      *string
	ScriptSize        *int64
}

// LockedSession is a Session row locked for the rest of the transaction.
type LockedSession struct {
	ID                uint
	Status            string
	CreatedByUserUUID string
	LobbyExpiresAt    time.Time
}

// Participant is one player of a started Session.
type Participant struct {
	UserUUID      string
	ParticipantID string
}

// FindStartCandidate returns sessionUUID with its game version, or nil when
// there is no such Session.
func (r *Repo) FindStartCandidate(ctx context.Context, sessionUUID string) (*StartCandidate, error) {
	var candidate StartCandidate
	result := r.db.WithContext(ctx).Raw(`
		SELECT s.status, s.created_by_user_uuid, s.lobby_expires_at,
			v.uuid AS game_version_uuid, v.status AS version_status, v.manifest,
			v.script_object_key, v.script_sha256, v.script_size
		FROM play_sessions s
		JOIN play_game_versions v ON v.id = s.game_version_id
		WHERE s.uuid = ?
	`, sessionUUID).Scan(&candidate)
	if result.Error != nil {
		return nil, fmt.Errorf("finding start candidate: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &candidate, nil
}

// LockSession locks sessionUUID's row until tx ends and returns it, or nil
// when there is no such Session.
func (r *Repo) LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string) (*LockedSession, error) {
	var locked LockedSession
	result := tx.WithContext(ctx).Raw(`
		SELECT id, status, created_by_user_uuid, lobby_expires_at
		FROM play_sessions
		WHERE uuid = ?
		FOR UPDATE
	`, sessionUUID).Scan(&locked)
	if result.Error != nil {
		return nil, fmt.Errorf("locking session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &locked, nil
}

// CreateParticipants records participants as sessionID's players.
func (r *Repo) CreateParticipants(ctx context.Context, tx *gorm.DB, sessionID uint, participants []Participant) error {
	for _, participant := range participants {
		result := tx.WithContext(ctx).Exec(`
			INSERT INTO play_session_participants (session_id, user_uuid, participant_id)
			VALUES (?, ?, ?)
		`, sessionID, participant.UserUUID, participant.ParticipantID)
		if result.Error != nil {
			return fmt.Errorf("inserting participant: %w", result.Error)
		}
	}
	return nil
}

// SaveCheckpoint makes content sessionID's latest checkpoint, unless the one
// stored is of a newer context revision.
func (r *Repo) SaveCheckpoint(ctx context.Context, tx *gorm.DB, sessionID uint, contextRevision int64, content []byte, savedAt time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		INSERT INTO play_session_checkpoints (session_id, context_revision, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE
		SET context_revision = EXCLUDED.context_revision, content = EXCLUDED.content, updated_at = EXCLUDED.updated_at
		WHERE play_session_checkpoints.context_revision < EXCLUDED.context_revision
	`, sessionID, contextRevision, content, savedAt, savedAt)
	if result.Error != nil {
		return fmt.Errorf("saving checkpoint: %w", result.Error)
	}
	return nil
}

// ChangeSessionStatus moves sessionID from fromStatus to toStatus, recording
// endCause when it is not nil, and appends the change to its history.
// changedBy is nil when the platform makes the change. It fails when the
// Session is not in fromStatus.
func (r *Repo) ChangeSessionStatus(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_sessions
		SET status = ?, end_cause = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`, toStatus, endCause, changedAt, sessionID, fromStatus)
	if result.Error != nil {
		return fmt.Errorf("changing session status: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("session %d not found in status %s", sessionID, fromStatus)
	}
	return recordStatusChange(ctx, tx, sessionID, &fromStatus, toStatus, endCause, changedBy, changedAt)
}

// ReleaseJoinCodes releases every active join code of sessionID.
func (r *Repo) ReleaseJoinCodes(ctx context.Context, tx *gorm.DB, sessionID uint, releasedAt time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_session_join_codes
		SET released_at = ?
		WHERE session_id = ? AND released_at IS NULL
	`, releasedAt, sessionID)
	if result.Error != nil {
		return fmt.Errorf("releasing join codes: %w", result.Error)
	}
	return nil
}
