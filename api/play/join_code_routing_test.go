package play

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestResolveJoinCode(t *testing.T) {
	type dependencies struct {
		resolver *MockjoinCodeResolver
	}
	type test struct {
		joinCode            string
		expectedCode        int
		expectedBody        string
		expectedNextCalled  bool
		expectedSessionUUID string
		// expectedRefusal is the wire reason the join was refused with.
		expectedRefusal string
	}

	const (
		joinCode    = "K7QX2M"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)
	ctx := context.Background()
	resolves := func(deps dependencies, resolution sessionpkg.JoinCodeResolution) {
		deps.resolver.EXPECT().ResolveJoinCode(ctx, joinCode).Return(resolution, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"found_routes_the_session": func(t *testing.T, deps dependencies) test {
			resolves(deps, sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound, SessionUUID: sessionUUID})
			return test{
				joinCode:            joinCode,
				expectedCode:        http.StatusNoContent,
				expectedNextCalled:  true,
				expectedSessionUUID: sessionUUID,
			}
		},
		"invalid_code_is_refused": func(t *testing.T, deps dependencies) test {
			resolves(deps, sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid})
			return test{joinCode: joinCode, expectedCode: http.StatusOK, expectedRefusal: "invalid_join_code"}
		},
		"expired_code_is_refused": func(t *testing.T, deps dependencies) test {
			resolves(deps, sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeExpired})
			return test{joinCode: joinCode, expectedCode: http.StatusOK, expectedRefusal: "join_code_expired"}
		},
		"code_of_a_started_session_is_refused": func(t *testing.T, deps dependencies) test {
			resolves(deps, sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeSessionStarted})
			return test{joinCode: joinCode, expectedCode: http.StatusOK, expectedRefusal: "session_started"}
		},
		"found_without_session_is_a_failure": func(t *testing.T, deps dependencies) test {
			resolves(deps, sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound})
			return test{
				joinCode:     joinCode,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"resolving join code failed"}`,
			}
		},
		"unknown_outcome": func(t *testing.T, deps dependencies) test {
			resolves(deps, sessionpkg.JoinCodeResolution{Outcome: "something_new"})
			return test{
				joinCode:     joinCode,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"resolving join code failed"}`,
			}
		},
		"resolver_fails": func(t *testing.T, deps dependencies) test {
			deps.resolver.EXPECT().ResolveJoinCode(ctx, joinCode).Return(sessionpkg.JoinCodeResolution{}, errors.New("database down"))
			return test{
				joinCode:     joinCode,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"resolving join code failed"}`,
			}
		},
		"too_long_is_refused_unresolved": func(t *testing.T, deps dependencies) test {
			return test{
				joinCode:        strings.Repeat("A", maxJoinCodeLength+1),
				expectedCode:    http.StatusOK,
				expectedRefusal: "invalid_join_code",
			}
		},
		"empty_is_refused_unresolved": func(t *testing.T, deps dependencies) test {
			return test{expectedCode: http.StatusOK, expectedRefusal: "invalid_join_code"}
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
			var refusal string
			refuse := func(_ http.ResponseWriter, _ *http.Request, rejection rejectedMessage) { refusal = rejection.Reason }

			request := httptest.NewRequest(http.MethodGet, "/join/x", nil).WithContext(ctx)
			request.SetPathValue("joinCode", tc.joinCode)
			recorder := httptest.NewRecorder()

			resolveJoinCode(deps.resolver, refuse, "joinCode", "sessionUUID")(next)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedNextCalled, nextCalled)
			require.Equal(t, tc.expectedSessionUUID, routedSessionUUID)
			require.Equal(t, tc.expectedRefusal, refusal)
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			} else {
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}

func TestRefuseUpgraded(t *testing.T) {
	type test struct {
		// callerConnections are the caller's other live connections here.
		callerConnections int
		withoutCaller     bool

		// expectedRefusal is the status refusing the upgrade, if refused.
		expectedRefusal int
	}

	const callerUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"

	tests := map[string]func(t *testing.T) test{
		"client_reads_the_reason_and_the_connection_closes": func(t *testing.T) test {
			return test{}
		},
		"caller_without_room_is_refused_before_the_upgrade": func(t *testing.T) test {
			return test{callerConnections: live.MaxConnectionsPerCaller, expectedRefusal: http.StatusTooManyRequests}
		},
		"route_without_authorization_is_a_failure": func(t *testing.T) test {
			return test{withoutCaller: true, expectedRefusal: http.StatusInternalServerError}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			connections := live.New(0)
			for range tc.callerConnections {
				_, err := connections.Reserve(context.Background(), callerUUID)
				require.NoError(t, err)
			}
			handler := NewHandler(nil, nil, nil, nil, nil, nil, connections, nil)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				ctx := r.Context()
				if !tc.withoutCaller {
					ctx = auth.WithCaller(ctx, identity.User{UUID: callerUUID})
				}
				handler.refuseUpgraded(w, r.WithContext(ctx), rejectedMessage{Reason: "join_code_expired"})
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, server.URL+"/join/K7QX2M", &websocket.DialOptions{
				Subprotocols: []string{"playhoot.v1"},
			})
			if tc.expectedRefusal != 0 {
				require.Error(t, err)
				require.Equal(t, tc.expectedRefusal, response.StatusCode)
				return
			}
			require.NoError(t, err)
			_, data, err := conn.Read(ctx)
			require.NoError(t, err)
			require.JSONEq(t, `{"type":"rejected","reason":"join_code_expired"}`, string(data))
			_, _, err = conn.Read(ctx)
			var closed websocket.CloseError
			require.ErrorAs(t, err, &closed)
			require.Equal(t, websocket.StatusPolicyViolation, closed.Code)
			require.Equal(t, "admission rejected", closed.Reason)

			select {
			case <-handlerDone:
			case <-ctx.Done():
				t.Fatal("handler did not return")
			}
			for range live.MaxConnectionsPerCaller {
				_, err := connections.Reserve(context.Background(), callerUUID)
				require.NoError(t, err, "the refused connection holds no room")
			}
		})
	}
}
