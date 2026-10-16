package ownerrouting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestRequire(t *testing.T) {
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
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

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
		"session_unavailable": func(t *testing.T, deps dependencies) test {
			ctx := context.Background()
			deps.locator.EXPECT().
				LocateOrClaimOwner(ctx, sessionUUID).
				Return(runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable}, nil)
			return test{
				ctx:          ctx,
				sessionUUID:  sessionUUID,
				expectedCode: http.StatusNotFound,
				expectedBody: `{"message":"session not found or not joinable"}`,
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
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{
				ctx:          context.Background(),
				sessionUUID:  "not-a-uuid",
				expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"session UUID must be a valid UUID"}`,
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
				routedSessionUUID, routedEpoch, _ = OwnedSession(r.Context())
				w.WriteHeader(http.StatusNoContent)
			}

			request := httptest.NewRequest(http.MethodGet, "/sessions/x/admin", nil).WithContext(tc.ctx)
			request.SetPathValue("sessionUUID", tc.sessionUUID)
			recorder := httptest.NewRecorder()

			Require(deps.locator, "sessionUUID")(next)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedNextCalled, nextCalled)
			require.Equal(t, tc.expectedSessionUUID, routedSessionUUID)
			require.Equal(t, tc.expectedEpoch, routedEpoch)
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			} else {
				require.Empty(t, recorder.Body.String())
			}
		})
	}
}

func TestOwnedSession(t *testing.T) {
	type test struct {
		ctx                 context.Context
		expectedSessionUUID string
		expectedEpoch       int64
		expectedFound       bool
	}

	tests := map[string]func(t *testing.T) test{
		"session_present": func(t *testing.T) test {
			return test{
				ctx:                 WithOwnedSession(context.Background(), "session-1", 3),
				expectedSessionUUID: "session-1",
				expectedEpoch:       3,
				expectedFound:       true,
			}
		},
		"no_session": func(t *testing.T) test {
			return test{ctx: context.Background()}
		},
		"empty_session_is_not_a_session": func(t *testing.T) test {
			return test{ctx: WithOwnedSession(context.Background(), "", 3), expectedEpoch: 3}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			sessionUUID, epoch, found := OwnedSession(tc.ctx)

			require.Equal(t, tc.expectedSessionUUID, sessionUUID)
			require.Equal(t, tc.expectedEpoch, epoch)
			require.Equal(t, tc.expectedFound, found)
		})
	}
}
