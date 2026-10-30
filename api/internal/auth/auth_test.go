package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
)

func TestRequire(t *testing.T) {
	type dependencies struct {
		authorizer *Mockauthorizer
	}
	type test struct {
		ctx                context.Context
		authorization      string
		expectedCode       int
		expectedBody       string
		expectedAuthHeader string
		expectedNextCalled bool
		expectedCaller     identity.User
	}

	const (
		operation  = identity.OperationCreateSession
		callerUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		callerName = "Ana"
	)

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"allowed": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt-token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessAllowed, nil)
			return test{
				ctx:                ctx,
				authorization:      "Bearer jwt-token",
				expectedCode:       http.StatusNoContent,
				expectedNextCalled: true,
				expectedCaller:     identity.User{UUID: callerUUID, DisplayName: callerName},
			}
		},
		"allowed_with_lowercase_scheme": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt-token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessAllowed, nil)
			return test{
				ctx:                ctx,
				authorization:      "bearer jwt-token",
				expectedCode:       http.StatusNoContent,
				expectedNextCalled: true,
				expectedCaller:     identity.User{UUID: callerUUID, DisplayName: callerName},
			}
		},
		"missing_authorization_header": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"code":"token_required","message":"this endpoint requires a bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"authorization_scheme_is_not_bearer": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				authorization:      "Basic dXNlcjpwYXNz",
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"code":"token_required","message":"this endpoint requires a bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"bearer_without_token": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				authorization:      "Bearer ",
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"code":"token_required","message":"this endpoint requires a bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"identity_says_unauthenticated": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "expired", operation).
				Return(identity.User{}, identity.AccessUnauthenticated, nil)
			return test{
				ctx:                ctx,
				authorization:      "Bearer expired",
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"code":"token_invalid","message":"invalid or expired token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"identity_says_forbidden": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "guest-token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessForbidden, nil)
			return test{
				ctx:           ctx,
				authorization: "Bearer guest-token",
				expectedCode:  http.StatusForbidden,
				expectedBody:  `{"code":"operation_forbidden","message":"operation not allowed"}`,
			}
		},
		"identity_fails": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt-token", operation).
				Return(identity.User{}, identity.AccessDecision(""), errors.New("identity down"))
			return test{
				ctx:           ctx,
				authorization: "Bearer jwt-token",
				expectedCode:  http.StatusInternalServerError,
				expectedBody:  `{"message":"authorization failed"}`,
			}
		},
		"allowed_without_a_caller": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt-token", operation).
				Return(identity.User{}, identity.AccessAllowed, nil)
			return test{
				ctx:           ctx,
				authorization: "Bearer jwt-token",
				expectedCode:  http.StatusInternalServerError,
				expectedBody:  `{"message":"authorization failed"}`,
			}
		},
		"unknown_decision": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt-token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessDecision("something_new"), nil)
			return test{
				ctx:           ctx,
				authorization: "Bearer jwt-token",
				expectedCode:  http.StatusInternalServerError,
				expectedBody:  `{"message":"authorization failed"}`,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{authorizer: NewMockauthorizer(ctrl)}
			tc := setup(t, deps)

			nextCalled := false
			var caller identity.User
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				caller, _ = Caller(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}

			request := httptest.NewRequest(http.MethodPost, "/sessions", nil).WithContext(tc.ctx)
			if tc.authorization != "" {
				request.Header.Set("Authorization", tc.authorization)
			}
			recorder := httptest.NewRecorder()

			Require(deps.authorizer, operation)(next)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedNextCalled, nextCalled)
			require.Equal(t, tc.expectedCaller, caller)
			require.Equal(t, tc.expectedAuthHeader, recorder.Header().Get("WWW-Authenticate"))
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			} else {
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}

