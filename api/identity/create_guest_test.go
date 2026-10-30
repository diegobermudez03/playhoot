package identity

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

	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

func TestHandleCreateGuest(t *testing.T) {
	type dependencies struct {
		guests *MockguestCreator
	}
	type test struct {
		body         string
		expectedCode int
		expectedBody string
	}

	const (
		key      = "4f6c8a2e-1b3d-4e5f-9a7b-0c1d2e3f4a5b"
		userUUID = "aaaaaaaa-aaaa-4aaa-8aaa-0000000000ff"
	)
	ctx := context.Background()
	validBody := `{"display_name":"Ana","idempotency_key":"` + key + `"}`
	credentials := identitypkg.Credentials{
		User:            identitypkg.User{UUID: userUUID, DisplayName: "Ana", Level: identitypkg.LevelGuest},
		AccessToken:     "access-jwt",
		AccessIssuedAt:  time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		AccessExpiresAt: time.Date(2026, 10, 27, 12, 15, 0, 0, time.UTC),
		RefreshToken:    "phr_refresh",
	}
	credentialsJSON := `{"user":{"user_uuid":"` + userUUID + `","display_name":"Ana","level":"guest"},` +
		`"access_token":"access-jwt","access_expires_at":"2026-10-27T12:15:00Z","access_expires_in":900,"refresh_token":"phr_refresh"}`

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"guest_created": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{Outcome: identitypkg.GuestCreated, Credentials: credentials}, nil)
			return test{
				body:         validBody,
				expectedCode: http.StatusCreated,
				expectedBody: credentialsJSON,
			}
		},
		"idempotency_key_is_normalized": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{Outcome: identitypkg.GuestCreated, Credentials: credentials}, nil)
			return test{
				body:         `{"display_name":"Ana","idempotency_key":"` + strings.ToUpper(key) + `"}`,
				expectedCode: http.StatusCreated,
				expectedBody: credentialsJSON,
			}
		},
		"display_name_rejected_by_identity": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{Outcome: identitypkg.GuestInvalidDisplayName}, nil)
			return test{
				body:         validBody,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"code":"invalid_display_name","message":"display_name is not allowed"}`,
			}
		},
		"idempotency_conflict": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{}, identitypkg.ErrIdempotencyConflict)
			return test{
				body:         validBody,
				expectedCode: http.StatusConflict,
				expectedBody: `{"code":"idempotency_conflict","message":"idempotency key reused with a conflicting request"}`,
			}
		},
		"identity_fails": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{}, errors.New("identity down"))
			return test{
				body:         validBody,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"creating guest failed"}`,
			}
		},
		"key_spent": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{Outcome: identitypkg.GuestIdempotencyKeySpent}, nil)
			return test{
				body:         validBody,
				expectedCode: http.StatusConflict,
				expectedBody: `{"code":"idempotency_key_spent","message":"idempotency key already used; start a new attempt"}`,
			}
		},
		"created_without_a_refresh_token": func(t *testing.T, deps dependencies) test {
			incomplete := credentials
			incomplete.RefreshToken = ""
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{Outcome: identitypkg.GuestCreated, Credentials: incomplete}, nil)
			return test{
				body:         validBody,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"creating guest failed"}`,
			}
		},
		"unknown_outcome": func(t *testing.T, deps dependencies) test {
			deps.guests.EXPECT().CreateGuest(ctx, "Ana", key).
				Return(identitypkg.CreateGuestResult{Outcome: "something_new"}, nil)
			return test{
				body:         validBody,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"creating guest failed"}`,
			}
		},
		"idempotency_key_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{
				body:         `{"display_name":"Ana","idempotency_key":"key-1"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"idempotency_key must be a valid UUID"}`,
			}
		},
		"display_name_missing": func(t *testing.T, deps dependencies) test {
			return test{
				body:         `{"idempotency_key":"` + key + `"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"display_name is required and must be at most 64 characters"}`,
			}
		},
		"display_name_too_long": func(t *testing.T, deps dependencies) test {
			return test{
				body:         `{"display_name":"` + strings.Repeat("ñ", maxDisplayNameLength+1) + `","idempotency_key":"` + key + `"}`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"display_name is required and must be at most 64 characters"}`,
			}
		},
		"invalid_body": func(t *testing.T, deps dependencies) test {
			return test{
				body:         `not json`,
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"invalid request body"}`,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{guests: NewMockguestCreator(ctrl)}
			tc := setup(t, deps)

			request := httptest.NewRequest(http.MethodPost, "/v1/guests", strings.NewReader(tc.body)).WithContext(ctx)
			recorder := httptest.NewRecorder()

			NewHandler(deps.guests, nil, nil, nil, nil, nil, Logins{}).handleCreateGuest(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			if tc.expectedCode == http.StatusCreated {
				require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}
