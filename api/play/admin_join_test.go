package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// fakeAdmin is a Runtime admin connection whose views and end the test
// controls, recording what the handler asked of it.
type fakeAdmin struct {
	views     chan runtime.PlayerView
	lobby     chan runtime.LobbyUpdate
	selectErr error
	// finalView, when set, is produced by CancelSession right before the
	// Session ends.
	finalView *runtime.PlayerView

	endOnce sync.Once
	ended   chan struct{}
	reason  runtime.EndReason

	mu          sync.Mutex
	selected    []string
	disconnects int
}

func newFakeAdmin(views ...runtime.PlayerView) *fakeAdmin {
	admin := &fakeAdmin{
		views: make(chan runtime.PlayerView, len(views)+1),
		lobby: make(chan runtime.LobbyUpdate, 4),
		ended: make(chan struct{}),
	}
	for _, view := range views {
		admin.views <- view
	}
	return admin
}

func (f *fakeAdmin) end(reason runtime.EndReason) {
	f.endOnce.Do(func() {
		f.reason = reason
		close(f.ended)
	})
}

// endedBy returns a fake admin the Runtime has already ended with reason.
func endedBy(reason runtime.EndReason) *fakeAdmin {
	admin := newFakeAdmin()
	admin.end(reason)
	return admin
}

func (f *fakeAdmin) connection() runtime.AdminConnection {
	return runtime.AdminConnection{
		SelectPerspective: func(_ context.Context, participantID string) (runtime.SelectPerspectiveResult, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.selected = append(f.selected, participantID)
			if f.selectErr != nil {
				return runtime.SelectPerspectiveResult{}, f.selectErr
			}
			return runtime.SelectPerspectiveResult{Outcome: runtime.PerspectiveSelected}, nil
		},
		StartSession: func(context.Context) (runtime.StartSessionResult, error) {
			return runtime.StartSessionResult{Outcome: runtime.StartInvalidPlayerCount}, nil
		},
		CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
			if f.finalView != nil {
				f.views <- *f.finalView
			}
			f.end(runtime.EndSessionEnded)
			return runtime.CancelSessionResult{Outcome: runtime.SessionCancelled}, nil
		},
		NextLobbyUpdate: func(ctx context.Context) (runtime.LobbyUpdate, bool) {
			return nextOf(ctx, f.lobby, f.ended)
		},
		NextView: func(ctx context.Context) (runtime.PlayerView, bool) {
			return nextOf(ctx, f.views, f.ended)
		},
		Ended: func(ctx context.Context) (runtime.ConnectionEnd, bool) {
			select {
			case <-f.ended:
				return runtime.ConnectionEnd{Reason: f.reason}, true
			case <-ctx.Done():
				return runtime.ConnectionEnd{}, false
			}
		},
		Disconnect: func() {
			f.mu.Lock()
			f.disconnects++
			f.mu.Unlock()
			f.end(runtime.EndDisconnected)
		},
	}
}

// nextOf is a Runtime stream: the next item of items, where items produced
// before ended still come first.
func nextOf[T any](ctx context.Context, items chan T, ended chan struct{}) (T, bool) {
	var none T
	select {
	case item := <-items:
		return item, true
	case <-ended:
		select {
		case item := <-items:
			return item, true
		default:
			return none, false
		}
	case <-ctx.Done():
		return none, false
	}
}

