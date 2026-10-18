package repo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

type testGame struct {
	ID               uint
	GameUUID         string
	CurrentVersionID *uint
}

func (testGame) TableName() string { return "play_games" }

type testGameVersion struct {
	ID           uint
	UUID         string
	GameID       uint
	Status       string
	Manifest     string
	ExecutableAt *time.Time
}

func (testGameVersion) TableName() string { return "play_game_versions" }

type testJoinCode struct {
	ID         uint
	SessionID  uint
	Code       string
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (testJoinCode) TableName() string { return "play_session_join_codes" }

type testCreateSessionRequest struct {
	ID             uint
	UserUUID       string
	IdempotencyKey string
	GameUUID       string
	Outcome        *string
	SessionID      *uint
}

func (testCreateSessionRequest) TableName() string { return "play_create_session_requests" }

// seedGame creates a Game whose current version has status, or a Game with no
// current version when status is empty.
func seedGame(t *testing.T, db *gorm.DB, status string) (testGame, *testGameVersion) {
	t.Helper()
	game := testGame{GameUUID: uuid.NewString()}
	require.NoError(t, db.Create(&game).Error)
	if status == "" {
		return game, nil
	}
	version := testGameVersion{UUID: uuid.NewString(), GameID: game.ID, Status: status, Manifest: `{"schema_version":1,"players":{"min":1,"max":8}}`}
	require.NoError(t, db.Create(&version).Error)
	require.NoError(t, db.Exec(`UPDATE play_games SET current_version_id = ? WHERE id = ?`, version.ID, game.ID).Error)
	game.CurrentVersionID = &version.ID
	return game, &version
}

func seedSession(t *testing.T, db *gorm.DB, lobbyExpiresAt time.Time) session {
	t.Helper()
	_, version := seedGame(t, db, "EXECUTABLE")
	created := session{
		UUID:              uuid.NewString(),
		GameVersionID:     version.ID,
		CreatedByUserUUID: uuid.NewString(),
		Status:            "LOBBY",
		LobbyExpiresAt:    lobbyExpiresAt,
	}
	require.NoError(t, db.Create(&created).Error)
	return created
}

// randomCode returns a join code no other test uses, since every test shares
// one database.
func randomCode() string {
	return strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
}

func activeJoinCodes(t *testing.T, db *gorm.DB, code string) []testJoinCode {
	t.Helper()
	var codes []testJoinCode
	require.NoError(t, db.Raw(`SELECT id, session_id, code, expires_at, released_at FROM play_session_join_codes WHERE code = ? AND released_at IS NULL`, code).Scan(&codes).Error)
	return codes
}

func TestClaimCreateSessionRequest(t *testing.T) {
	type test struct {
		userUUID       string
		idempotencyKey string
		gameUUID       string

		expectFreshClaim bool
		expectedExisting *CreateSessionRequest
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"free_identity_is_claimed": func(t *testing.T, db *gorm.DB) test {
			return test{userUUID: uuid.NewString(), idempotencyKey: "key-1", gameUUID: uuid.NewString(), expectFreshClaim: true}
		},
		"claimed_identity_returns_the_existing_request": func(t *testing.T, db *gorm.DB) test {
			userUUID, gameUUID := uuid.NewString(), uuid.NewString()
			created := seedSession(t, db, time.Now().Add(time.Hour))
			outcome := "created"
			existing := testCreateSessionRequest{UserUUID: userUUID, IdempotencyKey: "key-1", GameUUID: gameUUID, Outcome: &outcome, SessionID: &created.ID}
			require.NoError(t, db.Create(&existing).Error)
			return test{
				userUUID: userUUID, idempotencyKey: "key-1", gameUUID: uuid.NewString(),
				expectedExisting: &CreateSessionRequest{ID: existing.ID, GameUUID: gameUUID, Outcome: &outcome, SessionID: &created.ID},
			}
		},
		"same_key_of_another_user_is_a_separate_identity": func(t *testing.T, db *gorm.DB) test {
			require.NoError(t, db.Create(&testCreateSessionRequest{UserUUID: uuid.NewString(), IdempotencyKey: "shared-key", GameUUID: uuid.NewString()}).Error)
			return test{userUUID: uuid.NewString(), idempotencyKey: "shared-key", gameUUID: uuid.NewString(), expectFreshClaim: true}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			requestID, existing, err := repo.ClaimCreateSessionRequest(context.Background(), db, tc.userUUID, tc.idempotencyKey, tc.gameUUID)

			require.NoError(t, err)
			require.Equal(t, tc.expectedExisting, existing)
			if !tc.expectFreshClaim {
				require.Zero(t, requestID)
				return
			}
			require.NotZero(t, requestID)
			var stored testCreateSessionRequest
			require.NoError(t, db.Raw(`SELECT id, user_uuid, idempotency_key, game_uuid, outcome, session_id FROM play_create_session_requests WHERE id = ?`, requestID).Scan(&stored).Error)
			require.Equal(t, testCreateSessionRequest{ID: requestID, UserUUID: tc.userUUID, IdempotencyKey: tc.idempotencyKey, GameUUID: tc.gameUUID}, stored)
		})
	}
}

