package sessionlifecycle

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// Integration tests run CreateSession end to end against the shared disposable
// database: Manager, repository and schema together. Every case uses its own
// random user, Games and join codes so cases never see each other's rows.

type integrationGame struct {
	ID               uint
	GameUUID         string
	CurrentVersionID *uint
}

func (integrationGame) TableName() string { return "play_games" }

type integrationGameVersion struct {
	ID     uint
	UUID   string
	GameID uint
	Status string
}

func (integrationGameVersion) TableName() string { return "play_game_versions" }

type integrationSession struct {
	ID                uint
	UUID              string
	GameVersionID     uint
	CreatedByUserUUID string
	Status            string
	LobbyExpiresAt    time.Time
}

func (integrationSession) TableName() string { return "play_sessions" }

type integrationJoinCode struct {
	ID         uint
	SessionID  uint
	Code       string
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (integrationJoinCode) TableName() string { return "play_session_join_codes" }

type integrationRequest struct {
	IdempotencyKey string
	GameUUID       string
	Outcome        *string
	SessionID      *uint
}

// seedIntegrationGame creates a Game. With a status it also gets a version in
// that status set as its current version; with an empty status it has none.
func seedIntegrationGame(t *testing.T, db *gorm.DB, status string) (string, uint) {
	t.Helper()
	game := integrationGame{GameUUID: uuid.NewString()}
	require.NoError(t, db.Create(&game).Error)
	if status == "" {
		return game.GameUUID, 0
	}
	version := integrationGameVersion{UUID: uuid.NewString(), GameID: game.ID, Status: status}
	require.NoError(t, db.Create(&version).Error)
	require.NoError(t, db.Exec(`UPDATE play_games SET current_version_id = ? WHERE id = ?`, version.ID, game.ID).Error)
	return game.GameUUID, version.ID
}

// seedJoinCodeHolder creates another Session holding code as an active join
// code expiring at expiresAt, and returns that Session's id.
func seedJoinCodeHolder(t *testing.T, db *gorm.DB, code string, expiresAt time.Time) uint {
	t.Helper()
	_, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
	holder := integrationSession{
		UUID:              uuid.NewString(),
		GameVersionID:     versionID,
		CreatedByUserUUID: uuid.NewString(),
		Status:            "LOBBY",
		LobbyExpiresAt:    expiresAt,
	}
	require.NoError(t, db.Create(&holder).Error)
	require.NoError(t, db.Create(&integrationJoinCode{SessionID: holder.ID, Code: code, ExpiresAt: expiresAt}).Error)
	return holder.ID
}

func integrationCode() string {
	return strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
}

func sessionsCreatedBy(t *testing.T, db *gorm.DB, userUUID string) []integrationSession {
	t.Helper()
	var sessions []integrationSession
	require.NoError(t, db.Raw(`
		SELECT id, uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at
		FROM play_sessions WHERE created_by_user_uuid = ? ORDER BY id
	`, userUUID).Scan(&sessions).Error)
	for i := range sessions {
		sessions[i].LobbyExpiresAt = sessions[i].LobbyExpiresAt.UTC()
	}
	return sessions
}

func joinCodesOf(t *testing.T, db *gorm.DB, sessionID uint) []integrationJoinCode {
	t.Helper()
	var codes []integrationJoinCode
	require.NoError(t, db.Raw(`
		SELECT id, session_id, code, expires_at, released_at
		FROM play_session_join_codes WHERE session_id = ? ORDER BY id
	`, sessionID).Scan(&codes).Error)
	for i := range codes {
		codes[i].ID = 0
		codes[i].ExpiresAt = codes[i].ExpiresAt.UTC()
	}
	return codes
}

func requestsOf(t *testing.T, db *gorm.DB, userUUID string) []integrationRequest {
	t.Helper()
	var requests []integrationRequest
	require.NoError(t, db.Raw(`
		SELECT idempotency_key, game_uuid, outcome, session_id
		FROM play_create_session_requests WHERE user_uuid = ? ORDER BY id
	`, userUUID).Scan(&requests).Error)
	return requests
}

func TestCreateSessionIntegration(t *testing.T) {
	type test struct {
		userUUID       string
		gameUUID       string
		idempotencyKey string
		joinCodes      []string

		expectedResult  func(sessions []integrationSession) sessionpkg.CreateSessionResult
		expectErr       bool
		expectedErrorIs error

		// assertDB checks every row the call should (or should not) have
		// left behind.
		assertDB func(t *testing.T, sessions []integrationSession, requests []integrationRequest)
	}

	now := time.Date(2026, 10, 15, 12, 0, 0, 123456789, time.UTC)
	nowInDB := now.Truncate(time.Microsecond)
	lobbyExpiresAt := nowInDB.Add(defaultLobbyTTL)
	created := func(code string) func(sessions []integrationSession) sessionpkg.CreateSessionResult {
		return func(sessions []integrationSession) sessionpkg.CreateSessionResult {
			return sessionpkg.CreateSessionResult{
				Outcome:     sessionpkg.CreateSessionCreated,
				SessionUUID: sessions[0].UUID,
				JoinCode:    code,
				ExpiresAt:   lobbyExpiresAt,
			}
		}
	}
	unavailable := func([]integrationSession) sessionpkg.CreateSessionResult {
		return sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}
	}
	outcome := func(o sessionpkg.CreateSessionOutcome) *string { s := string(o); return &s }

	// assertCreated checks the single Session, its single active join code,
	// and the single completed request a successful creation leaves.
	assertCreated := func(db *gorm.DB, userUUID string, gameUUID string, versionID uint, code string) func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
		return func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
			require.Len(t, sessions, 1)
			require.Equal(t, integrationSession{
				ID:                sessions[0].ID,
				UUID:              sessions[0].UUID,
				GameVersionID:     versionID,
				CreatedByUserUUID: userUUID,
				Status:            "LOBBY",
				LobbyExpiresAt:    lobbyExpiresAt,
			}, sessions[0])
			require.Equal(t, []integrationJoinCode{{SessionID: sessions[0].ID, Code: code, ExpiresAt: lobbyExpiresAt}}, joinCodesOf(t, db, sessions[0].ID))
			sessionID := sessions[0].ID
			require.Equal(t, []integrationRequest{{IdempotencyKey: "key-1", GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionCreated), SessionID: &sessionID}}, requests)
		}
	}

	db := testdb.OpenSessionDB(t)

	tests := map[string]func(t *testing.T, db *gorm.DB, manager *Manager) test{
		"session_created": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, code := uuid.NewString(), integrationCode()
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1", joinCodes: []string{code},
				expectedResult: created(code),
				assertDB:       assertCreated(db, userUUID, gameUUID, versionID, code),
			}
		},
		"game_uuid_in_upper_case_is_the_same_game": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, code := uuid.NewString(), integrationCode()
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			return test{
				userUUID: userUUID, gameUUID: strings.ToUpper(gameUUID), idempotencyKey: "key-1", joinCodes: []string{code},
				expectedResult: created(code),
				assertDB:       assertCreated(db, userUUID, gameUUID, versionID, code),
			}
		},
		"retry_with_same_key_replays_the_created_session": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, code := uuid.NewString(), integrationCode()
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			manager.newJoinCode = func() (string, error) { return code, nil }
			_, err := manager.CreateSession(context.Background(), userUUID, gameUUID, "key-1")
			require.NoError(t, err)
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1",
				expectedResult: created(code),
				assertDB:       assertCreated(db, userUUID, gameUUID, versionID, code),
			}
		},
		"same_key_for_another_game_is_a_conflict": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, code := uuid.NewString(), integrationCode()
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			otherGameUUID, _ := seedIntegrationGame(t, db, "EXECUTABLE")
			manager.newJoinCode = func() (string, error) { return code, nil }
			_, err := manager.CreateSession(context.Background(), userUUID, gameUUID, "key-1")
			require.NoError(t, err)
			return test{
				userUUID: userUUID, gameUUID: otherGameUUID, idempotencyKey: "key-1",
				expectErr: true, expectedErrorIs: sessionpkg.ErrIdempotencyConflict,
				assertDB: assertCreated(db, userUUID, gameUUID, versionID, code),
			}
		},
		"same_user_with_new_key_creates_another_session": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, firstCode, secondCode := uuid.NewString(), integrationCode(), integrationCode()
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			manager.newJoinCode = func() (string, error) { return firstCode, nil }
			_, err := manager.CreateSession(context.Background(), userUUID, gameUUID, "key-0")
			require.NoError(t, err)
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1", joinCodes: []string{secondCode},
				expectedResult: func(sessions []integrationSession) sessionpkg.CreateSessionResult {
					return sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionCreated, SessionUUID: sessions[1].UUID, JoinCode: secondCode, ExpiresAt: lobbyExpiresAt}
				},
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					require.Len(t, sessions, 2)
					require.Equal(t, versionID, sessions[1].GameVersionID)
					require.Equal(t, []integrationJoinCode{{SessionID: sessions[1].ID, Code: secondCode, ExpiresAt: lobbyExpiresAt}}, joinCodesOf(t, db, sessions[1].ID))
					first, second := sessions[0].ID, sessions[1].ID
					require.Equal(t, []integrationRequest{
						{IdempotencyKey: "key-0", GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionCreated), SessionID: &first},
						{IdempotencyKey: "key-1", GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionCreated), SessionID: &second},
					}, requests)
				},
			}
		},
		"game_without_current_version_is_unavailable": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID := uuid.NewString()
			gameUUID, _ := seedIntegrationGame(t, db, "")
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1",
				expectedResult: unavailable,
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					require.Empty(t, sessions)
					require.Equal(t, []integrationRequest{{IdempotencyKey: "key-1", GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionGameUnavailable)}}, requests)
				},
			}
		},
		"unknown_game_is_unavailable": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, gameUUID := uuid.NewString(), uuid.NewString()
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1",
				expectedResult: unavailable,
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					require.Empty(t, sessions)
					require.Equal(t, []integrationRequest{{IdempotencyKey: "key-1", GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionGameUnavailable)}}, requests)
				},
			}
		},
		"retry_replays_unavailable_even_after_the_game_became_executable": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID := uuid.NewString()
			gameUUID, _ := seedIntegrationGame(t, db, "")
			_, err := manager.CreateSession(context.Background(), userUUID, gameUUID, "key-1")
			require.NoError(t, err)
			var gameID uint
			require.NoError(t, db.Raw(`SELECT id FROM play_games WHERE game_uuid = ?`, gameUUID).Scan(&gameID).Error)
			version := integrationGameVersion{UUID: uuid.NewString(), GameID: gameID, Status: "EXECUTABLE"}
			require.NoError(t, db.Create(&version).Error)
			require.NoError(t, db.Exec(`UPDATE play_games SET current_version_id = ? WHERE id = ?`, version.ID, gameID).Error)
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1",
				expectedResult: unavailable,
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					require.Empty(t, sessions)
					require.Equal(t, []integrationRequest{{IdempotencyKey: "key-1", GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionGameUnavailable)}}, requests)
				},
			}
		},
		"join_code_held_by_an_active_session_is_skipped": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, takenCode, freeCode := uuid.NewString(), integrationCode(), integrationCode()
			holderID := seedJoinCodeHolder(t, db, takenCode, nowInDB.Add(time.Minute))
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			createdAssert := assertCreated(db, userUUID, gameUUID, versionID, freeCode)
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1", joinCodes: []string{takenCode, freeCode},
				expectedResult: created(freeCode),
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					createdAssert(t, sessions, requests)
					require.Equal(t, []integrationJoinCode{{SessionID: holderID, Code: takenCode, ExpiresAt: nowInDB.Add(time.Minute)}}, joinCodesOf(t, db, holderID))
				},
			}
		},
		"expired_join_code_is_released_and_reused": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID, code := uuid.NewString(), integrationCode()
			holderExpiresAt := nowInDB.Add(-time.Minute)
			holderID := seedJoinCodeHolder(t, db, code, holderExpiresAt)
			gameUUID, versionID := seedIntegrationGame(t, db, "EXECUTABLE")
			createdAssert := assertCreated(db, userUUID, gameUUID, versionID, code)
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1", joinCodes: []string{code},
				expectedResult: created(code),
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					createdAssert(t, sessions, requests)
					holderCodes := joinCodesOf(t, db, holderID)
					require.Len(t, holderCodes, 1)
					require.NotNil(t, holderCodes[0].ReleasedAt)
					require.True(t, holderCodes[0].ReleasedAt.Equal(nowInDB))
				},
			}
		},
		"current_version_not_executable_leaves_nothing_behind": func(t *testing.T, db *gorm.DB, manager *Manager) test {
			userUUID := uuid.NewString()
			gameUUID, _ := seedIntegrationGame(t, db, "DRAFT")
			return test{
				userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: "key-1",
				expectErr: true,
				assertDB: func(t *testing.T, sessions []integrationSession, requests []integrationRequest) {
					require.Empty(t, sessions)
					require.Empty(t, requests, "the claim must roll back with the failed transaction")
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			manager := New(db)
			manager.now = func() time.Time { return now }
			tc := setup(t, db, manager)
			if tc.joinCodes != nil {
				codes := tc.joinCodes
				manager.newJoinCode = func() (string, error) {
					require.NotEmpty(t, codes, "more join codes generated than the case expects")
					code := codes[0]
					codes = codes[1:]
					return code, nil
				}
			}

			result, err := manager.CreateSession(context.Background(), tc.userUUID, tc.gameUUID, tc.idempotencyKey)

			sessions := sessionsCreatedBy(t, db, tc.userUUID)
			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				require.Equal(t, sessionpkg.CreateSessionResult{}, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedResult(sessions), result)
			}
			tc.assertDB(t, sessions, requestsOf(t, db, tc.userUUID))
		})
	}
}

// TestCreateSessionIntegrationConcurrentRetries sends the same request many
// times at once: exactly one Session must be created and every call must
// answer with it.
func TestCreateSessionIntegrationConcurrentRetries(t *testing.T) {
	const calls = 10

	db := testdb.OpenSessionDB(t)
	manager := New(db)
	userUUID := uuid.NewString()
	gameUUID, _ := seedIntegrationGame(t, db, "EXECUTABLE")

	results := make([]sessionpkg.CreateSessionResult, calls)
	errs := make([]error, calls)
	var wg sync.WaitGroup
	for i := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = manager.CreateSession(context.Background(), userUUID, gameUUID, "key-1")
		}()
	}
	wg.Wait()

	sessions := sessionsCreatedBy(t, db, userUUID)
	require.Len(t, sessions, 1)
	for i := range calls {
		require.NoError(t, errs[i])
		require.Equal(t, results[0], results[i])
	}
	require.Equal(t, sessionpkg.CreateSessionCreated, results[0].Outcome)
	require.Equal(t, sessions[0].UUID, results[0].SessionUUID)
	require.Len(t, requestsOf(t, db, userUUID), 1)
	require.Len(t, joinCodesOf(t, db, sessions[0].ID), 1)
}
