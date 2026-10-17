package play

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestResolveJoinCode(t *testing.T) {
	type dependencies struct {
		resolver *MockjoinCodeResolver
	}
	type test struct {
		ctx                 context.Context
		joinCode            string
		expectedCode        int
		expectedBody        string
		expectedNextCalled  bool
		expectedSessionUUID string
	}

	const (
		joinCode    = "K7QX2M"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"found_routes_the_session": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.resolver.EXPECT().
				ResolveJoinCode(ctx, joinCode).
				Return(sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound, SessionUUID: sessionUUID}, nil)
			return test{
				ctx:                 ctx,
				joinCode:            joinCode,
				expectedCode:        http.StatusNoContent,
				expectedNextCalled:  true,
				expectedSessionUUID: sessionUUID,
			}
		},
		"not_found": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.resolver.EXPECT().
				ResolveJoinCode(ctx, joinCode).
				Return(sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeNotFound}, nil)
			return test{
				ctx:          ctx,
				joinCode:     joinCode,
				expectedCode: http.StatusNotFound,
				expectedBody: `{"message":"join code not found or expired"}`,
			}
		},
		"found_without_session_is_a_failure": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.resolver.EXPECT().
				ResolveJoinCode(ctx, joinCode).
				Return(sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound}, nil)
			return test{
				ctx:          ctx,
				joinCode:     joinCode,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"resolving join code failed"}`,
			}
		},
		"unknown_outcome": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.resolver.EXPECT().
				ResolveJoinCode(ctx, joinCode).
				Return(sessionpkg.JoinCodeResolution{Outcome: "something_new"}, nil)
			return test{
				ctx:          ctx,
				joinCode:     joinCode,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"resolving join code failed"}`,
			}
		},
		"resolver_fails": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.resolver.EXPECT().
				ResolveJoinCode(ctx, joinCode).
				Return(sessionpkg.JoinCodeResolution{}, errors.New("database down"))
			return test{
				ctx:          ctx,
				joinCode:     joinCode,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"resolving join code failed"}`,
			}
		},
		"too_long_is_not_resolved": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          context.Background(),
				joinCode:     strings.Repeat("A", maxJoinCodeLength+1),
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"join code is invalid"}`,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{resolver: NewMockjoinCodeResolver(ctrl)}
			tc := setup(t, deps)

			nextCalled := false
			var routedSessionUUID string
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				routedSessionUUID = r.PathValue("sessionUUID")
				w.WriteHeader(http.StatusNoContent)
			}

			request := httptest.NewRequest(http.MethodGet, "/join/x", nil).WithContext(tc.ctx)
			request.SetPathValue("joinCode", tc.joinCode)
			recorder := httptest.NewRecorder()

			resolveJoinCode(deps.resolver, "joinCode", "sessionUUID")(next)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedNextCalled, nextCalled)
			require.Equal(t, tc.expectedSessionUUID, routedSessionUUID)
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			} else {
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}
