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

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

var renewed = identitypkg.Credentials{
	User:            identitypkg.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", DisplayName: "Ana", Level: identitypkg.LevelGuest},
	AccessToken:     "access-jwt",
	AccessIssuedAt:  time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	AccessExpiresAt: time.Date(2026, 10, 27, 12, 15, 0, 0, time.UTC),
	RefreshToken:    "phr_refresh",
}

func TestHandleRefresh(t *testing.T) {
	type test struct {
		body         string
		expectedCode int
		expectedBody string
	}

	ctx := context.Background()

	tests := map[string]func(t *testing.T, refreshes *MockcredentialRefresher) test{
		"renewed": func(t *testing.T, refreshes *MockcredentialRefresher) test {
			refreshes.EXPECT().Refresh(ctx, "phr_refresh").
				Return(identitypkg.RefreshResult{Outcome: identitypkg.RefreshRenewed, Credentials: renewed}, nil)
			return test{
				body:         `{"refresh_token":"phr_refresh"}`,
				expectedCode: http.StatusOK,
				expectedBody: `{"user":{"user_uuid":"0f8fad5b-d9cb-469f-a165-70867728950e","display_name":"Ana","level":"guest"},` +
					`"access_token":"access-jwt","access_expires_at":"2026-10-27T12:15:00Z","access_expires_in":900,"refresh_token":"phr_refresh"}`,
			}
		},
		"invalid_refresh_token": func(t *testing.T, refreshes *MockcredentialRefresher) test {
			refreshes.EXPECT().Refresh(ctx, "phr_old").Return(identitypkg.RefreshResult{Outcome: identitypkg.RefreshInvalid}, nil)
			return test{
				body:         `{"refresh_token":"phr_old"}`,
				expectedCode: http.StatusUnauthorized,
				expectedBody: `{"code":"refresh_invalid","message":"the refresh token is no longer valid"}`,
			}
		},
		"identity_unavailable_is_not_an_invalid_token": func(t *testing.T, refreshes *MockcredentialRefresher) test {
			refreshes.EXPECT().Refresh(ctx, "phr_refresh").Return(identitypkg.RefreshResult{}, errors.New("database down"))
			return test{
				body:         `{"refresh_token":"phr_refresh"}`,
				expectedCode: http.StatusServiceUnavailable,
				expectedBody: `{"message":"renewing credentials failed"}`,
			}
		},
		"missing_refresh_token": func(t *testing.T, refreshes *MockcredentialRefresher) test {
			return test{body: `{}`, expectedCode: http.StatusBadRequest, expectedBody: `{"message":"a refresh_token is required"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			refreshes := NewMockcredentialRefresher(ctrl)
			tc := setup(t, refreshes)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/refresh", strings.NewReader(tc.body)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, refreshes, nil, nil, nil, Logins{}).handleRefresh(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleLogout(t *testing.T) {
	ctx := context.Background()

	t.Run("ends_the_session", func(t *testing.T) {
		sessions := NewMocksessionEnder(gomock.NewController(t))
		sessions.EXPECT().EndSession(ctx, "phr_refresh").Return(nil)

		request := httptest.NewRequest(http.MethodPost, "/v1/identity/logout", strings.NewReader(`{"refresh_token":"phr_refresh"}`)).WithContext(ctx)
		recorder := httptest.NewRecorder()
		NewHandler(nil, nil, nil, sessions, nil, nil, Logins{}).handleLogout(recorder, request)

		require.Equal(t, http.StatusNoContent, recorder.Code)
	})

	t.Run("missing_refresh_token", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/v1/identity/logout", strings.NewReader(`{}`)).WithContext(ctx)
		recorder := httptest.NewRecorder()
		NewHandler(nil, nil, nil, nil, nil, nil, Logins{}).handleLogout(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestHandleMe(t *testing.T) {
	ctx := auth.WithCaller(context.Background(), renewed.User)
	request := httptest.NewRequest(http.MethodGet, "/v1/identity/me", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()

	NewHandler(nil, nil, nil, nil, nil, nil, Logins{}).handleMe(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"user":{"user_uuid":"0f8fad5b-d9cb-469f-a165-70867728950e","display_name":"Ana","level":"guest"}}`, recorder.Body.String())
}
