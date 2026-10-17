package playlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestCreateSession(t *testing.T) {
	type dependencies struct {
		games    *MockgamePlayabilityChecker
		sessions *MocksessionCreator
	}
	type test struct {
		ctx            context.Context
		userUUID       string
		gameUUID       string
		idempotencyKey string

		expectedResult  sessionpkg.CreateSessionResult
		expectErr       bool
		expectedErrorIs error
	}

	const (
		userUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		gameUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		key      = "key-1"
	)
	expiresAt := time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC)
	created := sessionpkg.CreateSessionResult{
		Outcome:     sessionpkg.CreateSessionCreated,
		SessionUUID: "session-1",
		JoinCode:    "ABC123",
		ExpiresAt:   expiresAt,
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_created_when_game_is_playable": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.games.EXPECT().IsPlayable(ctx, gameUUID, userUUID).Return(true, nil)
			deps.sessions.EXPECT().CreateSession(ctx, userUUID, gameUUID, key).Return(created, nil)
			return test{
				ctx: ctx, userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: key,
				expectedResult: created,
			}
		},
		"game_not_playable_does_not_create_a_session": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.games.EXPECT().IsPlayable(ctx, gameUUID, userUUID).Return(false, nil)
			return test{
				ctx: ctx, userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: key,
				expectedResult: sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable},
			}
		},
		"playability_check_fails": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.games.EXPECT().IsPlayable(ctx, gameUUID, userUUID).Return(false, errors.New("game store down"))
			return test{
				ctx: ctx, userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: key,
				expectErr: true,
			}
		},
		"session_declines_the_game": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			declined := sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}
			deps.games.EXPECT().IsPlayable(ctx, gameUUID, userUUID).Return(true, nil)
			deps.sessions.EXPECT().CreateSession(ctx, userUUID, gameUUID, key).Return(declined, nil)
			return test{
				ctx: ctx, userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: key,
				expectedResult: declined,
			}
		},
		"session_reports_idempotency_conflict": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.games.EXPECT().IsPlayable(ctx, gameUUID, userUUID).Return(true, nil)
			deps.sessions.EXPECT().CreateSession(ctx, userUUID, gameUUID, key).Return(sessionpkg.CreateSessionResult{}, sessionpkg.ErrIdempotencyConflict)
			return test{
				ctx: ctx, userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: key,
				expectErr: true, expectedErrorIs: sessionpkg.ErrIdempotencyConflict,
			}
		},
		"session_creation_fails": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.games.EXPECT().IsPlayable(ctx, gameUUID, userUUID).Return(true, nil)
			deps.sessions.EXPECT().CreateSession(ctx, userUUID, gameUUID, key).Return(sessionpkg.CreateSessionResult{}, errors.New("database down"))
			return test{
				ctx: ctx, userUUID: userUUID, gameUUID: gameUUID, idempotencyKey: key,
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{
				games:    NewMockgamePlayabilityChecker(ctrl),
				sessions: NewMocksessionCreator(ctrl),
			}
			tc := setup(t, deps)

			result, err := New(deps.games, deps.sessions).CreateSession(tc.ctx, tc.userUUID, tc.gameUUID, tc.idempotencyKey)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expectedResult, result)
		})
	}
}
