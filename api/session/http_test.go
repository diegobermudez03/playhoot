package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

func TestHandleCreateSession(t *testing.T) {
	type dependencies struct {
		creator *MocksessionCreator
	}
	type test struct {
		ctx          context.Context
		body         string
		expectedCode int
		expectedBody string
	}

	const (
		callerUUID = identity.UserUUID("0f8fad5b-d9cb-469f-a165-70867728950e")
		gameUUID   = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)
	expiresAt := time.Date(2026, 10, 14, 12, 30, 0, 0, time.UTC)
	callerCtx := func() context.Context {
		return auth.WithUserUUID(context.Background(), callerUUID)
	}
	validBody := `{"game_uuid":"` + gameUUID + `","idempotency_key":"key-1"}`

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_created": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{
					Outcome:     sessionpkg.CreateSessionCreated,
					SessionUUID: "session-1",
					JoinCode:    "ABC123",
					ExpiresAt:   expiresAt,
				}, nil)
			return test{
				ctx:          ctx,
				body:         validBody,
				expectedCode: http.StatusCreated,
				expectedBody: `{"session_uuid":"session-1","join_code":"ABC123","lobby_expires_at":"2026-10-14T12:30:00Z"}`,
			}
		},
		"game_uuid_is_normalized_before_calling_creator": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{
					Outcome:     sessionpkg.CreateSessionCreated,
					SessionUUID: "session-1",
					JoinCode:    "ABC123",
					ExpiresAt:   expiresAt,
				}, nil)
			return test{
				ctx:          ctx,
				body:         `{"game_uuid":"` + strings.ToUpper(gameUUID) + `","idempotency_key":"key-1"}`,
				expectedCode: http.StatusCreated,
				expectedBody: `{"session_uuid":"session-1","join_code":"ABC123","lobby_expires_at":"2026-10-14T12:30:00Z"}`,
			}
		},
		"idempotency_key_at_max_length": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			key := strings.Repeat("k", maxIdempotencyKeyLength)
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey(key)).
				Return(sessionpkg.CreateSessionResult{
					Outcome:     sessionpkg.CreateSessionCreated,
					SessionUUID: "session-1",
					JoinCode:    "ABC123",
					ExpiresAt:   expiresAt,
				}, nil)
			return test{
				ctx:          ctx,
				body:         `{"game_uuid":"` + gameUUID + `","idempotency_key":"` + key + `"}`,
				expectedCode: http.StatusCreated,
				expectedBody: `{"session_uuid":"session-1","join_code":"ABC123","lobby_expires_at":"2026-10-14T12:30:00Z"}`,
			}
		},
		"game_unavailable": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}, nil)
			return test{
				ctx:          ctx,
				body:         validBody,
				expectedCode: http.StatusNotFound,
				expectedBody: `{"message":"game not found or not currently playable"}`,
			}
		},
		"idempotency_conflict": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{}, sessionpkg.ErrIdempotencyConflict)
			return test{
				ctx:          ctx,
				body:         validBody,
				expectedCode: http.StatusConflict,
				expectedBody: `{"message":"idempotency key reused with a conflicting request"}`,
			}
		},
		"idempotency_conflict_wrapped": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{}, errors.Join(errors.New("claiming"), sessionpkg.ErrIdempotencyConflict))
			return test{
				ctx:          ctx,
				body:         validBody,
				expectedCode: http.StatusConflict,
				expectedBody: `{"message":"idempotency key reused with a conflicting request"}`,
			}
		},
		"creator_fails": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{}, errors.New("database down"))
			return test{
				ctx:          ctx,
				body:         validBody,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"creating session failed"}`,
			}
		},
		"creator_returns_unknown_outcome": func(t *testing.T, deps dependencies) test {
			ctx := callerCtx()
			deps.creator.EXPECT().
				CreateSession(ctx, callerUUID, sessionpkg.GameUUID(gameUUID), sessionpkg.IdempotencyKey("key-1")).
				Return(sessionpkg.CreateSessionResult{Outcome: "something_new"}, nil)
			return test{
				ctx:          ctx,
				body:         validBody,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"creating session failed"}`,
			}
		},
		"no_authorized_caller_in_context": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          context.Background(),
				body:         validBody,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"creating session failed"}`,
			}
		},
		"body_is_not_json": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          callerCtx(),
				body:         `not json`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"invalid request body"}`,
			}
		},
		"body_over_size_limit": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          callerCtx(),
				body:         `{"game_uuid":"` + gameUUID + `","idempotency_key":"` + strings.Repeat("k", maxCreateSessionBodyBytes) + `"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"invalid request body"}`,
			}
		},
		"game_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          callerCtx(),
				body:         `{"game_uuid":"not-a-uuid","idempotency_key":"key-1"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"game_uuid must be a valid UUID"}`,
			}
		},
		"game_uuid_missing": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          callerCtx(),
				body:         `{"idempotency_key":"key-1"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"game_uuid must be a valid UUID"}`,
			}
		},
		"idempotency_key_missing": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          callerCtx(),
				body:         `{"game_uuid":"` + gameUUID + `"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"idempotency_key is required and must be at most 128 characters"}`,
			}
		},
		"idempotency_key_too_long": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          callerCtx(),
				body:         `{"game_uuid":"` + gameUUID + `","idempotency_key":"` + strings.Repeat("k", maxIdempotencyKeyLength+1) + `"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"idempotency_key is required and must be at most 128 characters"}`,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{creator: NewMocksessionCreator(ctrl)}
			tc := setup(t, deps)

			handler := NewHandler(nil, deps.creator)
			request := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(tc.body)).WithContext(tc.ctx)
			recorder := httptest.NewRecorder()

			handler.handleCreateSession(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}
