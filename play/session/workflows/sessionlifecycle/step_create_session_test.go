package sessionlifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

func TestCreateSessionInTx(t *testing.T) {
	type dependencies struct {
		repo *MockcreateSessionRepo
	}
	type test struct {
		joinCodes []string

		expectedAttempt createSessionAttempt
		expectErr       bool
		expectedErrorIs error
	}

	const (
		userUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		gameUUID  = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		otherGame = "16fd2706-8baf-433b-82eb-8c7fada847da"
		key       = "key-1"
		requestID = uint(11)
		versionID = uint(21)
		sessionID = uint(31)
	)
	ctx := context.Background()
	tx := &gorm.DB{}
	now := time.Date(2026, 10, 15, 12, 0, 0, 123456789, time.UTC)
	nowInDB := now.Truncate(time.Microsecond)
	lobbyExpiresAt := nowInDB.Add(defaultLobbyTTL)
	executable := &internalrepo.GameVersion{ID: versionID, Status: "EXECUTABLE"}
	createdSession := internalrepo.CreatedSession{ID: sessionID, UUID: "session-uuid-1"}
	outcome := func(o sessionpkg.CreateSessionOutcome) *string { s := string(o); return &s }
	sessionIDRef := func() *uint { id := sessionID; return &id }
	createdResult := func(code string) createSessionAttempt {
		return createSessionAttempt{result: sessionpkg.CreateSessionResult{
			Outcome:     sessionpkg.CreateSessionCreated,
			SessionUUID: "session-uuid-1",
			JoinCode:    code,
			ExpiresAt:   lobbyExpiresAt,
		}}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_created": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "ABC234", lobbyExpiresAt).Return(true, nil),
				deps.repo.EXPECT().CompleteCreateSessionRequest(ctx, tx, requestID, "created", sessionIDRef()).Return(nil),
			)
			return test{joinCodes: []string{"ABC234"}, expectedAttempt: createdResult("ABC234")}
		},
		"join_code_in_use_is_skipped_for_a_new_one": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "TAKEN1", lobbyExpiresAt).Return(false, nil),
				deps.repo.EXPECT().ReleaseExpiredJoinCode(ctx, tx, "TAKEN1", nowInDB).Return(false, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "FREE22", lobbyExpiresAt).Return(true, nil),
				deps.repo.EXPECT().CompleteCreateSessionRequest(ctx, tx, requestID, "created", sessionIDRef()).Return(nil),
			)
			return test{joinCodes: []string{"TAKEN1", "FREE22"}, expectedAttempt: createdResult("FREE22")}
		},
		"expired_join_code_is_released_and_reused": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "OLD333", lobbyExpiresAt).Return(false, nil),
				deps.repo.EXPECT().ReleaseExpiredJoinCode(ctx, tx, "OLD333", nowInDB).Return(true, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "OLD333", lobbyExpiresAt).Return(true, nil),
				deps.repo.EXPECT().CompleteCreateSessionRequest(ctx, tx, requestID, "created", sessionIDRef()).Return(nil),
			)
			return test{joinCodes: []string{"OLD333"}, expectedAttempt: createdResult("OLD333")}
		},
		"released_join_code_taken_again_concurrently_moves_on": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "RACE44", lobbyExpiresAt).Return(false, nil),
				deps.repo.EXPECT().ReleaseExpiredJoinCode(ctx, tx, "RACE44", nowInDB).Return(true, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "RACE44", lobbyExpiresAt).Return(false, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "NEXT55", lobbyExpiresAt).Return(true, nil),
				deps.repo.EXPECT().CompleteCreateSessionRequest(ctx, tx, requestID, "created", sessionIDRef()).Return(nil),
			)
			return test{joinCodes: []string{"RACE44", "NEXT55"}, expectedAttempt: createdResult("NEXT55")}
		},
		"no_free_join_code_after_max_attempts": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil)
			deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil)
			deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil)
			deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "BUSY66", lobbyExpiresAt).Return(false, nil).Times(maxJoinCodeAttempts)
			deps.repo.EXPECT().ReleaseExpiredJoinCode(ctx, tx, "BUSY66", nowInDB).Return(false, nil).Times(maxJoinCodeAttempts)
			codes := make([]string, maxJoinCodeAttempts)
			for i := range codes {
				codes[i] = "BUSY66"
			}
			return test{joinCodes: codes, expectErr: true}
		},
		"game_without_current_version_is_unavailable": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(nil, nil),
				deps.repo.EXPECT().CompleteCreateSessionRequest(ctx, tx, requestID, "game_unavailable", nil).Return(nil),
			)
			return test{expectedAttempt: createSessionAttempt{result: sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}}}
		},
		"current_version_not_executable_is_broken_data": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(&internalrepo.GameVersion{ID: versionID, Status: "DRAFT"}, nil),
			)
			return test{expectErr: true}
		},
		"replay_of_created_session": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
					ID: requestID, GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionCreated), SessionID: sessionIDRef(),
				}, nil),
				deps.repo.EXPECT().FindSessionWithJoinCode(ctx, tx, sessionID).Return(&internalrepo.SessionWithJoinCode{
					UUID: "session-uuid-1", JoinCode: "ABC234", LobbyExpiresAt: lobbyExpiresAt.In(time.FixedZone("UTC-5", -5*3600)),
				}, nil),
			)
			attempt := createdResult("ABC234")
			attempt.replayed = true
			return test{expectedAttempt: attempt}
		},
		"replay_of_game_unavailable": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
				ID: requestID, GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionGameUnavailable),
			}, nil)
			return test{expectedAttempt: createSessionAttempt{
				result:   sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable},
				replayed: true,
			}}
		},
		"same_key_for_another_game_is_a_conflict": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
				ID: requestID, GameUUID: otherGame, Outcome: outcome(sessionpkg.CreateSessionCreated), SessionID: sessionIDRef(),
			}, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrIdempotencyConflict}
		},
		"replayed_request_without_outcome_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
				ID: requestID, GameUUID: gameUUID,
			}, nil)
			return test{expectErr: true}
		},
		"replayed_created_request_without_session_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
				ID: requestID, GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionCreated),
			}, nil)
			return test{expectErr: true}
		},
		"replayed_session_not_found_is_broken_data": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
					ID: requestID, GameUUID: gameUUID, Outcome: outcome(sessionpkg.CreateSessionCreated), SessionID: sessionIDRef(),
				}, nil),
				deps.repo.EXPECT().FindSessionWithJoinCode(ctx, tx, sessionID).Return(nil, nil),
			)
			return test{expectErr: true}
		},
		"replayed_request_with_unknown_outcome_is_broken_data": func(t *testing.T, deps dependencies) test {
			unknown := "something_else"
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), &internalrepo.CreateSessionRequest{
				ID: requestID, GameUUID: gameUUID, Outcome: &unknown,
			}, nil)
			return test{expectErr: true}
		},
		"claim_fails": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(uint(0), nil, errors.New("db down"))
			return test{expectErr: true}
		},
		"finding_current_version_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(nil, errors.New("db down")),
			)
			return test{expectErr: true}
		},
		"creating_session_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(internalrepo.CreatedSession{}, errors.New("db down")),
			)
			return test{expectErr: true}
		},
		"creating_join_code_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "ABC234", lobbyExpiresAt).Return(false, errors.New("db down")),
			)
			return test{joinCodes: []string{"ABC234"}, expectErr: true}
		},
		"completing_request_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateSessionRequest(ctx, tx, userUUID, key, gameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().FindCurrentGameVersion(ctx, tx, gameUUID).Return(executable, nil),
				deps.repo.EXPECT().CreateSession(ctx, tx, versionID, userUUID, "LOBBY", lobbyExpiresAt).Return(createdSession, nil),
				deps.repo.EXPECT().CreateJoinCode(ctx, tx, sessionID, "ABC234", lobbyExpiresAt).Return(true, nil),
				deps.repo.EXPECT().CompleteCreateSessionRequest(ctx, tx, requestID, "created", sessionIDRef()).Return(errors.New("db down")),
			)
			return test{joinCodes: []string{"ABC234"}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcreateSessionRepo(ctrl)}
			tc := setup(t, deps)

			codes := tc.joinCodes
			manager := &Manager{
				createSessionRepo: deps.repo,
				now:               func() time.Time { return now },
				newJoinCode: func() (string, error) {
					require.NotEmpty(t, codes, "more join codes generated than the case expects")
					code := codes[0]
					codes = codes[1:]
					return code, nil
				},
				lobbyTTL: defaultLobbyTTL,
			}

			attempt, err := manager.createSessionInTx(ctx, tx, userUUID, gameUUID, key)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedAttempt, attempt)
		})
	}
}

