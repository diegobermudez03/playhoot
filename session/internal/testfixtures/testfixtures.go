// Package testfixtures provides shared repository-test seeding helpers for
// session's workflows/sessionlifecycle package (its internal/repo
// persistence tests and its Manager repository-integration/concurrency
// tests), so they do not each reinvent
// sessions/session_actors/session_participants/join_codes seeding.
package testfixtures

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// SessionFixture identifies a seeded sessions row.
type SessionFixture struct {
	SessionID          uint
	SessionUUID        string
	GameDefinitionUUID string
}

type sessionSeedInsert struct {
	ID                 uint      `gorm:"column:id"`
	UUID               string    `gorm:"column:uuid"`
	GameDefinitionUUID string    `gorm:"column:game_definition_uuid"`
	Phase              string    `gorm:"column:phase"`
	LobbyExpiresAt     time.Time `gorm:"column:lobby_expires_at"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (sessionSeedInsert) TableName() string { return "sessions" }

// SeedLobbySession inserts a LOBBY sessions row with the given
// lobby_expires_at (pass a past time to seed an already-expired lobby).
func SeedLobbySession(t *testing.T, db *gorm.DB, lobbyExpiresAt time.Time) SessionFixture {
	t.Helper()

	now := time.Now().UTC()
	gameDefinitionUUID := uuid.NewString()
	row := sessionSeedInsert{
		UUID:               uuid.NewString(),
		GameDefinitionUUID: gameDefinitionUUID,
		Phase:              session.PhaseLobby,
		LobbyExpiresAt:     lobbyExpiresAt,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	require.NoError(t, db.Create(&row).Error)
	return SessionFixture{SessionID: row.ID, SessionUUID: row.UUID, GameDefinitionUUID: gameDefinitionUUID}
}

type actorSeedInsert struct {
	ID               uint      `gorm:"column:id"`
	SessionID        uint      `gorm:"column:session_id"`
	UserUUID         string    `gorm:"column:user_uuid"`
	SemanticPresence string    `gorm:"column:semantic_presence"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (actorSeedInsert) TableName() string { return "session_actors" }

// SeedActor inserts a session_actors row and returns its id.
func SeedActor(t *testing.T, db *gorm.DB, sessionID uint, userUUID string) uint {
	t.Helper()

	row := actorSeedInsert{
		SessionID:        sessionID,
		UserUUID:         userUUID,
		SemanticPresence: session.PresenceConnected,
		CreatedAt:        time.Now().UTC(),
	}
	require.NoError(t, db.Create(&row).Error)
	return row.ID
}

type participantSeedInsert struct {
	ID             uint      `gorm:"column:id"`
	SessionActorID uint      `gorm:"column:session_actor_id"`
	DisplayName    string    `gorm:"column:display_name"`
	Active         bool      `gorm:"column:active"`
	JoinedAt       time.Time `gorm:"column:joined_at"`
}

func (participantSeedInsert) TableName() string { return "session_participants" }

// SeedParticipantForActor inserts an active session_participants row owned
// by actorID.
func SeedParticipantForActor(t *testing.T, db *gorm.DB, actorID uint, displayName string) {
	t.Helper()

	row := participantSeedInsert{
		SessionActorID: actorID,
		DisplayName:    displayName,
		Active:         true,
		JoinedAt:       time.Now().UTC(),
	}
	require.NoError(t, db.Create(&row).Error)
}

// SeedActiveParticipant seeds both the SessionActor and its active
// Participant for (sessionID, userUUID).
func SeedActiveParticipant(t *testing.T, db *gorm.DB, sessionID uint, userUUID, displayName string) uint {
	t.Helper()

	actorID := SeedActor(t, db, sessionID, userUUID)
	SeedParticipantForActor(t, db, actorID, displayName)
	return actorID
}

type joinCodeSeedInsert struct {
	ID        uint       `gorm:"column:id"`
	SessionID uint       `gorm:"column:session_id"`
	Code      uint       `gorm:"column:code"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
}

func (joinCodeSeedInsert) TableName() string { return "join_codes" }

// SeedJoinCode inserts a join_codes row, optionally already revoked.
func SeedJoinCode(t *testing.T, db *gorm.DB, sessionID uint, code uint, revoked bool) {
	t.Helper()

	row := joinCodeSeedInsert{
		SessionID: sessionID,
		Code:      code,
		CreatedAt: time.Now().UTC(),
	}
	if revoked {
		revokedAt := time.Now().UTC()
		row.RevokedAt = &revokedAt
	}
	require.NoError(t, db.Create(&row).Error)
}

// GameVersionFixture identifies a seeded session_games/
// session_game_version_artifacts pair.
type GameVersionFixture struct {
	GameUUID       string
	DefinitionUUID string
}

type gameSeedInsert struct {
	ID                    uint      `gorm:"column:id"`
	GameUUID              string    `gorm:"column:game_uuid"`
	CurrentDefinitionUUID *string   `gorm:"column:current_definition_uuid"`
	CreatedAt             time.Time `gorm:"column:created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (gameSeedInsert) TableName() string { return "session_games" }

type gameVersionArtifactSeedInsert struct {
	ID             uint      `gorm:"column:id"`
	DefinitionUUID string    `gorm:"column:definition_uuid"`
	GameUUID       string    `gorm:"column:game_uuid"`
	BackendScript  string    `gorm:"column:backend_script"`
	FrontendScript string    `gorm:"column:frontend_script"`
	ParticipantMin int       `gorm:"column:participant_min"`
	ParticipantMax *int      `gorm:"column:participant_max"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (gameVersionArtifactSeedInsert) TableName() string { return "session_game_version_artifacts" }

type runtimeTurnSeedInsert struct {
	ID         uint            `gorm:"column:id"`
	SessionID  uint            `gorm:"column:session_id"`
	Sequence   uint64          `gorm:"column:sequence"`
	SourceKind string          `gorm:"column:source_kind"`
	NewState   json.RawMessage `gorm:"column:new_state"`
	CreatedAt  time.Time       `gorm:"column:created_at"`
}

func (runtimeTurnSeedInsert) TableName() string { return "session_runtime_turns" }

// SeedRuntimeTurn inserts a session_runtime_turns row directly (bypassing
// any Manager step), for a RUNNING-phase test that needs a Session already
// past Start without exercising Start itself. Returns the new row's
// internal id, the value a test typically also assigns to
// sessions.current_turn_id itself.
func SeedRuntimeTurn(t *testing.T, db *gorm.DB, sessionID uint, sequence uint64, sourceKind string, newState json.RawMessage) uint {
	t.Helper()

	row := runtimeTurnSeedInsert{
		SessionID:  sessionID,
		Sequence:   sequence,
		SourceKind: sourceKind,
		NewState:   newState,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, db.Create(&row).Error)
	return row.ID
}

type timerObligationSeedInsert struct {
	ID              uint      `gorm:"column:id"`
	UUID            string    `gorm:"column:uuid"`
	SessionID       uint      `gorm:"column:session_id"`
	Timer           string    `gorm:"column:timer"`
	DelayMs         int64     `gorm:"column:delay_ms"`
	State           string    `gorm:"column:state"`
	CreatedByTurnID uint      `gorm:"column:created_by_turn_id"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

func (timerObligationSeedInsert) TableName() string { return "session_timer_obligations" }

// SeedTimerObligation inserts an ACTIVE session_timer_obligations row
// directly - a test exercising expiration seeds the obligation it expires
// directly, rather than first executing a ScheduleTimer command through the
// full RUNNING-phase machinery merely to set up its own starting state.
// Returns the new row's public UUID.
func SeedTimerObligation(t *testing.T, db *gorm.DB, sessionID uint, timer string, delayMs int64, createdByTurnID uint) string {
	t.Helper()

	row := timerObligationSeedInsert{
		UUID:            uuid.NewString(),
		SessionID:       sessionID,
		Timer:           timer,
		DelayMs:         delayMs,
		State:           session.TimerObligationStateActive,
		CreatedByTurnID: createdByTurnID,
		CreatedAt:       time.Now().UTC(),
	}
	require.NoError(t, db.Create(&row).Error)
	return row.UUID
}

// SeedCurrentGameVersion seeds a session_game_version_artifacts row and a
// session_games row pointing at it as the Game's current version - the
// fixture shape Create's own tests need in place of Game Management, since
// nothing yet populates these tables for a real, newly authored Game version.
// Insertion order follows both tables' FK pair: session_games first (its
// current_definition_uuid FK cannot yet point anywhere), then the artifact
// (its game_uuid FK requires the session_games row to already exist), then
// an update assigning the now-existing artifact as current - mirroring this
// package's own CreateSessionWithHost insert-then-assign pattern.
// participantMax nil seeds an unlimited version, matching
// ParticipantConstraints' own "Max absent means unlimited" contract.
func SeedCurrentGameVersion(t *testing.T, db *gorm.DB, backendScript, frontendScript string, participantMin int, participantMax *int) GameVersionFixture {
	t.Helper()

	now := time.Now().UTC()
	gameUUID := uuid.NewString()
	definitionUUID := uuid.NewString()

	require.NoError(t, db.Create(&gameSeedInsert{GameUUID: gameUUID, CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&gameVersionArtifactSeedInsert{
		DefinitionUUID: definitionUUID,
		GameUUID:       gameUUID,
		BackendScript:  backendScript,
		FrontendScript: frontendScript,
		ParticipantMin: participantMin,
		ParticipantMax: participantMax,
		CreatedAt:      now,
	}).Error)
	require.NoError(t, db.Exec(`UPDATE session_games SET current_definition_uuid = ? WHERE game_uuid = ?`, definitionUUID, gameUUID).Error)

	return GameVersionFixture{GameUUID: gameUUID, DefinitionUUID: definitionUUID}
}
