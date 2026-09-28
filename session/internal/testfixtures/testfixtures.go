// Package testfixtures provides shared repository-test seeding helpers for
// session's workflows/sessionlifecycle package (its internal/repo
// persistence tests and its Manager repository-integration/concurrency
// tests), so they do not each reinvent
// sessions/session_actors/session_participants/join_codes seeding.
package testfixtures

import (
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
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (gameVersionArtifactSeedInsert) TableName() string { return "session_game_version_artifacts" }

// SeedCurrentGameVersion seeds a session_game_version_artifacts row and a
// session_games row pointing at it as the Game's current version - the
// fixture shape Create's own tests need in place of Game Management, since
// nothing yet populates these tables for a real, newly authored Game version.
// Insertion order follows both tables' FK pair: session_games first (its
// current_definition_uuid FK cannot yet point anywhere), then the artifact
// (its game_uuid FK requires the session_games row to already exist), then
// an update assigning the now-existing artifact as current - mirroring this
// package's own CreateSessionWithHost insert-then-assign pattern.
func SeedCurrentGameVersion(t *testing.T, db *gorm.DB, backendScript, frontendScript string) GameVersionFixture {
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
		CreatedAt:      now,
	}).Error)
	require.NoError(t, db.Exec(`UPDATE session_games SET current_definition_uuid = ? WHERE game_uuid = ?`, definitionUUID, gameUUID).Error)

	return GameVersionFixture{GameUUID: gameUUID, DefinitionUUID: definitionUUID}
}