func TestOptional(t *testing.T) {
	const operation = identity.OperationViewOwnIdentity
	guest := identity.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", Level: identity.LevelGuest}

	run := func(t *testing.T, authorizer *Mockauthorizer, authorization string) (*httptest.ResponseRecorder, bool, identity.User, bool) {
		t.Helper()
		var nextCalled, identified bool
		var caller identity.User
		next := func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			caller, identified = Caller(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/identity/login/start", nil)
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		recorder := httptest.NewRecorder()
		Optional(authorizer, operation)(next)(recorder, request)
		return recorder, nextCalled, caller, identified
	}

	t.Run("without_a_token_goes_through_as_nobody", func(t *testing.T) {
		recorder, nextCalled, _, identified := run(t, NewMockauthorizer(gomock.NewController(t)), "")
		require.Equal(t, http.StatusNoContent, recorder.Code)
		require.True(t, nextCalled)
		require.False(t, identified)
	})

	t.Run("a_valid_token_names_the_caller", func(t *testing.T) {
		authorizer := NewMockauthorizer(gomock.NewController(t))
		authorizer.EXPECT().Authorize(gomock.Any(), "guest-jwt", operation).Return(guest, identity.AccessAllowed, nil)
		_, nextCalled, caller, identified := run(t, authorizer, "Bearer guest-jwt")
		require.True(t, nextCalled)
		require.True(t, identified)
		require.Equal(t, guest, caller)
	})

	t.Run("an_expired_token_is_refused_so_the_client_renews_it", func(t *testing.T) {
		authorizer := NewMockauthorizer(gomock.NewController(t))
		authorizer.EXPECT().Authorize(gomock.Any(), "expired-jwt", operation).Return(identity.User{}, identity.AccessUnauthenticated, nil)
		recorder, nextCalled, _, _ := run(t, authorizer, "Bearer expired-jwt")
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.False(t, nextCalled)
		require.JSONEq(t, `{"code":"token_invalid","message":"invalid or expired token"}`, recorder.Body.String())
	})
}

func TestRequireWebSocket(t *testing.T) {
	type dependencies struct {
		authorizer *Mockauthorizer
	}
	type test struct {
		ctx                context.Context
		authorization      string
		protocols          []string
		expectedCode       int
		expectedBody       string
		expectedAuthHeader string
		expectedNextCalled bool
		expectedCaller     identity.User
	}

	const (
		operation  = identity.OperationJoinSessionAsAdmin
		callerUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		callerName = "Ana"
	)

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"allowed_with_token_subprotocol": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt.token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessAllowed, nil)
			return test{
				ctx:                ctx,
				protocols:          []string{"playhoot.v1, " + bearerProtocol("jwt.token")},
				expectedCode:       http.StatusNoContent,
				expectedNextCalled: true,
				expectedCaller:     identity.User{UUID: callerUUID, DisplayName: callerName},
			}
		},
		"allowed_with_authorization_header": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "jwt.token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessAllowed, nil)
			return test{
				ctx:                ctx,
				authorization:      "Bearer jwt.token",
				protocols:          []string{"playhoot.v1"},
				expectedCode:       http.StatusNoContent,
				expectedNextCalled: true,
				expectedCaller:     identity.User{UUID: callerUUID, DisplayName: callerName},
			}
		},
		"forbidden_with_token_subprotocol": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, "guest-token", operation).
				Return(identity.User{UUID: callerUUID, DisplayName: callerName}, identity.AccessForbidden, nil)
			return test{
				ctx:          ctx,
				protocols:    []string{"playhoot.v1", bearerProtocol("guest-token")},
				expectedCode: http.StatusForbidden,
				expectedBody: `{"code":"operation_forbidden","message":"operation not allowed"}`,
			}
		},
		"malformed_token_subprotocol": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				protocols:          []string{"playhoot.v1, " + webSocketBearerProtocolPrefix + "not*base64"},
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"code":"token_malformed","message":"malformed bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"no_token_anywhere": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				protocols:          []string{"playhoot.v1"},
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"code":"token_required","message":"this endpoint requires a bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{authorizer: NewMockauthorizer(ctrl)}
			tc := setup(t, deps)

			nextCalled := false
			var caller identity.User
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				caller, _ = Caller(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}

			request := httptest.NewRequest(http.MethodGet, "/sessions/x/admin", nil).WithContext(tc.ctx)
			if tc.authorization != "" {
				request.Header.Set("Authorization", tc.authorization)
			}
			for _, protocols := range tc.protocols {
				request.Header.Add("Sec-WebSocket-Protocol", protocols)
			}
			recorder := httptest.NewRecorder()

			RequireWebSocket(deps.authorizer, operation)(next)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedNextCalled, nextCalled)
			require.Equal(t, tc.expectedCaller, caller)
			require.Equal(t, tc.expectedAuthHeader, recorder.Header().Get("WWW-Authenticate"))
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			} else {
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}

