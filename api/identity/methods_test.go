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

var caller = identitypkg.User{UUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", Level: identitypkg.LevelAccount}

func callerContext() context.Context {
	return auth.WithCaller(context.Background(), caller)
}

func TestHandleListMethods(t *testing.T) {
	added := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, reader *MockmethodReader) (int, string){
		"the caller's own ways in": func(t *testing.T, reader *MockmethodReader) (int, string) {
			reader.EXPECT().ListMethods(gomock.Any(), caller.UUID).Return([]identitypkg.SignInMethod{
				{MethodID: "4", Issuer: identitypkg.ProviderGoogle, Address: "ana@example.com", Name: "Ana", AddedAt: added},
				{MethodID: "9", Issuer: identitypkg.ProviderEmail, Address: "ana@other.com", AddedAt: added},
			}, nil)
			return http.StatusOK, `{"methods":[
				{"method_id":"4","issuer":"google","address":"ana@example.com","name":"Ana","added_at":"2026-11-16T09:00:00Z"},
				{"method_id":"9","issuer":"email","address":"ana@other.com","name":"","added_at":"2026-11-16T09:00:00Z"}]}`
		},
		"an account with none answers an empty list": func(t *testing.T, reader *MockmethodReader) (int, string) {
			reader.EXPECT().ListMethods(gomock.Any(), caller.UUID).Return(nil, nil)
			return http.StatusOK, `{"methods":[]}`
		},
		"identity_failure_is_transient": func(t *testing.T, reader *MockmethodReader) (int, string) {
			reader.EXPECT().ListMethods(gomock.Any(), caller.UUID).Return(nil, errors.New("db down"))
			return http.StatusServiceUnavailable, `{"message":"reading the sign-in methods failed"}`
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			reader := NewMockmethodReader(gomock.NewController(t))
			expectedCode, expectedBody := setup(t, reader)

			request := httptest.NewRequest(http.MethodGet, "/v1/identity/me/methods", nil).WithContext(callerContext())
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{}, Methods{Reader: reader}).handleListMethods(recorder, request)

			require.Equal(t, expectedCode, recorder.Code)
			require.JSONEq(t, expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleRemoveMethod(t *testing.T) {
	tests := map[string]struct {
		outcome      identitypkg.RemoveMethodOutcome
		failure      error
		expectedCode int
		expectedBody string
	}{
		"removed": {outcome: identitypkg.MethodRemoved, expectedCode: http.StatusOK, expectedBody: `{}`},
		"unknown": {
			outcome: identitypkg.MethodUnknown, expectedCode: http.StatusNotFound,
			expectedBody: `{"code":"no_such_method","message":"this account has no such sign-in method"}`,
		},
		"the last one stays": {
			outcome: identitypkg.MethodLastOne, expectedCode: http.StatusConflict,
			expectedBody: `{"code":"last_method","message":"add another way in before removing this one"}`,
		},
		"identity_failure_is_transient": {
			failure: errors.New("db down"), expectedCode: http.StatusServiceUnavailable,
			expectedBody: `{"message":"removing the sign-in method failed"}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			remover := NewMockmethodRemover(gomock.NewController(t))
			remover.EXPECT().RemoveMethod(gomock.Any(), caller.UUID, "9").Return(tc.outcome, tc.failure)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/me/methods/9/remove", nil).WithContext(callerContext())
			request.SetPathValue("methodID", "9")
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{}, Methods{Remover: remover}).handleRemoveMethod(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleCompleteLinkEmail(t *testing.T) {
	tests := map[string]struct {
		outcome      identitypkg.LinkMethodOutcome
		expectedCode int
		expectedBody string
	}{
		"linked": {outcome: identitypkg.MethodLinked, expectedCode: http.StatusOK, expectedBody: `{}`},
		"another account's address is never merged": {
			outcome: identitypkg.MethodInUse, expectedCode: http.StatusConflict,
			expectedBody: `{"code":"method_in_use","message":"that way in belongs to another account; sign in with it instead"}`,
		},
		"a wrong code": {
			outcome: identitypkg.MethodInvalidCode, expectedCode: http.StatusBadRequest,
			expectedBody: `{"code":"invalid_code","message":"that code is not the one sent"}`,
		},
		"a spent attempt": {
			outcome: identitypkg.MethodAttemptGone, expectedCode: http.StatusGone,
			expectedBody: `{"code":"attempt_gone","message":"that attempt is no longer valid; start again"}`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			linker := NewMockemailLinker(gomock.NewController(t))
			linker.EXPECT().CompleteLinkEmail(gomock.Any(), caller.UUID, "ana@other.com", "123456").
				Return(identitypkg.LinkMethodResult{Outcome: tc.outcome}, nil)

			body := `{"email":"ana@other.com","code":"123456"}`
			request := httptest.NewRequest(http.MethodPost, "/v1/identity/me/methods/email/complete", strings.NewReader(body)).WithContext(callerContext())
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, nil, nil, Logins{}, EmailLogins{Linker: linker}, Methods{}).handleCompleteLinkEmail(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}

func TestHandleLinkProvider(t *testing.T) {
	tests := map[string]func(t *testing.T, linker *MockproviderLinker) (string, int, string){
		"starting answers where to prove the identity": func(t *testing.T, linker *MockproviderLinker) (string, int, string) {
			linker.EXPECT().StartLinkProvider(gomock.Any(), caller.UUID, "local", "challenge").
				Return(identitypkg.StartLoginResult{Outcome: identitypkg.LoginStarted, AuthorizationURL: "/authorize?state=s"}, nil)
			return "start", http.StatusOK, `{"authorization_url":"/authorize?state=s"}`
		},
		"an unknown provider is refused": func(t *testing.T, linker *MockproviderLinker) (string, int, string) {
			linker.EXPECT().StartLinkProvider(gomock.Any(), caller.UUID, "local", "challenge").
				Return(identitypkg.StartLoginResult{Outcome: identitypkg.LoginProviderUnavailable}, nil)
			return "start", http.StatusBadRequest, `{"code":"provider_unavailable","message":"this sign-in provider is not available"}`
		},
		"completing links the identity": func(t *testing.T, linker *MockproviderLinker) (string, int, string) {
			linker.EXPECT().CompleteLinkProvider(gomock.Any(), caller.UUID, "phl_code", "verifier").
				Return(identitypkg.LinkMethodResult{Outcome: identitypkg.MethodLinked}, nil)
			return "complete", http.StatusOK, `{}`
		},
		"completing refuses another account's identity": func(t *testing.T, linker *MockproviderLinker) (string, int, string) {
			linker.EXPECT().CompleteLinkProvider(gomock.Any(), caller.UUID, "phl_code", "verifier").
				Return(identitypkg.LinkMethodResult{Outcome: identitypkg.MethodInUse}, nil)
			return "complete", http.StatusConflict, `{"code":"method_in_use","message":"that way in belongs to another account; sign in with it instead"}`
		},
	}

	body := `{"provider":"local","code_challenge":"challenge","code":"phl_code","code_verifier":"verifier"}`
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			linker := NewMockproviderLinker(gomock.NewController(t))
			step, expectedCode, expectedBody := setup(t, linker)

			request := httptest.NewRequest(http.MethodPost, "/v1/identity/me/methods/provider/"+step, strings.NewReader(body)).WithContext(callerContext())
			recorder := httptest.NewRecorder()
			handler := NewHandler(nil, nil, nil, nil, nil, nil, Logins{Linker: linker}, EmailLogins{}, Methods{})
			if step == "start" {
				handler.handleStartLinkProvider(recorder, request)
			} else {
				handler.handleCompleteLinkProvider(recorder, request)
			}

			require.Equal(t, expectedCode, recorder.Code)
			require.JSONEq(t, expectedBody, recorder.Body.String())
		})
	}
}
