package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/ownerrouting"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestHandleAdminJoin(t *testing.T) {
	type dependencies struct {
		admins *MockadminAttacher
	}
	type test struct {
		// unrouted leaves the request without the caller and Session the
		// middlewares put on its context.
		unrouted       bool
		protocols      []string
		origin         string
		originPatterns []string
		// clientCloses ends the connection from the client right away;
		// otherwise the client reads until the server ends it.
		clientCloses bool

		expectedDialStatus  int
		expectedReceived    []string
		expectedCloseStatus websocket.StatusCode
	}

	const (
		callerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		ownerEpoch  = int64(3)
	)
	// connection returns a Runtime connection whose Outbound already holds
	// messages, closed after them.
	connection := func(messages ...runtime.ServerMessage) runtime.Connection {
		outbound := make(chan runtime.ServerMessage, len(messages))
		for _, message := range messages {
			outbound <- message
		}
		close(outbound)
		return runtime.Connection{Inbound: make(chan runtime.ClientMessage, 1), Outbound: outbound}
	}
	// openConnection returns a Runtime connection that has nothing to send
	// yet.
	openConnection := func() runtime.Connection {
		return runtime.Connection{
			Inbound:  make(chan runtime.ClientMessage, 1),
			Outbound: make(chan runtime.ServerMessage),
		}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"accepted_connection_is_served": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{Outcome: runtime.AttachAccepted, Connection: openConnection()}, nil)
			return test{
				protocols:    []string{"playhoot.v1"},
				clientCloses: true,
			}
		},
		"rejected_connection_delivers_its_rejection_and_closes": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{
					Outcome: runtime.AttachRejected,
					Connection: connection(
						runtime.Rejected{Reason: runtime.RejectNotCreator},
						runtime.Close{Reason: runtime.CloseAdmissionRejected},
					),
				}, nil)
			return test{
				protocols:           []string{"playhoot.v1"},
				expectedReceived:    []string{`{"type":"rejected","reason":"not_creator"}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"attach_fails": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{}, errors.New("runtime down"))
			return test{
				protocols:           []string{"playhoot.v1"},
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"connection_without_channels": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{Outcome: runtime.AttachAccepted}, nil)
			return test{
				protocols:           []string{"playhoot.v1"},
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"unknown_outcome": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{Outcome: "something_new", Connection: openConnection()}, nil)
			return test{
				protocols:           []string{"playhoot.v1"},
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"allowed_cross_origin": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{Outcome: runtime.AttachAccepted, Connection: openConnection()}, nil)
			return test{
				protocols:      []string{"playhoot.v1"},
				origin:         "https://app.example.com",
				originPatterns: []string{"app.example.com"},
				clientCloses:   true,
			}
		},
		"disallowed_cross_origin_is_not_upgraded": func(t *testing.T, deps dependencies) test {
			return test{
				protocols:          []string{"playhoot.v1"},
				origin:             "https://evil.example.com",
				originPatterns:     []string{"app.example.com"},
				expectedDialStatus: http.StatusForbidden,
			}
		},
		"subprotocol_not_offered_is_not_upgraded": func(t *testing.T, deps dependencies) test {
			return test{
				protocols:          []string{"other.v1"},
				expectedDialStatus: http.StatusBadRequest,
			}
		},
		"no_caller_or_session_in_context_is_not_upgraded": func(t *testing.T, deps dependencies) test {
			return test{
				unrouted:           true,
				protocols:          []string{"playhoot.v1"},
				expectedDialStatus: http.StatusInternalServerError,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{admins: NewMockadminAttacher(ctrl)}
			tc := setup(t, deps)

			handler := NewHandler(nil, nil, nil, deps.admins, tc.originPatterns)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				ctx := r.Context()
				if !tc.unrouted {
					ctx = ownerrouting.WithOwnedSession(auth.WithUserUUID(ctx, callerUUID), sessionUUID, ownerEpoch)
				}
				handler.handleAdminJoin(w, r.WithContext(ctx))
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			header := http.Header{}
			if tc.origin != "" {
				header.Set("Origin", tc.origin)
			}
			conn, response, err := websocket.Dial(ctx, server.URL+"/sessions/"+sessionUUID+"/admin", &websocket.DialOptions{
				Subprotocols: tc.protocols,
				HTTPHeader:   header,
			})

			var received []string
			switch {
			case tc.expectedDialStatus != 0:
				require.Error(t, err)
				require.Equal(t, tc.expectedDialStatus, response.StatusCode)
			case tc.clientCloses:
				require.NoError(t, err)
				require.Equal(t, "playhoot.v1", conn.Subprotocol())
				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
			default:
				require.NoError(t, err)
				for {
					_, data, err := conn.Read(ctx)
					if err != nil {
						require.Equal(t, tc.expectedCloseStatus, websocket.CloseStatus(err))
						break
					}
					received = append(received, string(data))
				}
			}
			require.Equal(t, tc.expectedReceived, received)

			select {
			case <-handlerDone:
			case <-ctx.Done():
				t.Fatal("handler did not return")
			}
		})
	}
}
