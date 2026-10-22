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
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// fakeAdmin is a Runtime admin connection whose views and end the test
// controls, recording what the handler asked of it.
type fakeAdmin struct {
	*fakeLobby[runtime.SessionStart]
	views     chan runtime.WatchedView
	selectErr error
	// finalView, when set, is produced by CancelSession right before the
	// Session ends, and resyncView by Resync.
	finalView  *runtime.WatchedView
	resyncView *runtime.WatchedView

	endOnce sync.Once
	ended   chan struct{}
	ending  runtime.ConnectionEnd

	mu          sync.Mutex
	selected    []string
	resyncs     int
	disconnects int
}

func newFakeAdmin(views ...runtime.WatchedView) *fakeAdmin {
	admin := &fakeAdmin{
		fakeLobby: newFakeLobby[runtime.SessionStart](),
		views:     make(chan runtime.WatchedView, len(views)+2),
		ended:     make(chan struct{}),
	}
	for _, view := range views {
		admin.views <- view
	}
	return admin
}

// startedFakeAdmin returns a fake admin of a Session that started with roster.
func startedFakeAdmin(views ...runtime.WatchedView) *fakeAdmin {
	admin := newFakeAdmin(views...)
	admin.start(roster)
	return admin
}

func (f *fakeAdmin) end(reason runtime.EndReason) {
	f.endWith(runtime.ConnectionEnd{Reason: reason})
}

// endWith ends the connection as the Runtime would when the Session ended.
func (f *fakeAdmin) endWith(ending runtime.ConnectionEnd) {
	f.endOnce.Do(func() {
		f.ending = ending
		close(f.ended)
		f.closeLobby()
	})
}

// hostCancelled is how a fake's connection ends when its Session is cancelled.
var hostCancelled = runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: "CANCELLED", EndCause: "HOST_CANCELLED"}

// roster is the Session start the fakes report.
var roster = runtime.SessionStart{
	Participants: []runtime.Participant{
		{ParticipantID: "P1", UserUUID: "u1", DisplayName: "Ana"},
		{ParticipantID: "P2", UserUUID: "u2", DisplayName: "Bruno"},
	},
	Frontend: runtime.GameFrontend{
		URL:       "https://storage.example/frontend?sig=abc",
		SHA256:    "9f86d081",
		ExpiresAt: time.Date(2026, 10, 17, 21, 0, 0, 0, time.UTC),
		Viewport:  runtime.GameViewport{AspectRatio: "9:16", Width: 1080, Height: 1920},
	},
}

// sessionUUIDJSON is the routed Session of these tests, as lobby updates and
// session_started carry it.
const sessionUUIDJSON = `"session_uuid":"7c9e6679-7425-40de-944b-e07fc1f90ae7"`

// rosterJSON is roster's fields on the wire, inside a session_started, with
// the Session it started.
const rosterJSON = sessionUUIDJSON + `,"participants":[{"participant_id":"P1","user_uuid":"u1","display_name":"Ana"},{"participant_id":"P2","user_uuid":"u2","display_name":"Bruno"}],` +
	`"frontend":{"url":"https://storage.example/frontend?sig=abc","sha256":"9f86d081","expires_at":"2026-10-17T21:00:00Z",` +
	`"viewport":{"aspect_ratio":"9:16","width":1080,"height":1920}}`

// fakeLobby is the lobby side of a fake Runtime connection: its lobby stream
// ends once the Session starts or the connection ends.
type fakeLobby[S any] struct {
	lobby    chan runtime.LobbyUpdate
	started  chan S
	overOnce sync.Once
	over     chan struct{}
}

func newFakeLobby[S any]() *fakeLobby[S] {
	return &fakeLobby[S]{
		lobby:   make(chan runtime.LobbyUpdate, 4),
		started: make(chan S, 1),
		over:    make(chan struct{}),
	}
}

// start starts the Session, which ends the lobby.
func (l *fakeLobby[S]) start(start S) {
	l.started <- start
	l.closeLobby()
}

func (l *fakeLobby[S]) closeLobby() {
	l.overOnce.Do(func() { close(l.over) })
}

func (l *fakeLobby[S]) nextLobbyUpdate(ctx context.Context) (runtime.LobbyUpdate, bool) {
	return nextOf(ctx, l.lobby, l.over)
}

// waitStarted is the Runtime's Started: false once ended without a start.
func (l *fakeLobby[S]) waitStarted(ctx context.Context, ended chan struct{}) (S, bool) {
	var none S
	select {
	case start := <-l.started:
		return start, true
	case <-ended:
		select {
		case start := <-l.started:
			return start, true
		default:
			return none, false
		}
	case <-ctx.Done():
		return none, false
	}
}

// endedBy returns a fake admin the Runtime has already ended with reason.
func endedBy(reason runtime.EndReason) *fakeAdmin {
	admin := newFakeAdmin()
	admin.end(reason)
	return admin
}

