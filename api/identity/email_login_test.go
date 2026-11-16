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
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

func TestHandleStartEmailLogin(t *testing.T) {
	type test struct {
		body         string
		language     string
		expectedCode int
		expectedBody string
	}

	ctx := context.Background()

	tests := map[string]func(t *testing.T, starter *MockemailLoginStarter) test{
		"a known and an unknown address answer the same": func(t *testing.T, starter *MockemailLoginStarter) test {
			starter.EXPECT().StartEmailLogin(ctx, "ana@example.com", "es", "").
				Return(identitypkg.StartEmailLoginResult{Outcome: identitypkg.EmailLoginAcknowledged}, nil)
			return test{body: `{"email":"ana@example.com","language":"es"}`, expectedCode: http.StatusOK, expectedBody: `{}`}
		},
		"an unsupported language falls back to what the browser asks": func(t *testing.T, starter *MockemailLoginStarter) test {
			starter.EXPECT().StartEmailLogin(ctx, "ana@example.com", "pt", "").
				Return(identitypkg.StartEmailLoginResult{Outcome: identitypkg.EmailLoginAcknowledged}, nil)
			return test{
				body: `{"email":"ana@example.com","language":"fr"}`, language: "fr-FR,pt-BR;q=0.9",
				expectedCode: http.StatusOK, expectedBody: `{}`,
			}
		},
		"a value that is not an address is refused": func(t *testing.T, starter *MockemailLoginStarter) test {
			starter.EXPECT().StartEmailLogin(ctx, "nope", "", "").
				Return(identitypkg.StartEmailLoginResult{Outcome: identitypkg.EmailLoginInvalidAddress}, nil)
			return test{
				body: `{"email":"nope"}`, expectedCode: http.StatusBadRequest,
				expectedBody: `{"code":"invalid_address","message":"that is not an email address"}`,
			}
		},
		"identity_failure_is_transient": func(t *testing.T, starter *MockemailLoginStarter) test {
			starter.EXPECT().StartEmailLogin(ctx, "ana@example.com", "", "").
				Return(identitypkg.StartEmailLoginResult{}, errors.New("db down"))
			return test{
				body: `{"email":"ana@example.com"}`, expectedCode: http.StatusServiceUnavailable,
				expectedBody: `{"message":"asking for the code failed"}`,
			}
		},
		"invalid_body": func(t *testing.T, _ *MockemailLoginStarter) test {
			return test{body: `{`, expectedCode: http.StatusBadRequest, expectedBody: `{"message":"invalid request body"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			starter := NewMockemailLoginStarter(gomock.NewController(t))
			tc := setup(t, starter)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/email/start", strings.NewReader(tc.body)).WithContext(ctx)
			if tc.language != "" {
				request.Header.Set("Accept-Language", tc.language)
			}
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{Starter: starter}, Methods{}).handleStartEmailLogin(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleStartEmailLoginAsAGuest(t *testing.T) {
	guest := identitypkg.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", Level: identitypkg.LevelGuest}
	account := identitypkg.User{UUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", Level: identitypkg.LevelAccount}

	tests := map[string]struct {
		caller   *identitypkg.User
		expected string
	}{
		"a guest becomes the account the code creates": {caller: &guest, expected: guest.UUID},
		"an account signing in again upgrades nobody":  {caller: &account, expected: ""},
		"an anonymous caller upgrades nobody":          {expected: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			starter := NewMockemailLoginStarter(gomock.NewController(t))
			ctx := context.Background()
			if tc.caller != nil {
				ctx = auth.WithCaller(ctx, *tc.caller)
			}
			starter.EXPECT().StartEmailLogin(gomock.Any(), "ana@example.com", "", tc.expected).
				Return(identitypkg.StartEmailLoginResult{Outcome: identitypkg.EmailLoginAcknowledged}, nil)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/email/start", strings.NewReader(`{"email":"ana@example.com"}`)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{Starter: starter}, Methods{}).handleStartEmailLogin(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
		})
	}
}

func TestHandleCompleteEmailLogin(t *testing.T) {
	type test struct {
		expectedCode int
		expectedBody string
	}

	ctx := context.Background()
	body := `{"email":"ana@example.com","code":"123456","replaces_refresh_token":"phr_guest"}`
	issued := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	credentials := identitypkg.Credentials{
		User:        identitypkg.User{UUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", Level: identitypkg.LevelAccount},
		AccessToken: "access", AccessIssuedAt: issued, AccessExpiresAt: issued.Add(15 * time.Minute),
		RefreshToken: "phr_next",
	}

	tests := map[string]func(t *testing.T, completer *MockemailLoginCompleter) test{
		"completed": func(t *testing.T, completer *MockemailLoginCompleter) test {
			completer.EXPECT().CompleteEmailLogin(ctx, "ana@example.com", "123456", "phr_guest").
				Return(identitypkg.CompleteEmailLoginResult{Outcome: identitypkg.EmailLoginCompleted, Credentials: credentials}, nil)
			return test{
				expectedCode: http.StatusOK,
				expectedBody: `{"user":{"user_uuid":"7c9e6679-7425-40de-944b-e07fc1f90ae7","display_name":"","level":"account","mode":"normal"},
					"access_token":"access","access_expires_at":"2026-11-16T12:15:00Z","access_expires_in":900,"refresh_token":"phr_next"}`,
			}
		},
		"a wrong code leaves the attempt alive": func(t *testing.T, completer *MockemailLoginCompleter) test {
			completer.EXPECT().CompleteEmailLogin(ctx, "ana@example.com", "123456", "phr_guest").
				Return(identitypkg.CompleteEmailLoginResult{Outcome: identitypkg.EmailLoginInvalidCode}, nil)
			return test{
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"code":"invalid_code","message":"that code is not the one sent to this address"}`,
			}
		},
		"a spent attempt asks for a new code": func(t *testing.T, completer *MockemailLoginCompleter) test {
			completer.EXPECT().CompleteEmailLogin(ctx, "ana@example.com", "123456", "phr_guest").
				Return(identitypkg.CompleteEmailLoginResult{Outcome: identitypkg.EmailLoginAttemptGone}, nil)
			return test{
				expectedCode: http.StatusGone,
				expectedBody: `{"code":"attempt_gone","message":"that code is no longer valid; ask for a new one"}`,
			}
		},
		"identity_failure_is_transient": func(t *testing.T, completer *MockemailLoginCompleter) test {
			completer.EXPECT().CompleteEmailLogin(ctx, "ana@example.com", "123456", "phr_guest").
				Return(identitypkg.CompleteEmailLoginResult{}, errors.New("db down"))
			return test{expectedCode: http.StatusServiceUnavailable, expectedBody: `{"message":"completing the sign-in failed"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			completer := NewMockemailLoginCompleter(gomock.NewController(t))
			tc := setup(t, completer)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/email/complete", strings.NewReader(body)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{Completer: completer}, Methods{}).handleCompleteEmailLogin(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestEmailCodeRoutesAreCappedPerClientAddress(t *testing.T) {
	expected := map[string]ratelimit.Limit{
		"POST /v1/identity/login/email/start":      {Count: 30, Per: time.Hour},
		"POST /v1/identity/login/email/complete":   {Count: 10, Per: time.Minute},
		"POST /v1/identity/me/methods/email/start": {Count: 30, Per: time.Hour},
	}
	seen := 0
	for _, route := range NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{}, Methods{}).RESTRoutes() {
		limit, capped := expected[route.Pattern]
		if !capped {
			continue
		}
		seen++
		require.NotNil(t, route.Limits, route.Pattern)
		require.Contains(t, route.Limits.PerClient, limit, route.Pattern)
	}
	require.Equal(t, len(expected), seen, "every email code route is declared")
}