func TestCreateSession(t *testing.T) {
	type test struct {
		userUUID       string
		gameUUID       string
		idempotencyKey string
	}

	// Only input validation is exercised here: everything past it runs in a
	// database transaction, covered by createSessionInTx's tests and the
	// repository tests.
	tests := map[string]func(t *testing.T) test{
		"user_uuid_is_not_a_uuid": func(t *testing.T) test {
			return test{userUUID: "nope", gameUUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", idempotencyKey: "key-1"}
		},
		"game_uuid_is_not_a_uuid": func(t *testing.T) test {
			return test{userUUID: "0f8fad5b-d9cb-469f-a165-70867728950e", gameUUID: "nope", idempotencyKey: "key-1"}
		},
		"idempotency_key_missing": func(t *testing.T) test {
			return test{userUUID: "0f8fad5b-d9cb-469f-a165-70867728950e", gameUUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			result, err := (&Manager{}).CreateSession(context.Background(), tc.userUUID, tc.gameUUID, tc.idempotencyKey)

			require.Error(t, err)
			require.Equal(t, sessionpkg.CreateSessionResult{}, result)
		})
	}
}

func TestNewJoinCode(t *testing.T) {
	// Random output: check the shape over many draws instead of exact values.
	for range 1000 {
		code, err := newJoinCode()

		require.NoError(t, err)
		require.Len(t, code, joinCodeLength)
		for _, char := range code {
			require.True(t, strings.ContainsRune(joinCodeAlphabet, char), "unexpected character %q in %q", char, code)
		}
	}
}