func TestHandleAdminJoin(t *testing.T) {
	type dependencies struct {
		admins *MockadminAttacher
	}
	type test struct {
		// unrouted leaves the request without the caller and Session the
		// middlewares put on its context.
		unrouted bool
		// shuttingDown starts the server's shutdown before the request.
		shuttingDown   bool
		protocols      []string
		origin         string
		originPatterns []string
		admin          *fakeAdmin
		clientSends    []string
		// clientCloses ends the connection from the client once it received
		// expectedReceived; otherwise it reads until the server ends it.
		clientCloses bool

		expectedDialStatus int
		expectedReceived   []string
		// expectedReceivedAnyOrder are messages sent concurrently, whose
		// relative order is not defined.
		expectedReceivedAnyOrder []string
		expectedCloseStatus      websocket.StatusCode
		expectedSelected         []string
		expectedDisconnects      int
	}

	const (
		callerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		ownerEpoch  = int64(3)
	)
	accept := func(deps dependencies, admin *fakeAdmin) {
		deps.admins.EXPECT().
			AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
			Return(runtime.AttachAdminResult{Outcome: runtime.AttachAccepted, Connection: admin.connection()}, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"selecting_a_perspective_is_answered": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientSends:         []string{`{"type":"select_perspective","participant_id":"P2"}`},
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"select_perspective_result","outcome":"selected"}`},
				expectedSelected:    []string{"P2"},
				expectedDisconnects: 1,
			}
		},
		"views_are_delivered_with_the_game_state_as_is": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin(runtime.PlayerView{ParticipantID: "P1", Full: true, State: []byte(`{"score":3}`)})
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"player_view","participant_id":"P1","full":true,"state":{"score":3}}`},
				expectedDisconnects: 1,
			}
		},
		"starting_is_answered": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientSends:         []string{`{"type":"start_session"}`},
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"start_session_result","outcome":"invalid_player_count"}`},
				expectedDisconnects: 1,
			}
		},
		"lobby_updates_are_delivered_until_the_lobby_closes": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			admin.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "u1"}, {UserUUID: "u2"}}}
			admin.lobby <- runtime.LobbyUpdate{Closed: true}
			// Never sent: nothing follows lobby_closed.
			admin.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "u3"}}}
			admin.end(runtime.EndSessionEnded)
			accept(deps, admin)
			return test{
				protocols: []string{"playhoot.v1"},
				admin:     admin,
				expectedReceived: []string{
					`{"type":"lobby_update","members":[{"user_uuid":"u1"},{"user_uuid":"u2"}]}`,
					`{"type":"lobby_closed"}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"cancelling_is_answered_then_the_runtime_ends_the_connection": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientSends:         []string{`{"type":"cancel_session"}`},
				expectedReceived:    []string{`{"type":"cancel_session_result","outcome":"cancelled"}`},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"cancelling_delivers_the_answer_and_the_final_view_before_closing": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			admin.finalView = &runtime.PlayerView{ParticipantID: "P1", Full: true, State: []byte(`{"over":true}`)}
			accept(deps, admin)
			return test{
				protocols:   []string{"playhoot.v1"},
				admin:       admin,
				clientSends: []string{`{"type":"cancel_session"}`},
				expectedReceivedAnyOrder: []string{
					`{"type":"cancel_session_result","outcome":"cancelled"}`,
					`{"type":"player_view","participant_id":"P1","full":true,"state":{"over":true}}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"connection_ended_for_an_owner_change_asks_the_client_to_reconnect": func(t *testing.T, deps dependencies) test {
			admin := endedBy(runtime.EndOwnerChanged)
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				expectedCloseStatus: websocket.StatusServiceRestart,
				expectedDisconnects: 1,
			}
		},
		"perspective_without_participant_is_malformed": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientSends:         []string{`{"type":"select_perspective"}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedDisconnects: 1,
			}
		},
		"unknown_message_type_ends_the_connection": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientSends:         []string{`{"type":"sample"}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
				expectedDisconnects: 1,
			}
		},
		"runtime_operation_failing_is_a_server_failure": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			admin.selectErr = errors.New("runtime down")
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				clientSends:         []string{`{"type":"select_perspective","participant_id":"P2"}`},
				expectedCloseStatus: websocket.StatusInternalError,
				expectedSelected:    []string{"P2"},
				expectedDisconnects: 1,
			}
		},
		"rejected_connection_receives_the_reason_and_closes": func(t *testing.T, deps dependencies) test {
			deps.admins.EXPECT().
				AttachAdmin(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachAdminResult{Outcome: runtime.AttachRejected, Rejection: runtime.RejectNotCreator}, nil)
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
		"accepted_without_operations": func(t *testing.T, deps dependencies) test {
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
				Return(runtime.AttachAdminResult{Outcome: "something_new"}, nil)
			return test{
				protocols:           []string{"playhoot.v1"},
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"admitted_connection_is_refused_while_shutting_down": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				shuttingDown:        true,
				protocols:           []string{"playhoot.v1"},
				admin:               admin,
				expectedCloseStatus: websocket.StatusGoingAway,
				expectedDisconnects: 1,
			}
		},
		"allowed_cross_origin": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:           []string{"playhoot.v1"},
				origin:              "https://app.example.com",
				originPatterns:      []string{"app.example.com"},
				admin:               admin,
				clientCloses:        true,
				expectedDisconnects: 1,
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

			connections := live.New()
			if tc.shuttingDown {
				require.NoError(t, connections.Shutdown(context.Background()))
			}
			handler := NewHandler(nil, nil, nil, deps.admins, nil, nil, connections, tc.originPatterns)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				ctx := r.Context()
				if !tc.unrouted {
					ctx = withRoutedSession(auth.WithUserUUID(ctx, callerUUID), sessionUUID, ownerEpoch)
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
			if tc.expectedDialStatus != 0 {
				require.Error(t, err)
				require.Equal(t, tc.expectedDialStatus, response.StatusCode)
			} else {
				require.NoError(t, err)
				require.Equal(t, "playhoot.v1", conn.Subprotocol())
				for _, message := range tc.clientSends {
					require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(message)))
				}
				if tc.clientCloses {
					for range tc.expectedReceived {
						_, data, err := conn.Read(ctx)
						require.NoError(t, err)
						received = append(received, string(data))
					}
					require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
				} else {
					for {
						_, data, err := conn.Read(ctx)
						if err != nil {
							require.Equal(t, tc.expectedCloseStatus, websocket.CloseStatus(err))
							break
						}
						received = append(received, string(data))
					}
				}
			}
			if tc.expectedReceivedAnyOrder != nil {
				require.ElementsMatch(t, normalizeJSON(t, tc.expectedReceivedAnyOrder), normalizeJSON(t, received))
			} else {
				require.Len(t, received, len(tc.expectedReceived))
				for i, expected := range tc.expectedReceived {
					require.JSONEq(t, expected, received[i])
				}
			}

			select {
			case <-handlerDone:
			case <-ctx.Done():
				t.Fatal("handler did not return")
			}
			require.NoError(t, connections.Shutdown(ctx), "the served connection must end")
			if tc.admin != nil {
				tc.admin.mu.Lock()
				defer tc.admin.mu.Unlock()
				require.Equal(t, tc.expectedSelected, tc.admin.selected)
				require.Equal(t, tc.expectedDisconnects, tc.admin.disconnects)
			}
		})
	}
}

// normalizeJSON re-encodes each message so equal JSON compares equal.
func normalizeJSON(t *testing.T, messages []string) []string {
	normalized := make([]string, len(messages))
	for i, message := range messages {
		var value any
		require.NoError(t, json.Unmarshal([]byte(message), &value))
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		normalized[i] = string(encoded)
	}
	return normalized
}