func (f *fakeAdmin) connection() runtime.AdminConnection {
	return runtime.AdminConnection{
		LimitHit: func(context.Context, string) {},
		SelectPerspective: func(_ context.Context, participantID string) (runtime.SelectPerspectiveResult, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.selected = append(f.selected, participantID)
			if f.selectErr != nil {
				return runtime.SelectPerspectiveResult{}, f.selectErr
			}
			return runtime.SelectPerspectiveResult{Outcome: runtime.PerspectiveSelected}, nil
		},
		Resync: func(context.Context) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.resyncs++
			if f.resyncView != nil {
				f.views <- *f.resyncView
			}
			return nil
		},
		StartSession: func(context.Context) (runtime.StartSessionResult, error) {
			return runtime.StartSessionResult{Outcome: runtime.StartInvalidPlayerCount}, nil
		},
		CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
			if f.finalView != nil {
				f.views <- *f.finalView
			}
			f.endWith(hostCancelled)
			return runtime.CancelSessionResult{Outcome: runtime.SessionCancelled}, nil
		},
		NextLobbyUpdate: f.nextLobbyUpdate,
		Started: func(ctx context.Context) (runtime.SessionStart, bool) {
			return f.waitStarted(ctx, f.ended)
		},
		NextView: func(ctx context.Context) (runtime.WatchedView, bool) {
			return nextOf(ctx, f.views, f.ended)
		},
		Ended: func(ctx context.Context) (runtime.ConnectionEnd, bool) {
			select {
			case <-f.ended:
				return f.ending, true
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
		expectedResyncs          int
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
			admin := startedFakeAdmin(runtime.WatchedView{
				PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 4, State: []byte(`{"score":3}`)},
				Events:     [][]byte{[]byte(`{"kind":"goal"}`)},
			})
			accept(deps, admin)
			return test{
				protocols:    []string{"playhoot.v1"},
				admin:        admin,
				clientCloses: true,
				expectedReceived: []string{
					`{"type":"session_started",` + rosterJSON + `}`,
					`{"type":"player_view","participant_id":"P1","full":true,"revision":4,"state":{"score":3},"events":[{"kind":"goal"}]}`,
				},
				expectedDisconnects: 1,
			}
		},
		"resync_delivers_the_full_view_it_asked_for": func(t *testing.T, deps dependencies) test {
			admin := startedFakeAdmin()
			admin.resyncView = &runtime.WatchedView{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, Revision: 7, State: []byte(`{"lap":2}`)}}
			accept(deps, admin)
			return test{
				protocols:    []string{"playhoot.v1"},
				admin:        admin,
				clientSends:  []string{`{"type":"resync"}`},
				clientCloses: true,
				expectedReceived: []string{
					`{"type":"session_started",` + rosterJSON + `}`,
					`{"type":"player_view","participant_id":"P2","full":true,"revision":7,"state":{"lap":2}}`,
				},
				expectedResyncs:     1,
				expectedDisconnects: 1,
			}
		},
		"start_follows_the_lobby_and_precedes_the_views": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin(runtime.WatchedView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 1, State: []byte(`{"round":1}`)}})
			admin.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "u1", DisplayName: "Ana"}, {UserUUID: "u2", DisplayName: "Bruno"}}}
			admin.start(roster)
			accept(deps, admin)
			return test{
				protocols:    []string{"playhoot.v1"},
				admin:        admin,
				clientCloses: true,
				expectedReceived: []string{
					`{"type":"lobby_update",` + sessionUUIDJSON + `,"members":[{"user_uuid":"u1","display_name":"Ana"},{"user_uuid":"u2","display_name":"Bruno"}]}`,
					`{"type":"session_started",` + rosterJSON + `}`,
					`{"type":"player_view","participant_id":"P1","full":true,"revision":1,"state":{"round":1}}`,
				},
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
			admin.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "u1", DisplayName: "Ana"}, {UserUUID: "u2", DisplayName: "Bruno"}}}
			admin.endWith(runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: "EXPIRED", EndCause: "LOBBY_EXPIRED"})
			accept(deps, admin)
			return test{
				protocols: []string{"playhoot.v1"},
				admin:     admin,
				expectedReceived: []string{
					`{"type":"lobby_update",` + sessionUUIDJSON + `,"members":[{"user_uuid":"u1","display_name":"Ana"},{"user_uuid":"u2","display_name":"Bruno"}]}`,
					`{"type":"lobby_closed","status":"EXPIRED","end_cause":"LOBBY_EXPIRED"}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"cancelling_is_answered_then_the_runtime_ends_the_connection": func(t *testing.T, deps dependencies) test {
			admin := newFakeAdmin()
			accept(deps, admin)
			return test{
				protocols:   []string{"playhoot.v1"},
				admin:       admin,
				clientSends: []string{`{"type":"cancel_session"}`},
				expectedReceived: []string{
					`{"type":"cancel_session_result","outcome":"cancelled"}`,
					`{"type":"lobby_closed","status":"CANCELLED","end_cause":"HOST_CANCELLED"}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"cancelling_delivers_the_answer_and_the_final_view_before_closing": func(t *testing.T, deps dependencies) test {
			admin := startedFakeAdmin()
			admin.finalView = &runtime.WatchedView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 9, State: []byte(`{"over":true}`)}}
			accept(deps, admin)
			return test{
				protocols:   []string{"playhoot.v1"},
				admin:       admin,
				clientSends: []string{`{"type":"cancel_session"}`},
				expectedReceivedAnyOrder: []string{
					`{"type":"session_started",` + rosterJSON + `}`,
					`{"type":"cancel_session_result","outcome":"cancelled"}`,
					`{"type":"player_view","participant_id":"P1","full":true,"revision":9,"state":{"over":true}}`,
					`{"type":"session_ended","status":"CANCELLED","end_cause":"HOST_CANCELLED"}`,
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

			connections := live.New(0)
			if tc.shuttingDown {
				require.NoError(t, connections.Shutdown(context.Background()))
			}
			handler := NewHandler(nil, nil, nil, deps.admins, nil, nil, connections, tc.originPatterns)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				ctx := r.Context()
				if !tc.unrouted {
					ctx = withRoutedSession(auth.WithCaller(ctx, identity.User{UUID: callerUUID}), sessionUUID, ownerEpoch)
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
				require.Equal(t, tc.expectedResyncs, tc.admin.resyncs)
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