func TestCompleteCreateSessionRequest(t *testing.T) {
	type test struct {
		requestID uint
		outcome   string
		sessionID *uint

		expectErr       bool
		expectedOutcome *string
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"completed_with_created_session": func(t *testing.T, db *gorm.DB) test {
			request := testCreateSessionRequest{UserUUID: uuid.NewString(), IdempotencyKey: "key-1", GameUUID: uuid.NewString()}
			require.NoError(t, db.Create(&request).Error)
			created := seedSession(t, db, time.Now().Add(time.Hour))
			outcome := "created"
			return test{requestID: request.ID, outcome: outcome, sessionID: &created.ID, expectedOutcome: &outcome}
		},
		"completed_as_game_unavailable": func(t *testing.T, db *gorm.DB) test {
			request := testCreateSessionRequest{UserUUID: uuid.NewString(), IdempotencyKey: "key-1", GameUUID: uuid.NewString()}
			require.NoError(t, db.Create(&request).Error)
			outcome := "game_unavailable"
			return test{requestID: request.ID, outcome: outcome, expectedOutcome: &outcome}
		},
		"unknown_request": func(t *testing.T, db *gorm.DB) test {
			var maxID uint
			require.NoError(t, db.Raw(`SELECT COALESCE(MAX(id), 0) FROM play_create_session_requests`).Scan(&maxID).Error)
			return test{requestID: maxID + 1000, outcome: "created", expectErr: true}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			err := repo.CompleteCreateSessionRequest(context.Background(), db, tc.requestID, tc.outcome, tc.sessionID)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var stored testCreateSessionRequest
			require.NoError(t, db.Raw(`SELECT id, outcome, session_id FROM play_create_session_requests WHERE id = ?`, tc.requestID).Scan(&stored).Error)
			require.Equal(t, tc.expectedOutcome, stored.Outcome)
			require.Equal(t, tc.sessionID, stored.SessionID)
		})
	}
}

func TestFindCurrentGameVersion(t *testing.T) {
	type test struct {
		gameUUID        string
		expectedVersion *GameVersion
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"executable_current_version": func(t *testing.T, db *gorm.DB) test {
			game, version := seedGame(t, db, "EXECUTABLE")
			return test{gameUUID: game.GameUUID, expectedVersion: &GameVersion{ID: version.ID, Status: "EXECUTABLE"}}
		},
		"current_version_reported_with_its_actual_status": func(t *testing.T, db *gorm.DB) test {
			game, version := seedGame(t, db, "DRAFT")
			return test{gameUUID: game.GameUUID, expectedVersion: &GameVersion{ID: version.ID, Status: "DRAFT"}}
		},
		"newer_draft_does_not_replace_the_current_version": func(t *testing.T, db *gorm.DB) test {
			game, version := seedGame(t, db, "EXECUTABLE")
			require.NoError(t, db.Create(&testGameVersion{UUID: uuid.NewString(), GameID: game.ID, Status: "DRAFT", Manifest: `{"schema_version":1,"players":{"min":1,"max":8}}`}).Error)
			return test{gameUUID: game.GameUUID, expectedVersion: &GameVersion{ID: version.ID, Status: "EXECUTABLE"}}
		},
		"game_without_current_version": func(t *testing.T, db *gorm.DB) test {
			game, _ := seedGame(t, db, "")
			return test{gameUUID: game.GameUUID}
		},
		"unknown_game": func(t *testing.T, db *gorm.DB) test {
			return test{gameUUID: uuid.NewString()}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			version, err := repo.FindCurrentGameVersion(context.Background(), db, tc.gameUUID)

			require.NoError(t, err)
			require.Equal(t, tc.expectedVersion, version)
		})
	}
}

