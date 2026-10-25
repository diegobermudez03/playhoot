package play

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// fakePlayer is a Runtime player connection whose streams and end the test
// controls, recording what the handler asked of it.
type fakePlayer struct {
	*fakeLobby[runtime.PlayerStart]
	views      chan runtime.OwnView
	rosters    chan runtime.RosterUpdate
	commandErr error
	// sampleErr is what Sample answers, after recording the SAMPLE.
	sampleErr error
	// commandAwaitsSamples makes Command wait until this many SAMPLEs arrived.
	commandAwaitsSamples int
	sampled              chan struct{}
	// resyncView is produced by Resync.
	resyncView runtime.OwnView

	endOnce sync.Once
	ended   chan struct{}
	ending  runtime.ConnectionEnd

	mu          sync.Mutex
	samples     []runtime.Sample
	commands    []runtime.Command
	faults      []runtime.GameFault
	limitHits   []string
	disconnects int
}

func newFakePlayer() *fakePlayer {
	return &fakePlayer{
		fakeLobby: newFakeLobby[runtime.PlayerStart](),
		views:     make(chan runtime.OwnView, 4),
		rosters:   make(chan runtime.RosterUpdate, 4),
		sampled:   make(chan struct{}, 8),
		ended:     make(chan struct{}),
	}
}

// startedFakePlayer returns a fake player, P2, of a Session that started with
// roster.
func startedFakePlayer() *fakePlayer {
	player := newFakePlayer()
	player.start(runtime.PlayerStart{SessionStart: roster, ParticipantID: "P2"})
	return player
}

// playerStartedJSON is the start startedFakePlayer reports, on the wire.
const playerStartedJSON = `{"type":"session_started","participant_id":"P2",` + rosterJSON + `}`

func (f *fakePlayer) end(reason runtime.EndReason) {
	f.endWith(runtime.ConnectionEnd{Reason: reason})
}

