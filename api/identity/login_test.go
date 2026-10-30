package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

func TestHandleStartLogin(t *testing.T) {
	type test struct {
		body         string
		expectedCode int
		expectedBody string
	}

	ctx := context.Background()
	body := `{"provider":"local","code_challenge":"challenge"}`

	tests := map[string]func(t *testing.T, starter *MockloginStarter) test{
		"started": func(t *testing.T, starter *MockloginStarter) test {
			starter.EXPECT().StartLogin(ctx, "local", "challenge", "").
				Return(identitypkg.StartLoginResult{Outcome: identitypkg.LoginStarted, AuthorizationURL: "/authorize?state=s"}, nil)
			return test{body: body, expectedCode: http.StatusOK, expectedBody: `{"authorization_url":"/authorize?state=s"}`}
		},
		"provider_unavailable": func(t *testing.T, starter *MockloginStarter) test {
			starter.EXPECT().StartLogin(ctx, "local", "challenge", "").
				Return(identitypkg.StartLoginResult{Outcome: identitypkg.LoginProviderUnavailable}, nil)
			return test{body: body, expectedCode: http.StatusBadRequest, expectedBody: `{"code":"provider_unavailable","message":"this sign-in provider is not available"}`}
		},
		"invalid_code_challenge": func(t *testing.T, starter *MockloginStarter) test {
			starter.EXPECT().StartLogin(ctx, "local", "challenge", "").
				Return(identitypkg.StartLoginResult{Outcome: identitypkg.LoginInvalidCodeChallenge}, nil)
			return test{body: body, expectedCode: http.StatusBadRequest, expectedBody: `{"code":"invalid_code_challenge","message":"code_challenge must be an S256 hash"}`}
		},
		"identity_failure_is_transient": func(t *testing.T, starter *MockloginStarter) test {
			starter.EXPECT().StartLogin(ctx, "local", "challenge", "").Return(identitypkg.StartLoginResult{}, errors.New("db down"))
			return test{body: body, expectedCode: http.StatusServiceUnavailable, expectedBody: `{"message":"starting the sign-in failed"}`}
		},
		"invalid_body": func(t *testing.T, starter *MockloginStarter) test {
			return test{body: `{`, expectedCode: http.StatusBadRequest, expectedBody: `{"message":"invalid request body"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			starter := NewMockloginStarter(gomock.NewController(t))
			tc := setup(t, starter)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/start", strings.NewReader(tc.body)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{Starter: starter}).handleStartLogin(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleStartLoginAsAGuest(t *testing.T) {
	guest := identitypkg.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", Level: identitypkg.LevelGuest}
	account := identitypkg.User{UUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", Level: identitypkg.LevelAccount}
	started := identitypkg.StartLoginResult{Outcome: identitypkg.LoginStarted, AuthorizationURL: "/authorize?state=s"}

	for name, tc := range map[string]struct {
		caller        identitypkg.User
		expectedGuest string
	}{
		"a_guest_is_named_so_a_new_account_is_theirs": {caller: guest, expectedGuest: guest.UUID},
		"an_account_links_nothing":                    {caller: account, expectedGuest: ""},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := auth.WithCaller(context.Background(), tc.caller)
			starter := NewMockloginStarter(gomock.NewController(t))
			starter.EXPECT().StartLogin(ctx, "local", "challenge", tc.expectedGuest).Return(started, nil)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/start", strings.NewReader(`{"provider":"local","code_challenge":"challenge"}`)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{Starter: starter}).handleStartLogin(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
		})
	}
}

func TestHandleLoginCallback(t *testing.T) {
	type test struct {
		query            string
		expectedLocation string
	}

	tests := map[string]func(t *testing.T, callbacks *MockloginCallbackHandler) test{
		"code_issued_goes_to_the_completion_route_with_it": func(t *testing.T, callbacks *MockloginCallbackHandler) test {
			callbacks.EXPECT().HandleCallback(gomock.Any(), "local", "s", "Ana", "").
				Return(identitypkg.LoginCallbackResult{Outcome: identitypkg.LoginCodeIssued, LoginCode: "phl_code"}, nil)
			return test{query: "state=s&code=Ana", expectedLocation: "/sign-in/complete?code=phl_code"}
		},
		"cancelled": func(t *testing.T, callbacks *MockloginCallbackHandler) test {
			callbacks.EXPECT().HandleCallback(gomock.Any(), "local", "s", "", "access_denied").
				Return(identitypkg.LoginCallbackResult{Outcome: identitypkg.LoginCancelled}, nil)
			return test{query: "state=s&error=access_denied", expectedLocation: "/sign-in/complete?error=cancelled"}
		},
		"failed": func(t *testing.T, callbacks *MockloginCallbackHandler) test {
			callbacks.EXPECT().HandleCallback(gomock.Any(), "local", "", "", "").
				Return(identitypkg.LoginCallbackResult{Outcome: identitypkg.LoginFailed}, nil)
			return test{query: "", expectedLocation: "/sign-in/complete?error=failed"}
		},
		"provider_unreachable": func(t *testing.T, callbacks *MockloginCallbackHandler) test {
			callbacks.EXPECT().HandleCallback(gomock.Any(), "local", "s", "Ana", "").
				Return(identitypkg.LoginCallbackResult{Outcome: identitypkg.LoginUnavailable}, nil)
			return test{query: "state=s&code=Ana", expectedLocation: "/sign-in/complete?error=unavailable"}
		},
		"identity_failure": func(t *testing.T, callbacks *MockloginCallbackHandler) test {
			callbacks.EXPECT().HandleCallback(gomock.Any(), "local", "s", "Ana", "").Return(identitypkg.LoginCallbackResult{}, errors.New("db down"))
			return test{query: "state=s&code=Ana", expectedLocation: "/sign-in/complete?error=unavailable"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			callbacks := NewMockloginCallbackHandler(gomock.NewController(t))
			tc := setup(t, callbacks)

			request := httptest.NewRequest(http.MethodGet, "/v1/identity/login/local/callback?"+tc.query, nil)
			request.SetPathValue("provider", "local")
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{Callbacks: callbacks, CompleteURL: "/sign-in/complete"}).handleLoginCallback(recorder, request)

			require.Equal(t, http.StatusFound, recorder.Code)
			require.Equal(t, tc.expectedLocation, recorder.Header().Get("Location"))
			require.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
			require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		})
	}
}

func TestHandleCompleteLogin(t *testing.T) {
	type test struct {
		body         string
		expectedCode int
		expectedBody string
	}

	ctx := context.Background()
	body := `{"code":"phl_code","code_verifier":"verifier","replaces_refresh_token":"phr_guest"}`
	account := renewed
	account.User.Level = identitypkg.LevelAccount

	tests := map[string]func(t *testing.T, completer *MockloginCompleter) test{
		"completed": func(t *testing.T, completer *MockloginCompleter) test {
			completer.EXPECT().CompleteLogin(ctx, "phl_code", "verifier", "phr_guest").
				Return(identitypkg.CompleteLoginResult{Outcome: identitypkg.LoginCompleted, Credentials: account}, nil)
			return test{body: body, expectedCode: http.StatusOK, expectedBody: `{"user":{"user_uuid":"0f8fad5b-d9cb-469f-a165-70867728950e","display_name":"Ana","level":"account"},` +
				`"access_token":"access-jwt","access_expires_at":"2026-10-27T12:15:00Z","access_expires_in":900,"refresh_token":"phr_refresh"}`}
		},
		"invalid_code": func(t *testing.T, completer *MockloginCompleter) test {
			completer.EXPECT().CompleteLogin(ctx, "phl_code", "verifier", "phr_guest").
				Return(identitypkg.CompleteLoginResult{Outcome: identitypkg.LoginCodeInvalid}, nil)
			return test{body: body, expectedCode: http.StatusBadRequest, expectedBody: `{"code":"login_code_invalid","message":"the sign-in code is not valid; start the sign-in again"}`}
		},
		"identity_failure_is_transient": func(t *testing.T, completer *MockloginCompleter) test {
			completer.EXPECT().CompleteLogin(ctx, "phl_code", "verifier", "phr_guest").Return(identitypkg.CompleteLoginResult{}, errors.New("db down"))
			return test{body: body, expectedCode: http.StatusServiceUnavailable, expectedBody: `{"message":"completing the sign-in failed"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			completer := NewMockloginCompleter(gomock.NewController(t))
			tc := setup(t, completer)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/complete", strings.NewReader(tc.body)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{Completer: completer}).handleCompleteLogin(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleLocalAuthorize(t *testing.T) {
	t.Run("serves_the_page_where_enabled", func(t *testing.T) {
		pages := NewMocklocalPages(gomock.NewController(t))
		pages.EXPECT().LocalAuthorizePage("s").Return("<html>local</html>", true)

		recorder := httptest.NewRecorder()
		NewHandler(nil, nil, nil, nil, nil, nil, Logins{LocalPages: pages}).
			handleLocalAuthorize(recorder, httptest.NewRequest(http.MethodGet, "/v1/identity/login/local/authorize?state=s", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "<html>local</html>", recorder.Body.String())
		require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "default-src 'none'")
	})

	t.Run("not_found_where_disabled", func(t *testing.T) {
		pages := NewMocklocalPages(gomock.NewController(t))
		pages.EXPECT().LocalAuthorizePage("s").Return("", false)

		recorder := httptest.NewRecorder()
		NewHandler(nil, nil, nil, nil, nil, nil, Logins{LocalPages: pages}).
			handleLocalAuthorize(recorder, httptest.NewRequest(http.MethodGet, "/v1/identity/login/local/authorize?state=s", nil))

		require.Equal(t, http.StatusNotFound, recorder.Code)
	})
}