func TestCreateSession(t *testing.T) {
	type test struct {
		gameVersionID     uint
		createdByUserUUID string
		status            string
		lobbyExpiresAt    time.Time
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"lobby_session": func(t *testing.T, db *gorm.DB) test {
			_, version := seedGame(t, db, "EXECUTABLE")
			return test{
				gameVersionID:     version.ID,
				createdByUserUUID: uuid.NewString(),
				status:            "LOBBY",
				lobbyExpiresAt:    time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC),
			}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			created, err := repo.CreateSession(context.Background(), db, tc.gameVersionID, tc.createdByUserUUID, tc.status, tc.lobbyExpiresAt)

			require.NoError(t, err)
			require.NotZero(t, created.ID)
			_, err = uuid.Parse(created.UUID)
			require.NoError(t, err)

			var stored session
			require.NoError(t, db.Raw(`SELECT id, uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at FROM play_sessions WHERE id = ?`, created.ID).Scan(&stored).Error)
			require.Equal(t, session{
				ID:                created.ID,
				UUID:              created.UUID,
				GameVersionID:     tc.gameVersionID,
				CreatedByUserUUID: tc.createdByUserUUID,
				Status:            tc.status,
				LobbyExpiresAt:    tc.lobbyExpiresAt,
			}, session{
				ID:                stored.ID,
				UUID:              stored.UUID,
				GameVersionID:     stored.GameVersionID,
				CreatedByUserUUID: stored.CreatedByUserUUID,
				Status:            stored.Status,
				LobbyExpiresAt:    stored.LobbyExpiresAt.UTC(),
			})
		})
	}
}

func TestCreateJoinCode(t *testing.T) {
	type test struct {
		sessionID uint
		code      string
		expiresAt time.Time

		expectedCreated     bool
		expectedActiveOwner uint
	}

	expiresAt := time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"free_code": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, expiresAt)
			return test{sessionID: created.ID, code: randomCode(), expiresAt: expiresAt, expectedCreated: true, expectedActiveOwner: created.ID}
		},
		"code_held_by_an_active_join_code": func(t *testing.T, db *gorm.DB) test {
			holder := seedSession(t, db, expiresAt)
			code := randomCode()
			require.NoError(t, db.Create(&testJoinCode{SessionID: holder.ID, Code: code, ExpiresAt: expiresAt}).Error)
			created := seedSession(t, db, expiresAt)
			return test{sessionID: created.ID, code: code, expiresAt: expiresAt, expectedActiveOwner: holder.ID}
		},
		"code_held_only_by_released_join_codes": func(t *testing.T, db *gorm.DB) test {
			previous := seedSession(t, db, expiresAt)
			code := randomCode()
			releasedAt := expiresAt.Add(-time.Minute)
			require.NoError(t, db.Create(&testJoinCode{SessionID: previous.ID, Code: code, ExpiresAt: expiresAt, ReleasedAt: &releasedAt}).Error)
			created := seedSession(t, db, expiresAt)
			return test{sessionID: created.ID, code: code, expiresAt: expiresAt, expectedCreated: true, expectedActiveOwner: created.ID}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			created, err := repo.CreateJoinCode(context.Background(), db, tc.sessionID, tc.code, tc.expiresAt)

			require.NoError(t, err)
			require.Equal(t, tc.expectedCreated, created)
			active := activeJoinCodes(t, db, tc.code)
			require.Len(t, active, 1)
			require.Equal(t, tc.expectedActiveOwner, active[0].SessionID)
		})
	}
}