func TestWebSocketToken(t *testing.T) {
	type test struct {
		authorization string
		protocols     []string
		expectedToken string
		expectedErr   error
	}

	tests := map[string]func(t *testing.T) test{
		"token_subprotocol": func(t *testing.T) test {
			return test{protocols: []string{bearerProtocol("abc.def.ghi")}, expectedToken: "abc.def.ghi"}
		},
		"token_subprotocol_among_others_in_one_header": func(t *testing.T) test {
			return test{protocols: []string{"playhoot.v1,  " + bearerProtocol("abc") + " , other"}, expectedToken: "abc"}
		},
		"token_subprotocol_in_a_later_header": func(t *testing.T) test {
			return test{protocols: []string{"playhoot.v1", bearerProtocol("abc")}, expectedToken: "abc"}
		},
		"token_with_characters_a_subprotocol_cannot_carry_raw": func(t *testing.T) test {
			return test{protocols: []string{bearerProtocol("a b/c=d")}, expectedToken: "a b/c=d"}
		},
		"first_token_subprotocol_wins": func(t *testing.T) test {
			return test{protocols: []string{bearerProtocol("first") + ", " + bearerProtocol("second")}, expectedToken: "first"}
		},
		"authorization_header_wins_over_subprotocol": func(t *testing.T) test {
			return test{authorization: "Bearer from-header", protocols: []string{bearerProtocol("from-protocol")}, expectedToken: "from-header"}
		},
		"non_bearer_authorization_header_falls_back_to_subprotocol": func(t *testing.T) test {
			return test{authorization: "Basic abc", protocols: []string{bearerProtocol("from-protocol")}, expectedToken: "from-protocol"}
		},
		"padded_base64_is_malformed": func(t *testing.T) test {
			return test{protocols: []string{webSocketBearerProtocolPrefix + base64.URLEncoding.EncodeToString([]byte("ab"))}, expectedErr: errMalformedToken}
		},
		"not_base64_is_malformed": func(t *testing.T) test {
			return test{protocols: []string{webSocketBearerProtocolPrefix + "a+b"}, expectedErr: errMalformedToken}
		},
		"empty_token_is_malformed": func(t *testing.T) test {
			return test{protocols: []string{webSocketBearerProtocolPrefix}, expectedErr: errMalformedToken}
		},
		"blank_token_is_malformed": func(t *testing.T) test {
			return test{protocols: []string{bearerProtocol("   ")}, expectedErr: errMalformedToken}
		},
		"only_other_subprotocols": func(t *testing.T) test {
			return test{protocols: []string{"playhoot.v1"}, expectedErr: errMissingToken}
		},
		"nothing": func(t *testing.T) test {
			return test{expectedErr: errMissingToken}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.authorization != "" {
				request.Header.Set("Authorization", tc.authorization)
			}
			for _, protocols := range tc.protocols {
				request.Header.Add("Sec-WebSocket-Protocol", protocols)
			}

			token, err := webSocketToken(request)

			require.Equal(t, tc.expectedToken, token)
			require.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

// bearerProtocol is the subprotocol a browser offers to carry token.
func bearerProtocol(token string) string {
	return webSocketBearerProtocolPrefix + base64.RawURLEncoding.EncodeToString([]byte(token))
}

func TestBearerToken(t *testing.T) {
	type test struct {
		authorization string
		expectedToken string
		expectedOK    bool
	}

	tests := map[string]func(t *testing.T) test{
		"bearer_token": func(t *testing.T) test {
			return test{authorization: "Bearer abc.def.ghi", expectedToken: "abc.def.ghi", expectedOK: true}
		},
		"scheme_is_case_insensitive": func(t *testing.T) test {
			return test{authorization: "BEARER abc", expectedToken: "abc", expectedOK: true}
		},
		"surrounding_whitespace_is_trimmed": func(t *testing.T) test {
			return test{authorization: "Bearer   abc  ", expectedToken: "abc", expectedOK: true}
		},
		"no_header": func(t *testing.T) test {
			return test{authorization: ""}
		},
		"scheme_only": func(t *testing.T) test {
			return test{authorization: "Bearer"}
		},
		"empty_token": func(t *testing.T) test {
			return test{authorization: "Bearer    "}
		},
		"other_scheme": func(t *testing.T) test {
			return test{authorization: "Basic abc"}
		},
		"token_without_scheme": func(t *testing.T) test {
			return test{authorization: "abc"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.authorization != "" {
				request.Header.Set("Authorization", tc.authorization)
			}

			token, ok := bearerToken(request)

			require.Equal(t, tc.expectedToken, token)
			require.Equal(t, tc.expectedOK, ok)
		})
	}
}

func TestUserUUID(t *testing.T) {
	type test struct {
		ctx           context.Context
		expectedUUID  string
		expectedFound bool
	}

	tests := map[string]func(t *testing.T) test{
		"caller_present": func(t *testing.T) test {
			return test{
				ctx:           WithCaller(context.Background(), identity.User{UUID: "user-1", DisplayName: "Ana"}),
				expectedUUID:  "user-1",
				expectedFound: true,
			}
		},
		"no_caller": func(t *testing.T) test {
			return test{ctx: context.Background()}
		},
		"empty_caller_is_not_a_caller": func(t *testing.T) test {
			return test{ctx: WithCaller(context.Background(), identity.User{DisplayName: "Ana"})}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			userUUID, found := UserUUID(tc.ctx)

			require.Equal(t, tc.expectedUUID, userUUID)
			require.Equal(t, tc.expectedFound, found)
		})
	}
}