// endWith ends the connection as the Runtime would when the Session ended.
func (f *fakePlayer) endWith(ending runtime.ConnectionEnd) {
	f.endOnce.Do(func() {
		f.ending = ending
		close(f.ended)
		f.closeLobby()
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
			return f.sampleErr
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
			if command.Name == "replayed" {
				return runtime.CommandResult{Token: command.Token, Outcome: runtime.CommandAlreadyProcessed}, nil
			}
			return runtime.CommandResult{Token: command.Token, Outcome: runtime.CommandExecuted, Response: []byte(`{"result":"NOT_YOUR_TURN"}`)}, nil
		},
		Resync: func(context.Context) error {
			f.views <- f.resyncView
			return nil
		},
		LimitHit: func(_ context.Context, limit string) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.limitHits = append(f.limitHits, limit)
		},
		ReportGameFault: func(_ context.Context, fault runtime.GameFault) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.faults = append(f.faults, fault)
			return nil
		},
		NextLobbyUpdate: f.nextLobbyUpdate,
		Started: func(ctx context.Context) (runtime.PlayerStart, bool) {
			return f.waitStarted(ctx, f.ended)
		},
		NextView: func(ctx context.Context) (runtime.OwnView, bool) {
			return nextOf(ctx, f.views, f.ended)
		},
		NextRosterUpdate: func(ctx context.Context) (runtime.RosterUpdate, bool) {
			return nextOf(ctx, f.rosters, f.ended)
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

func TestHandlePlayerJoin(t *testing.T) {
	type dependencies struct {
		players *MockplayerAttacher
	}
	type test struct {
		player *fakePlayer
		// query is the join URL's query string, with its "?", if any.
		query       string
		clientSends []string
		// receivedBeforeSending is how many of expectedReceived the client
		// reads before it sends clientSends, as a client acting on them would.
		receivedBeforeSending int
		// clientCloses ends the connection from the client once it received
		// expectedReceived; otherwise it reads until the server ends it.
		clientCloses bool
		// callerConnections are the caller's other live connections here.
		callerConnections int

		// expectedRefusal is the status refusing the upgrade, if refused.
		expectedRefusal     int
		expectedReceived    []string
		expectedCloseStatus websocket.StatusCode
		// expectedCloseReason, when set, is the reason the server closed with.
		expectedCloseReason string
		expectedSamples     []runtime.Sample
		expectedCommands    []runtime.Command
		expectedFaults      []runtime.GameFault
		expectedLimitHits   []string
		expectedDisconnects int
	}

	const (
		callerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		callerName  = "Ana"
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		ownerEpoch  = int64(3)
	)
	accept := func(deps dependencies, player *fakePlayer) {
		deps.players.EXPECT().
			AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName}, ownerEpoch).
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
		"command_beyond_its_rate_is_answered_without_reaching_the_runtime": func(t *testing.T, deps dependencies) test {
			previous := playerCommandLimits
			playerCommandLimits = live.LaneLimits{Rate: []ratelimit.Limit{{Count: 1, Per: time.Hour}}, Capacity: 16}
			t.Cleanup(func() { playerCommandLimits = previous })
			player := newFakePlayer()
			// t1 is answered only after the SAMPLE sent behind t2, so t2's
			// refusal always comes first.
			player.commandAwaitsSamples = 1
			accept(deps, player)
			return test{
				player: player,
				clientSends: []string{
					`{"type":"command","token":"t1","name":"play_card"}`,
					`{"type":"command","token":"t2","name":"play_card"}`,
					`{"type":"sample","sequence":1,"inputs":{}}`,
				},
				clientCloses: true,
				expectedReceived: []string{
					`{"type":"command_result","token":"t2","outcome":"rate_limited"}`,
					`{"type":"command_result","token":"t1","outcome":"executed","response":{"result":"NOT_YOUR_TURN"}}`,
				},
				expectedSamples:     []runtime.Sample{{Sequence: 1, Inputs: []byte(`{}`)}},
				expectedCommands:    []runtime.Command{{Token: "t1", Name: "play_card"}},
				expectedLimitHits:   []string{"api.command.rate"},
				expectedDisconnects: 1,
			}
		},
		"game_fault_then_leave_keeps_the_report_before_leaving": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			accept(deps, player)
			return test{
				player: player,
				clientSends: []string{
					`{"type":"game_fault","kind":"bridge_abuse","detail":{}}`,
					`{"type":"leave"}`,
				},
				// The player reports and leaves the game they were told started.
				receivedBeforeSending: 1,
				expectedReceived:      []string{playerStartedJSON, `{"type":"leave_result","outcome":"left"}`},
				expectedCloseStatus:   websocket.StatusNormalClosure,
				expectedFaults:        []runtime.GameFault{{Kind: runtime.GameFaultBridgeAbuse, Detail: []byte(`{}`)}},
				expectedDisconnects:   1,
			}
		},
		"caller_with_as_many_connections_as_allowed_is_told_after_the_upgrade": func(t *testing.T, deps dependencies) test {
			return test{
				callerConnections:   live.MaxConnectionsPerCaller,
				expectedReceived:    []string{`{"type":"rejected","reason":"too_many_connections"}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
				expectedCloseReason: "admission rejected",
			}
		},
		"command_without_payload_or_response": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"command","token":"t2","name":"replayed","payload":null}`},
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"command_result","token":"t2","outcome":"already_processed"}`},
				expectedCommands:    []runtime.Command{{Token: "t2", Name: "replayed"}},
				expectedDisconnects: 1,
			}
		},
		"command_the_game_rejects_is_answered_and_the_connection_stays": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player: player,
				clientSends: []string{
					`{"type":"command","token":"t1","name":"play_card","payload":{"card":3}}`,
					`{"type":"command","token":"t2","name":"play_card","payload":{"card":4}}`,
				},
				clientCloses: true,
				expectedReceived: []string{
					`{"type":"command_result","token":"t1","outcome":"executed","response":{"result":"NOT_YOUR_TURN"}}`,
					`{"type":"command_result","token":"t2","outcome":"executed","response":{"result":"NOT_YOUR_TURN"}}`,
				},
				expectedCommands: []runtime.Command{
					{Token: "t1", Name: "play_card", Payload: []byte(`{"card":3}`)},
					{Token: "t2", Name: "play_card", Payload: []byte(`{"card":4}`)},
				},
				expectedDisconnects: 1,
			}
		},
		"command_the_game_does_not_declare_ends_the_connection_unanswered": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.commandErr = fmt.Errorf("%w: undeclared command: fly", runtime.ErrInvalidCommand)
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"command","token":"t1","name":"fly","payload":{}}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseReason: "invalid_game_data",
				expectedCommands:    []runtime.Command{{Token: "t1", Name: "fly", Payload: []byte(`{}`)}},
				expectedDisconnects: 1,
			}
		},
		"message_without_a_type_is_malformed_and_reaches_no_one": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"sequence":1,"inputs":{"jump":true}}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseReason: "malformed message",
				expectedDisconnects: 1,
			}
		},
		"views_carry_the_sample_acknowledgement": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			player.views <- runtime.OwnView{
				PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 7, State: []byte(`{"x":3}`)},
				ConsumedThroughSampleSequence: 22,
			}
			accept(deps, player)
			return test{
				player:       player,
				clientCloses: true,
				expectedReceived: []string{
					playerStartedJSON,
					`{"type":"player_view","participant_id":"P1","full":true,"revision":7,"state":{"x":3},"consumed_through_sample_sequence":22}`,
				},
				expectedDisconnects: 1,
			}
		},
		"views_carry_the_game_errors_and_events": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			player.views <- runtime.OwnView{
				PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Revision: 2, State: []byte(`{}`)},
				ConsumedThroughSampleSequence: 3,
				Errors:                        [][]byte{[]byte(`{"code":"NOT_YOUR_TURN"}`), []byte(`"OUT_OF_FUEL"`)},
				Events:                        [][]byte{[]byte(`{"type":"EXPLOSION"}`)},
			}
			accept(deps, player)
			return test{
				player:       player,
				clientCloses: true,
				expectedReceived: []string{
					playerStartedJSON,
					`{"type":"player_view","participant_id":"P1","full":false,"revision":2,"state":{},"consumed_through_sample_sequence":3,"errors":[{"code":"NOT_YOUR_TURN"},"OUT_OF_FUEL"],"events":[{"type":"EXPLOSION"}]}`,
				},
				expectedDisconnects: 1,
			}
		},
		"roster_updates_follow_the_start": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.rosters <- runtime.RosterUpdate{Members: []runtime.RosterMember{
				{Participant: roster.Participants[0], Status: runtime.PresenceReconnecting},
				{Participant: roster.Participants[1], Status: runtime.PresenceActive},
			}}
			player.start(runtime.PlayerStart{SessionStart: roster, ParticipantID: "P2"})
			accept(deps, player)
			return test{
				player:       player,
				clientCloses: true,
				expectedReceived: []string{
					playerStartedJSON,
					`{"type":"roster_update","members":[{"participant_id":"P1","user_uuid":"u1","display_name":"Ana","status":"reconnecting"},` +
						`{"participant_id":"P2","user_uuid":"u2","display_name":"Bruno","status":"active"}]}`,
				},
				expectedDisconnects: 1,
			}
		},
		"resync_is_followed_by_a_full_view": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			player.resyncView = runtime.OwnView{
				PlayerView:                    runtime.PlayerView{ParticipantID: "P2", Full: true, Revision: 3, State: []byte(`{"y":1}`)},
				ConsumedThroughSampleSequence: 5,
			}
			accept(deps, player)
			return test{
				player:       player,
				clientSends:  []string{`{"type":"resync"}`},
				clientCloses: true,
				expectedReceived: []string{
					playerStartedJSON,
					`{"type":"player_view","participant_id":"P2","full":true,"revision":3,"state":{"y":1},"consumed_through_sample_sequence":5}`,
				},
				expectedDisconnects: 1,
			}
		},
		"start_follows_the_lobby_and_precedes_the_views": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "u1", DisplayName: "Ana"}, {UserUUID: "u2", DisplayName: "Bruno"}}}
			player.views <- runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, Revision: 1, State: []byte(`{"round":1}`)}}
			player.start(runtime.PlayerStart{SessionStart: roster, ParticipantID: "P2"})
			accept(deps, player)
			return test{
				player:       player,
				clientCloses: true,
				expectedReceived: []string{
					`{"type":"lobby_update",` + sessionUUIDJSON + `,"members":[{"user_uuid":"u1","display_name":"Ana"},{"user_uuid":"u2","display_name":"Bruno"}]}`,
					playerStartedJSON,
					`{"type":"player_view","participant_id":"P2","full":true,"revision":1,"state":{"round":1},"consumed_through_sample_sequence":0}`,
				},
				expectedDisconnects: 1,
			}
		},
		"lobby_closed_without_starting_tells_how": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: callerUUID, DisplayName: callerName}}}
			player.endWith(hostCancelled)
			accept(deps, player)
			return test{
				player: player,
				expectedReceived: []string{
					`{"type":"lobby_update",` + sessionUUIDJSON + `,"members":[{"user_uuid":"` + callerUUID + `","display_name":"` + callerName + `"}]}`,
					`{"type":"lobby_closed","status":"CANCELLED","end_cause":"HOST_CANCELLED"}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"started_session_that_ends_tells_how_and_closes_normally": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			player.views <- runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P2", Revision: 2, State: []byte(`{"winner":"P2"}`)}}
			player.endWith(runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: "FINISHED", EndCause: "GAME_FINISHED"})
			accept(deps, player)
			return test{
				player: player,
				expectedReceived: []string{
					playerStartedJSON,
					`{"type":"player_view","participant_id":"P2","full":false,"revision":2,"state":{"winner":"P2"},"consumed_through_sample_sequence":0}`,
					`{"type":"session_ended","status":"FINISHED","end_cause":"GAME_FINISHED"}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"connection_ended_without_the_session_ending_sends_no_terminal_message": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			player.end(runtime.EndReplaced)
			accept(deps, player)
			return test{
				player:              player,
				expectedReceived:    []string{playerStartedJSON},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"lobby_updates_are_delivered": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.lobby <- runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: callerUUID, DisplayName: callerName}}}
			accept(deps, player)
			return test{
				player:              player,
				clientCloses:        true,
				expectedReceived:    []string{`{"type":"lobby_update",` + sessionUUIDJSON + `,"members":[{"user_uuid":"` + callerUUID + `","display_name":"` + callerName + `"}]}`},
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
		"sample_of_inputs_the_game_does_not_declare_is_invalid_game_data": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			player.sampleErr = fmt.Errorf("%w: jump is not a declared input", runtime.ErrInvalidSample)
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"sample","sequence":1,"inputs":{"jump":true}}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseReason: "invalid_game_data",
				expectedSamples:     []runtime.Sample{{Sequence: 1, Inputs: []byte(`{"jump":true}`)}},
				expectedDisconnects: 1,
			}
		},
		"game_fault_is_handed_to_the_runtime_unanswered": func(t *testing.T, deps dependencies) test {
			player := startedFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"game_fault","kind":"bridge_abuse","detail":{"refused":{"sample":900}}}`},
				clientCloses:        true,
				expectedReceived:    []string{playerStartedJSON},
				expectedFaults:      []runtime.GameFault{{Kind: runtime.GameFaultBridgeAbuse, Detail: []byte(`{"refused":{"sample":900}}`)}},
				expectedDisconnects: 1,
			}
		},
		"game_fault_of_an_unknown_kind_is_malformed": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"game_fault","kind":"cheating","detail":{}}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedDisconnects: 1,
			}
		},
		"game_fault_detail_must_be_an_object": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"game_fault","kind":"ready_timeout","detail":"slow"}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedDisconnects: 1,
			}
		},
		"game_fault_detail_over_its_limit_is_malformed": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			accept(deps, player)
			return test{
				player:              player,
				clientSends:         []string{`{"type":"game_fault","kind":"ready_timeout","detail":{"x":"` + strings.Repeat("a", 2048) + `"}}`},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
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
				expectedCloseReason: "malformed message",
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
				expectedCloseReason: "malformed message",
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
				AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName}, ownerEpoch).
				Return(runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: runtime.RejectLobbyFull}, nil)
			return test{
				expectedReceived:    []string{`{"type":"rejected","reason":"lobby_full"}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"device_is_passed_to_the_runtime": func(t *testing.T, deps dependencies) test {
			player := newFakePlayer()
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName, Device: runtime.DeviceMobile}, ownerEpoch).
				Return(runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: player.connection()}, nil)
			return test{
				player:              player,
				query:               "?device=mobile",
				clientSends:         []string{`{"type":"leave"}`},
				expectedReceived:    []string{`{"type":"leave_result","outcome":"left"}`},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedDisconnects: 1,
			}
		},
		"unsupported_device_receives_the_requirements_and_closes": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName, Device: runtime.Device("tablet")}, ownerEpoch).
				Return(runtime.AttachPlayerResult{
					Outcome:      runtime.AttachRejected,
					Rejection:    runtime.RejectDeviceNotSupported,
					Requirements: runtime.InputRequirements{Devices: "desktop", UsesKeyboard: true, UsesPointerPress: true},
				}, nil)
			return test{
				query: "?device=tablet",
				expectedReceived: []string{`{"type":"rejected","reason":"device_not_supported","requirements":` +
					`{"devices":"desktop","uses_keyboard":true,"uses_pointer_hover":false,"uses_pointer_press":true}}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
				expectedCloseReason: "admission rejected",
			}
		},
		"missing_device_reaches_the_runtime_empty": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName}, ownerEpoch).
				Return(runtime.AttachPlayerResult{
					Outcome:      runtime.AttachRejected,
					Rejection:    runtime.RejectDeviceNotSupported,
					Requirements: runtime.InputRequirements{Devices: "both", UsesPointerPress: true},
				}, nil)
			return test{
				expectedReceived: []string{`{"type":"rejected","reason":"device_not_supported","requirements":` +
					`{"devices":"both","uses_keyboard":false,"uses_pointer_hover":false,"uses_pointer_press":true}}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"attach_fails": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName}, ownerEpoch).
				Return(runtime.AttachPlayerResult{}, errors.New("runtime down"))
			return test{expectedCloseStatus: websocket.StatusInternalError}
		},
		"accepted_without_operations": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().
				AttachPlayer(gomock.Any(), sessionUUID, runtime.Player{UserUUID: callerUUID, DisplayName: callerName}, ownerEpoch).
				Return(runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted}, nil)
			return test{expectedCloseStatus: websocket.StatusInternalError}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{players: NewMockplayerAttacher(ctrl)}
			tc := setup(t, deps)

			connections := live.New(0)
			for range tc.callerConnections {
				_, err := connections.Reserve(context.Background(), callerUUID)
				require.NoError(t, err)
			}
			handler := NewHandler(nil, nil, nil, nil, deps.players, nil, connections, nil)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				ctx := withRoutedSession(auth.WithCaller(r.Context(), identity.User{UUID: callerUUID, DisplayName: callerName}), sessionUUID, ownerEpoch)
				handler.handlePlayerJoin(w, r.WithContext(ctx))
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, server.URL+"/sessions/"+sessionUUID+"/player"+tc.query, &websocket.DialOptions{
				Subprotocols: []string{"playhoot.v1"},
			})
			if tc.expectedRefusal != 0 {
				require.Error(t, err)
				require.Equal(t, tc.expectedRefusal, response.StatusCode)
				return
			}
			require.NoError(t, err)
			var received []string
			for range tc.receivedBeforeSending {
				_, data, err := conn.Read(ctx)
				require.NoError(t, err)
				received = append(received, string(data))
			}
			for _, message := range tc.clientSends {
				require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(message)))
			}

			if tc.clientCloses {
				for range tc.expectedReceived[len(received):] {
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
						if tc.expectedCloseReason != "" {
							var closed websocket.CloseError
							require.ErrorAs(t, err, &closed)
							require.Equal(t, tc.expectedCloseReason, closed.Reason)
						}
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
				require.Equal(t, tc.expectedFaults, tc.player.faults)
				require.Equal(t, tc.expectedLimitHits, tc.player.limitHits)
				require.Equal(t, tc.expectedDisconnects, tc.player.disconnects)
			}
		})
	}
}
