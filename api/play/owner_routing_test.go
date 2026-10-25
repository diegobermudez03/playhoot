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
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestRequireOwner(t *testing.T) {
	type dependencies struct {
		locator *MockownerLocator
	}
	type test struct {
		ctx                 context.Context
		sessionUUID         string
		expectedCode        int
		expectedBody        string
		expectedNextCalled  bool
		expectedSessionUUID string
		expectedEpoch       int64
		// expectedRefusal is what the join was refused with, if refused.
		expectedRefusal *rejectedMessage
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ended := func(deps dependencies, status string, endCause string) test {
		ctx := context.Background()
		deps.locator.EXPECT().
			LocateOrClaimOwner(ctx, sessionUUID).
			Return(runtime.OwnerPlacement{Outcome: runtime.OwnerSessionEnded, Ended: runtime.EndedSession{Status: status, EndCause: endCause}}, nil)
		return test{
			ctx:             ctx,
			sessionUUID:     sessionUUID,
			expectedCode:    http.StatusOK,
			expectedRefusal: &rejectedMessage{Reason: "session_ended", Status: status, EndCause: endCause},
		}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"owned_here": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: runtime.OwnerHere, Address: "10.0.0.1:8080", Epoch: 4}, nil)
			return test{
				ctx:                 ctx,
				sessionUUID:         sessionUUID,
				expectedCode:        http.StatusNoContent,
				expectedNextCalled:  true,
				expectedSessionUUID: sessionUUID,
				expectedEpoch:       4,
			}
		},
		"session_uuid_is_normalized": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: runtime.OwnerHere, Epoch: 1}, nil)
			return test{
				ctx:                 ctx,
				sessionUUID:         strings.ToUpper(sessionUUID),
				expectedCode:        http.StatusNoContent,
				expectedNextCalled:  true,
				expectedSessionUUID: sessionUUID,
				expectedEpoch:       1,
			}
		},
		"unknown_session_is_refused_as_invalid": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: runtime.OwnerSessionNotFound}, nil)
			return test{
				ctx:             ctx,
				sessionUUID:     sessionUUID,
				expectedCode:    http.StatusOK,
				expectedRefusal: &rejectedMessage{Reason: "invalid_session"},
			}
		},
		"finished_session_is_refused_with_how_it_ended": func(t *testing.T, deps dependencies) test {
			return ended(deps, "FINISHED", "GAME_FINISHED")
		},
		"cancelled_session_is_refused_with_how_it_ended": func(t *testing.T, deps dependencies) test {
			return ended(deps, "CANCELLED", "HOST_CANCELLED")
		},
		"expired_session_is_refused_with_how_it_ended": func(t *testing.T, deps dependencies) test {
			return ended(deps, "EXPIRED", "LOBBY_EXPIRED")
		},
		"aborted_session_is_refused_with_how_it_ended": func(t *testing.T, deps dependencies) test {
			return ended(deps, "ABORTED", "OWNER_LOST")
		},
		"unavailable_session_is_refused_as_unavailable": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable}, nil)
			return test{
				ctx:             ctx,
				sessionUUID:     sessionUUID,
				expectedCode:    http.StatusOK,
				expectedRefusal: &rejectedMessage{Reason: "session_unavailable"},
			}
		},
		"owned_elsewhere_is_not_forwarded_yet": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: runtime.OwnerElsewhere, Address: "10.0.0.2:8080", Epoch: 2}, nil)
			return test{
				ctx:          ctx,
				sessionUUID:  sessionUUID,
				expectedCode: http.StatusServiceUnavailable,
				expectedBody: `{"message":"session temporarily unavailable"}`,
			}
		},
		"unknown_outcome": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: "something_new"}, nil)
			return test{
				ctx:          ctx,
				sessionUUID:  sessionUUID,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"routing to session failed"}`,
			}
		},
		"locator_fails": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{}, errors.New("registry down"))
			return test{
				ctx:          ctx,
				sessionUUID:  sessionUUID,
				expectedCode: http.StatusInternalServerError,
				expectedBody: `{"message":"routing to session failed"}`,
			}
		},
		"session_uuid_that_is_not_a_uuid_is_refused_unrouted": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:             context.Background(),
				sessionUUID:     "not-a-uuid",
				expectedCode:    http.StatusOK,
				expectedRefusal: &rejectedMessage{Reason: "invalid_session"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{locator: NewMockownerLocator(ctrl)}
			tc := setup(t, deps)

			nextCalled := false
			var routedSessionUUID string
			var routedEpoch int64
			next := func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				routedSessionUUID, routedEpoch, _ = routedSessionFrom(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}
			var refusal *rejectedMessage
			refuse := func(_ http.ResponseWriter, _ *http.Request, rejection rejectedMessage) { refusal = &rejection }

			request := httptest.NewRequest(http.MethodGet, "/sessions/x/admin", nil).WithContext(tc.ctx)
			request.SetPathValue("sessionUUID", tc.sessionUUID)
			recorder := httptest.NewRecorder()

			requireOwner(deps.locator, refuse, "sessionUUID")(next)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedNextCalled, nextCalled)
			require.Equal(t, tc.expectedSessionUUID, routedSessionUUID)
			require.Equal(t, tc.expectedEpoch, routedEpoch)
			require.Equal(t, tc.expectedRefusal, refusal)
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			} else {
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}

// TestSessionRoutesRefuseReadably joins, as a client, the admin and player
// endpoints of a Session that cannot be joined, through their middlewares.
func TestSessionRoutesRefuseReadably(t *testing.T) {
	type test struct {
		sessionUUID string
		placement   *runtime.OwnerPlacement

		expectedRejection string
	}

	const (
		callerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)

	tests := map[string]func(t *testing.T) test{
		"unknown_session": func(t *testing.T) test {
			return test{
				sessionUUID:       sessionUUID,
				placement:         &runtime.OwnerPlacement{Outcome: runtime.OwnerSessionNotFound},
				expectedRejection: `{"type":"rejected","reason":"invalid_session"}`,
			}
		},
		"malformed_session_uuid": func(t *testing.T) test {
			return test{sessionUUID: "not-a-uuid", expectedRejection: `{"type":"rejected","reason":"invalid_session"}`}
		},
		"cancelled_session": func(t *testing.T) test {
			return test{
				sessionUUID: sessionUUID,
				placement: &runtime.OwnerPlacement{
					Outcome: runtime.OwnerSessionEnded,
					Ended:   runtime.EndedSession{Status: "CANCELLED", EndCause: "HOST_CANCELLED"},
				},
				expectedRejection: `{"type":"rejected","reason":"session_ended","status":"CANCELLED","end_cause":"HOST_CANCELLED"}`,
			}
		},
		"finished_session": func(t *testing.T) test {
			return test{
				sessionUUID: sessionUUID,
				placement: &runtime.OwnerPlacement{
					Outcome: runtime.OwnerSessionEnded,
					Ended:   runtime.EndedSession{Status: "FINISHED", EndCause: "GAME_FINISHED"},
				},
				expectedRejection: `{"type":"rejected","reason":"session_ended","status":"FINISHED","end_cause":"GAME_FINISHED"}`,
			}
		},
		"overdue_lobby": func(t *testing.T) test {
			return test{
				sessionUUID:       sessionUUID,
				placement:         &runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable},
				expectedRejection: `{"type":"rejected","reason":"session_unavailable"}`,
			}
		},
	}

	for _, endpoint := range []string{"admin", "player"} {
		for name, setup := range tests {
			t.Run(endpoint+"_"+name, func(t *testing.T) {
				tc := setup(t)
				locator := NewMockownerLocator(gomock.NewController(t))
				if tc.placement != nil {
					locator.EXPECT().LocateOrClaimOwner(gomock.Any(), tc.sessionUUID).Return(*tc.placement, nil)
				}
				handler := NewHandler(nil, nil, locator, nil, nil, nil, live.New(0), nil)
				serve := routeHandler(t, handler.WebSocketRoutes(), "GET /v1/play/join/sessions/{sessionUUID}/"+endpoint)
				mux := http.NewServeMux()
				mux.HandleFunc("GET /v1/play/join/sessions/{sessionUUID}/"+endpoint, func(w http.ResponseWriter, r *http.Request) {
					serve(w, r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: callerUUID})))
				})
				server := httptest.NewServer(mux)
				t.Cleanup(server.Close)

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				conn, _, err := websocket.Dial(ctx, server.URL+"/v1/play/join/sessions/"+tc.sessionUUID+"/"+endpoint, &websocket.DialOptions{
					Subprotocols: []string{"playhoot.v1"},
				})
				require.NoError(t, err, "the refusal is read on an upgraded connection")
				_, data, err := conn.Read(ctx)
				require.NoError(t, err)
				require.JSONEq(t, tc.expectedRejection, string(data))
				_, _, err = conn.Read(ctx)
				var closed websocket.CloseError
				require.ErrorAs(t, err, &closed)
				require.Equal(t, websocket.StatusPolicyViolation, closed.Code)
				require.Equal(t, "admission rejected", closed.Reason)
			})
		}
	}
}

// routeHandler is the route of routes with pattern, wrapped in its
// middlewares, without its authorization.
func routeHandler(t *testing.T, routes []routing.Route, pattern string) http.HandlerFunc {
	t.Helper()
	for _, route := range routes {
		if route.Pattern != pattern {
			continue
		}
		serve := route.Handler
		for i := len(route.Middlewares) - 1; i >= 0; i-- {
			serve = route.Middlewares[i](serve)
		}
		return serve
	}
	t.Fatalf("no route %s", pattern)
	return nil
}

func TestRoutedSessionFrom(t *testing.T) {
	type test struct {
		ctx                 context.Context
		expectedSessionUUID string
		expectedEpoch       int64
		expectedFound       bool
	}

	tests := map[string]func(t *testing.T) test{
		"session_present": func(t *testing.T) test {
			return test{
				ctx:                 withRoutedSession(context.Background(), "session-1", 3),
				expectedSessionUUID: "session-1",
				expectedEpoch:       3,
				expectedFound:       true,
			}
		},
		"no_session": func(t *testing.T) test {
			return test{ctx: context.Background()}
		},
		"empty_session_is_not_a_session": func(t *testing.T) test {
			return test{ctx: withRoutedSession(context.Background(), "", 3), expectedEpoch: 3}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			sessionUUID, epoch, found := routedSessionFrom(tc.ctx)

			require.Equal(t, tc.expectedSessionUUID, sessionUUID)
			require.Equal(t, tc.expectedEpoch, epoch)
			require.Equal(t, tc.expectedFound, found)
		})
	}
}
