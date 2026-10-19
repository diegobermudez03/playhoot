package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateSessionRequest is a claimed Session-creation idempotency request.
// Outcome and SessionID are nil until the request is completed.
type CreateSessionRequest struct {
	ID        uint
	GameUUID  string
	Outcome   *string
	SessionID *uint
}

// GameVersion is the version a Game currently points to.
type GameVersion struct {
	ID     uint
	Status string
}

// CreatedSession is a Session that was just inserted.
type CreatedSession struct {
	ID   uint
	UUID string
}

// SessionWithJoinCode is a Session together with the first join code it was
// given, which is what its creation answered with.
type SessionWithJoinCode struct {
	UUID           string
	JoinCode       string
	LobbyExpiresAt time.Time
}

type session struct {
	ID                uint
	UUID              string
	GameVersionID     uint
	CreatedByUserUUID string
	Status            string
	LobbyExpiresAt    time.Time
	ExpiresAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (session) TableName() string { return "play_sessions" }

// ClaimCreateSessionRequest claims (userUUID, idempotencyKey), or returns the
// request that already holds it. A concurrent claim of the same identity waits
// for the first transaction and then sees its request.
func (r *Repo) ClaimCreateSessionRequest(ctx context.Context, tx *gorm.DB, userUUID string, idempotencyKey string, gameUUID string) (uint, *CreateSessionRequest, error) {
	var claimedID uint
	claim := tx.WithContext(ctx).Raw(`
		INSERT INTO play_create_session_requests (user_uuid, idempotency_key, game_uuid)
		VALUES (?, ?, ?)
		ON CONFLICT (user_uuid, idempotency_key) DO NOTHING
		RETURNING id
	`, userUUID, idempotencyKey, gameUUID).Scan(&claimedID)
	if claim.Error != nil {
		return 0, nil, fmt.Errorf("claiming create session request: %s", claim.Error)
	}
	if claim.RowsAffected == 1 {
		return claimedID, nil, nil
	}

	var existing CreateSessionRequest
	read := tx.WithContext(ctx).Raw(`
		SELECT id, game_uuid, outcome, session_id
		FROM play_create_session_requests
		WHERE user_uuid = ? AND idempotency_key = ?
	`, userUUID, idempotencyKey).Scan(&existing)
	if read.Error != nil {
		return 0, nil, fmt.Errorf("reading existing create session request: %s", read.Error)
	}
	if read.RowsAffected != 1 {
		return 0, nil, fmt.Errorf("create session request was neither claimed nor found")
	}
	return 0, &existing, nil
}

// CompleteCreateSessionRequest records the outcome of a claimed request, and
// the Session it created when there is one.
func (r *Repo) CompleteCreateSessionRequest(ctx context.Context, tx *gorm.DB, requestID uint, outcome string, sessionID *uint) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_create_session_requests
		SET outcome = ?, session_id = ?
		WHERE id = ?
	`, outcome, sessionID, requestID)
	if result.Error != nil {
		return fmt.Errorf("completing create session request: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("create session request %d not found", requestID)
	}
	return nil
}

// FindCurrentGameVersion returns the version gameUUID currently points to,
// or nil when the Game is unknown or points to no version.
func (r *Repo) FindCurrentGameVersion(ctx context.Context, tx *gorm.DB, gameUUID string) (*GameVersion, error) {
	var version GameVersion
	result := tx.WithContext(ctx).Raw(`
		SELECT v.id, v.status
		FROM play_games g
		JOIN play_game_versions v ON v.id = g.current_version_id
		WHERE g.game_uuid = ?
	`, gameUUID).Scan(&version)
	if result.Error != nil {
		return nil, fmt.Errorf("finding current game version: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &version, nil
}

// CreateSession inserts a Session pinned to gameVersionID, with a freshly
// generated UUID, whose lobby expires at lobbyExpiresAt and itself at
// expiresAt, and starts its status history.
func (r *Repo) CreateSession(ctx context.Context, tx *gorm.DB, gameVersionID uint, createdByUserUUID string, status string, lobbyExpiresAt time.Time, expiresAt time.Time, createdAt time.Time) (CreatedSession, error) {
	created := session{
		UUID:              uuid.NewString(),
		GameVersionID:     gameVersionID,
		CreatedByUserUUID: createdByUserUUID,
		Status:            status,
		LobbyExpiresAt:    lobbyExpiresAt,
		ExpiresAt:         expiresAt,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
	if err := tx.WithContext(ctx).Create(&created).Error; err != nil {
		return CreatedSession{}, fmt.Errorf("inserting session: %s", err)
	}
	if err := recordStatusChange(ctx, tx, created.ID, nil, status, nil, &createdByUserUUID, createdAt); err != nil {
		return CreatedSession{}, err
	}
	return CreatedSession{ID: created.ID, UUID: created.UUID}, nil
}

// CreateJoinCode gives sessionID the join code code, active until it is
// released. It reports false, without failing, when another active join code
// already holds code.
func (r *Repo) CreateJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, code string, expiresAt time.Time) (bool, error) {
	var id uint
	result := tx.WithContext(ctx).Raw(`
		INSERT INTO play_session_join_codes (session_id, code, expires_at)
		VALUES (?, ?, ?)
		ON CONFLICT (code) WHERE released_at IS NULL DO NOTHING
		RETURNING id
	`, sessionID, code, expiresAt).Scan(&id)
	if result.Error != nil {
		return false, fmt.Errorf("inserting join code: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// ReleaseExpiredJoinCode releases the active join code code if it expired at
// or before releasedAt, reporting whether it did.
func (r *Repo) ReleaseExpiredJoinCode(ctx context.Context, tx *gorm.DB, code string, releasedAt time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_session_join_codes
		SET released_at = ?
		WHERE code = ? AND released_at IS NULL AND expires_at <= ?
	`, releasedAt, code, releasedAt)
	if result.Error != nil {
		return false, fmt.Errorf("releasing expired join code: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// FindSessionWithJoinCode returns sessionID together with the first join
// code it was given, or nil when there is no such Session or join code.
func (r *Repo) FindSessionWithJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint) (*SessionWithJoinCode, error) {
	var found SessionWithJoinCode
	result := tx.WithContext(ctx).Raw(`
		SELECT s.uuid, j.code AS join_code, s.lobby_expires_at
		FROM play_sessions s
		JOIN play_session_join_codes j ON j.session_id = s.id
		WHERE s.id = ?
		ORDER BY j.id
		LIMIT 1
	`, sessionID).Scan(&found)
	if result.Error != nil {
		return nil, fmt.Errorf("finding session with join code: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &found, nil
}