func TestReleaseExpiredJoinCode(t *testing.T) {
	type test struct {
		code       string
		releasedAt time.Time

		expectedReleased   bool
		expectedActiveLeft int
	}

	releasedAt := time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"expired_active_code_is_released": func(t *testing.T, db *gorm.DB) test {
			holder := seedSession(t, db, releasedAt)
			code := randomCode()
			require.NoError(t, db.Create(&testJoinCode{SessionID: holder.ID, Code: code, ExpiresAt: releasedAt.Add(-time.Minute)}).Error)
			return test{code: code, releasedAt: releasedAt, expectedReleased: true}
		},
		"code_expiring_exactly_now_is_released": func(t *testing.T, db *gorm.DB) test {
			holder := seedSession(t, db, releasedAt)
			code := randomCode()
			require.NoError(t, db.Create(&testJoinCode{SessionID: holder.ID, Code: code, ExpiresAt: releasedAt}).Error)
			return test{code: code, releasedAt: releasedAt, expectedReleased: true}
		},
		"code_not_expired_yet_stays_active": func(t *testing.T, db *gorm.DB) test {
			holder := seedSession(t, db, releasedAt)
			code := randomCode()
			require.NoError(t, db.Create(&testJoinCode{SessionID: holder.ID, Code: code, ExpiresAt: releasedAt.Add(time.Minute)}).Error)
			return test{code: code, releasedAt: releasedAt, expectedActiveLeft: 1}
		},
		"unused_code": func(t *testing.T, db *gorm.DB) test {
			return test{code: randomCode(), releasedAt: releasedAt}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			released, err := repo.ReleaseExpiredJoinCode(context.Background(), db, tc.code, tc.releasedAt)

			require.NoError(t, err)
			require.Equal(t, tc.expectedReleased, released)
			require.Len(t, activeJoinCodes(t, db, tc.code), tc.expectedActiveLeft)
			if tc.expectedReleased {
				var stored []testJoinCode
				require.NoError(t, db.Raw(`SELECT id, released_at FROM play_session_join_codes WHERE code = ?`, tc.code).Scan(&stored).Error)
				require.Len(t, stored, 1)
				require.NotNil(t, stored[0].ReleasedAt)
				require.True(t, stored[0].ReleasedAt.Equal(tc.releasedAt))
			}
		})
	}
}

func TestFindSessionWithJoinCode(t *testing.T) {
	type test struct {
		sessionID uint
		expected  *SessionWithJoinCode
	}

	lobbyExpiresAt := time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"session_with_its_first_join_code": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, lobbyExpiresAt)
			first, second := randomCode(), randomCode()
			releasedAt := lobbyExpiresAt
			require.NoError(t, db.Create(&testJoinCode{SessionID: created.ID, Code: first, ExpiresAt: lobbyExpiresAt, ReleasedAt: &releasedAt}).Error)
			require.NoError(t, db.Create(&testJoinCode{SessionID: created.ID, Code: second, ExpiresAt: lobbyExpiresAt}).Error)
			return test{sessionID: created.ID, expected: &SessionWithJoinCode{UUID: created.UUID, JoinCode: first, LobbyExpiresAt: lobbyExpiresAt}}
		},
		"session_without_join_code": func(t *testing.T, db *gorm.DB) test {
			created := seedSession(t, db, lobbyExpiresAt)
			return test{sessionID: created.ID}
		},
		"unknown_session": func(t *testing.T, db *gorm.DB) test {
			var maxID uint
			require.NoError(t, db.Raw(`SELECT COALESCE(MAX(id), 0) FROM play_sessions`).Scan(&maxID).Error)
			return test{sessionID: maxID + 1000}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			found, err := repo.FindSessionWithJoinCode(context.Background(), db, tc.sessionID)

			require.NoError(t, err)
			if found != nil {
				found.LobbyExpiresAt = found.LobbyExpiresAt.UTC()
			}
			require.Equal(t, tc.expected, found)
		})
	}
}
