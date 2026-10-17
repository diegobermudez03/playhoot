package play

import (
	"context"
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

// fakePlayer is a Runtime player connection whose streams and end the test
// controls, recording what the handler asked of it.
type fakePlayer struct {
	views      chan runtime.OwnView
	lobby      chan runtime.LobbyUpdate
	commandErr error
	// commandAwaitsSamples makes Command wait until this many SAMPLEs arrived.
	commandAwaitsSamples int
	sampled              chan struct{}
	// resyncView is produced by Resync.
	resyncView runtime.OwnView

	endOnce sync.Once
	ended   chan struct{}
	reason  runtime.EndReason

	mu          sync.Mutex
	samples     []runtime.Sample
	commands    []runtime.Command
	disconnects int
}

func newFakePlayer() *fakePlayer {
	return &fakePlayer{
		views:   make(chan runtime.OwnView, 4),
		lobby:   make(chan runtime.LobbyUpdate, 4),
		sampled: make(chan struct{}, 8),
		ended:   make(chan struct{}),
	}
}

func (f *fakePlayer) end(reason runtime.EndReason) {
	f.endOnce.Do(func() {
		f.reason = reason
		close(f.ended)
	})
}

func (f *fakePlayer) connection() runtime.PlayerConnection {
	return runtime.PlayerConnection{
		Leave: func(context.Context) (runtime.LeaveResult, error) {
			f.end(runtime.EndLeft)
			return runtime.LeaveResult{Outcome: runtime.PlayerLeft}, nil
		},
		Sample: func(_ context.Context, sample runtime.Sample) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.samples = append(f.samples, sample)
			f.sampled <- struct{}{}
			return nil
		},
		Command: func(ctx context.Context, command runtime.Command) (runtime.CommandResult, error) {
			for range f.commandAwaitsSamples {
				select {
				case <-f.sampled:
				case <-ctx.Done():
					return runtime.CommandResult{}, ctx.Err()
				}
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			f.commands = append(f.commands, command)
			if f.commandErr != nil {
				return runtime.CommandResult{}, f.commandErr
			}
			if command.Name == "unknown" {
				return runtime.CommandResult{Token: command.Token, Outcome: runtime.CommandInvalid}, nil
			}
			return runtime.CommandResult{Token: command.Token, Outcome: runtime.CommandExecuted, Response: []byte(`{"result":"NOT_YOUR_TURN"}`)}, nil
		},
		Resync: func(context.Context) error {
			f.views <- f.resyncView
			return nil
		},
		NextLobbyUpdate: func(ctx context.Context) (runtime.LobbyUpdate, bool) {
			return nextOf(ctx, f.lobby, f.ended)
		},
		NextView: func(ctx context.Context) (runtime.OwnView, bool) {
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

func TestHandlePlayerJoin(t *testing.T) {
	type dependencies struct {
		players *MockplayerAttacher
	}
	type test struct {
		player      *fakePlayer
		clientSends []string
		// clientCloses ends the connection from the client once it received
		// expectedReceived; otherwise it reads until the server ends it.
		clientCloses bool

		expectedReceived    []string
		expectedCloseStatus websocket.StatusCode
		expectedSamples     []runtime.Sample
		expectedCommands    []runtime.Command
		expectedDisconnects int
	}

	const (
		callerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		ownerEpoch  = int64(3)
	)
	accept := func(deps dependencies, player *fakePlayer) {
		deps.players.EXPECT().
			AttachPlayer(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
			Return(runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: player.connection()}, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"samples_are_not_blocked_by_a_pending_command": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			// Deadlocks unless the SAMPLEs sent after it are handled meanwhile.
			player.commandAwaitsSamples = 2
			accept(deps, player)
			return test{
				player: player,
				clientSends: []string{
					`{"type":"command","token":"t1","name":"play_card","payload":{"card":"A"}}`,
					`{"type":"sample","sequence":7,"inputs":{"move":[1,0]}}`,
					`{"type":"sample","sequence":8,"inputs":{"move":[0,1]}}`,
				},
				clientCloses:     true,
				expectedReceived: []string{`{"type":"command_result","token":"t1","outcome":"executed","response":{"result":"NOT_YOUR_TURN"}}`},
				expectedSamples: []runtime.Sample{
					{Sequence: 7, Inputs: []byte(`{"move":[1,0]}`)},
					{Sequence: 8, Inputs: []byte(`{"move":[0,1]}`)},
				},
				expectedCommands:    []runtime.Command{{Token: "t1", Name: "play_card", Payload: []byte(`{"card":"A"}`)}},
				expectedDisconnects: 1,
			}
		},
		"command_without_payload_or_response": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"command","token":"t2","name":"unknown","payload":null}`},
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"command_result","token":"t2","outcome":"invalid"}`},
				expectedCommands:    []runtime.Command{{Token: "t2", Name: "unknown"}},
				expectedDisconnects: 1,
			}
		},
		"views_carry_the_sample_acknowledgement": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.views <- runtime.OwnView{
				PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Full: true, State: []byte(`{"x":3}`)},
				ConsumedThroughSampleSequence: 22,
			}
			accept(deps, player)
			return test{
				player:              player,
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"player_view","participant_id":"P1","full":true,"state":{"x":3},"consumed_through_sample_sequence":22}`},
				expectedDisconnects: 1,
			}
		},
		"resync_is_followed_by_a_full_view": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.resyncView = runtime.OwnView{
				PlayerView:                    runtime.PlayerView{ParticipantID: "P2", Full: true, State: []byte(`{"y":1}`)},
				ConsumedThroughSampleSequence: 5,
			}
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"resync"}`},
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"player_view","participant_id":"P2","full":true,"state":{"y":1},"consumed_through_sample_sequence":5}`},
				expectedDisconnects: 1,
			}
		},
		"lobby_updates_are_delivered": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: callerUUID}}}
			accept(deps, player)
			return test{
				player:              player,
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"lobby_update","members":[{"user_uuid":"` + callerUUID + `"}]}`},
				expectedDisconnects: 1,
			}
		},
		"leaving_is_answered_then_the_connection_closes": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"leave"}`},
				expectedReceived:    []string{`{"type":"leave_result","outcome":"left"}`},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"connection_ended_for_an_owner_change_asks_the_client_to_reconnect": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.end(runtime.EndOwnerChanged)
			accept(deps, player)
			return test{
				player:              player,
				expectedCloseStatus: websocket.StatusServiceRestart,
				expectedDisconnects: 1,
			}
		},
		"sample_without_a_sequence_is_malformed": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"sample","inputs":{"move":[1,0]}}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedDisconnects: 1,
			}
		},
		"sample_inputs_must_be_an_object": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"sample","sequence":1,"inputs":[1,0]}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedDisconnects: 1,
			}
		},
		"command_without_a_token_is_malformed": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"command","name":"play_card"}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedDisconnects: 1,
			}
		},
		"admin_messages_are_not_accepted_from_a_player": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"start_session"}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
				expectedDisconnects: 1,
			}
		},
		"runtime_operation_failing_is_a_server_failure": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.commandErr = errors.New("runtime down")
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"command","token":"t3","name":"play_card"}`},
				expectedCloseStatus: websocket.StatusInternalError,
				expectedCommands:    []runtime.Command{{Token: "t3", Name: "play_card"}},
				expectedDisconnects: 1,
			}
		},
		"rejected_connection_receives_the_reason_and_closes": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: runtime.RejectLobbyFull}, nil)
			return test{
				expectedReceived:    []string{`{"type":"rejected","reason":"lobby_full"}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"attach_fails": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachPlayerResult{}, errors.New("runtime down"))
			return test{expectedCloseStatus: websocket.StatusInternalError}
		},
		"accepted_without_operations": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, callerUUID, ownerEpoch).
				Return(runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted}, nil)
			return test{expectedCloseStatus: websocket.StatusInternalError}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{players: NewMockplayerAttacher(ctrl)}
			tc := setup(t, deps)

			connections := live.New()
			handler := NewHandler(nil, nil, nil, nil, deps.players, nil, connections, nil)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				ctx := withRoutedSession(auth.WithUserUUID(r.Context(), callerUUID), sessionUUID, ownerEpoch)
				handler.handlePlayerJoin(w, r.WithContext(ctx))
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL+"/sessions/"+sessionUUID+"/player", &websocket.DialOptions{
				Subprotocols: []string{"playhoot.v1"},
			})
			require.NoError(t, err)
			for _, message := range tc.clientSends {
				require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(message)))
			}

			var received []string
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
			require.Len(t, received, len(tc.expectedReceived))
			for i, expected := range tc.expectedReceived {
				require.JSONEq(t, expected, received[i])
			}

			select {
			case <-handlerDone:
			case <-ctx.Done():
				t.Fatal("handler did not return")
			}
			require.NoError(t, connections.Shutdown(ctx), "the served connection must end")
			if tc.player != nil {
				tc.player.mu.Lock()
				defer tc.player.mu.Unlock()
				require.Equal(t, tc.expectedSamples, tc.player.samples)
				require.Equal(t, tc.expectedCommands, tc.player.commands)
				require.Equal(t, tc.expectedDisconnects, tc.player.disconnects)
			}
		})
	}
}
