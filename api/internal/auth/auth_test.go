package auth

import (
	"context"
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
		expectedCaller     identity.UserUUID
	}

	const (
		operation  = identity.OperationCreateSession
		callerUUID = identity.UserUUID("0f8fad5b-d9cb-469f-a165-70867728950e")
	)

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"allowed": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, identity.AccessToken("jwt-token"), operation).
				Return(callerUUID, identity.AccessAllowed, nil)
			return test{
				ctx:                ctx,
				authorization:      "Bearer jwt-token",
				expectedCode:       http.StatusNoContent,
				expectedNextCalled: true,
				expectedCaller:     callerUUID,
			}
		},
		"allowed_with_lowercase_scheme": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, identity.AccessToken("jwt-token"), operation).
				Return(callerUUID, identity.AccessAllowed, nil)
			return test{
				ctx:                ctx,
				authorization:      "bearer jwt-token",
				expectedCode:       http.StatusNoContent,
				expectedNextCalled: true,
				expectedCaller:     callerUUID,
			}
		},
		"missing_authorization_header": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"message":"missing bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"authorization_scheme_is_not_bearer": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				authorization:      "Basic dXNlcjpwYXNz",
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"message":"missing bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"bearer_without_token": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:                context.Background(),
				authorization:      "Bearer ",
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"message":"missing bearer token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"identity_says_unauthenticated": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, identity.AccessToken("expired"), operation).
				Return(identity.UserUUID(""), identity.AccessUnauthenticated, nil)
			return test{
				ctx:                ctx,
				authorization:      "Bearer expired",
				expectedCode:       http.StatusUnauthorized,
				expectedBody:       `{"message":"invalid or expired token"}`,
				expectedAuthHeader: "Bearer",
			}
		},
		"identity_says_forbidden": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, identity.AccessToken("guest-token"), operation).
				Return(callerUUID, identity.AccessForbidden, nil)
			return test{
				ctx:           ctx,
				authorization: "Bearer guest-token",
				expectedCode:  http.StatusForbidden,
				expectedBody:  `{"message":"operation not allowed"}`,
			}
		},
		"identity_fails": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.authorizer.EXPECT().
				Authorize(ctx, identity.AccessToken("jwt-token"), operation).
				Return(identity.UserUUID(""), identity.AccessDecision(""), errors.New("identity down"))
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
				Authorize(ctx, identity.AccessToken("jwt-token"), operation).
				Return(identity.UserUUID(""), identity.AccessAllowed, nil)
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
				Authorize(ctx, identity.AccessToken("jwt-token"), operation).
				Return(callerUUID, identity.AccessDecision("something_new"), nil)
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
			var caller identity.UserUUID
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				caller, _ = UserUUID(r.Context())
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

func TestBearerToken(t *testing.T) {
	type test struct {
		authorization string
		expectedToken identity.AccessToken
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
		expectedUUID  identity.UserUUID
		expectedFound bool
	}

	tests := map[string]func(t *testing.T) test{
		"caller_present": func(t *testing.T) test {
			return test{
				ctx:           WithUserUUID(context.Background(), "user-1"),
				expectedUUID:  "user-1",
				expectedFound: true,
			}
		},
		"no_caller": func(t *testing.T) test {
			return test{ctx: context.Background()}
		},
		"empty_caller_is_not_a_caller": func(t *testing.T) test {
			return test{ctx: WithUserUUID(context.Background(), "")}
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
