package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// Player wire messages, besides the ones every live connection shares.
const (
	playerLeave         = "leave"
	playerSample        = "sample"
	playerCommand       = "command"
	playerResync        = "resync"
	playerGameFault     = "game_fault"
	playerLeaveResult   = "leave_result"
	playerCommandResult = "command_result"
	playerRosterUpdate  = "roster_update"
)

// maxCommandTokenLength bounds a command's idempotency token, which the
// Runtime keeps for the Session's lifetime.
const maxCommandTokenLength = 128

type playerSampleRequest struct {
	Sequence int64           `json:"sequence"`
	Inputs   json.RawMessage `json:"inputs"`
}

type playerCommandRequest struct {
	Token   string          `json:"token"`
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

// playerGameFaultRequest reports that the client saw the game frontend fail
// or misbehave. kind is one of the Runtime's fault kinds, as on the wire.
type playerGameFaultRequest struct {
	Kind   string          `json:"kind"`
	Detail json.RawMessage `json:"detail"`
}

// maxGameFaultDetailBytes bounds a game fault's detail, which is stored for
// auditing.
const maxGameFaultDetailBytes = 2048

// gameFaultKinds are the fault kinds a client may report, by wire name.
var gameFaultKinds = map[string]runtime.GameFaultKind{
	string(runtime.GameFaultArtifactUnreachable):      runtime.GameFaultArtifactUnreachable,
	string(runtime.GameFaultArtifactHashMismatch):     runtime.GameFaultArtifactHashMismatch,
	string(runtime.GameFaultReadyTimeout):             runtime.GameFaultReadyTimeout,
	string(runtime.GameFaultUnsupportedBridgeVersion): runtime.GameFaultUnsupportedBridgeVersion,
	string(runtime.GameFaultContainerViolation):       runtime.GameFaultContainerViolation,
	string(runtime.GameFaultBridgeAbuse):              runtime.GameFaultBridgeAbuse,
}

type playerCommandResultMessage struct {
	Type     string          `json:"type"`
	Token    string          `json:"token"`
	Outcome  string          `json:"outcome"`
	Response json.RawMessage `json:"response,omitempty"`
}

type playerSessionStartedMessage struct {
	Type          string               `json:"type"`
	SessionUUID   string               `json:"session_uuid"`
	ParticipantID string               `json:"participant_id"`
	Participants  []participantMessage `json:"participants"`
	Frontend      frontendMessage      `json:"frontend"`
}

type playerOwnViewMessage struct {
	Type                          string            `json:"type"`
	ParticipantID                 string            `json:"participant_id"`
	Full                          bool              `json:"full"`
	Revision                      int64             `json:"revision"`
	State                         json.RawMessage   `json:"state"`
	ConsumedThroughSampleSequence int64             `json:"consumed_through_sample_sequence"`
	Errors                        []json.RawMessage `json:"errors,omitempty"`
	Events                        []json.RawMessage `json:"events,omitempty"`
}

// playerRosterUpdateMessage is every participant of the running game with
// whether they play: active, reconnecting, disconnected or left.
type playerRosterUpdateMessage struct {
	Type    string                `json:"type"`
	Members []rosterMemberMessage `json:"members"`
}

type rosterMemberMessage struct {
	participantMessage
	Status string `json:"status"`
}

var errIncompletePlayerConnection = errors.New("player connection is missing operations")

// Player message limits. A SAMPLE beyond them is dropped: each one is a full
// snapshot, so the next one carries the state and sequence gaps are allowed.
// A COMMAND beyond them is answered rate_limited without using its token. A
// resync or leave beyond them is dropped: the one waiting does the same.
var (
	playerSampleLimits = live.LaneLimits{
		Rate:     []ratelimit.Limit{{Count: 60, Per: time.Second}, {Count: 3600, Per: time.Minute}},
		Capacity: 64,
	}
	playerCommandLimits = live.LaneLimits{
		Rate:     []ratelimit.Limit{{Count: 20, Per: time.Second}, {Count: 300, Per: time.Minute}},
		Capacity: 16,
	}
	playerLeaveLimits = live.LaneLimits{
		Rate:     []ratelimit.Limit{{Count: 1, Per: time.Second}},
		Capacity: 1,
	}
	// A client reports a game fault once when it stops the game; beyond these
	// a report is dropped, as the stored one already counts it.
	playerGameFaultLimits = live.LaneLimits{
		Rate:     []ratelimit.Limit{{Count: 1, Per: time.Second}, {Count: 10, Per: time.Minute}},
		Capacity: 2,
	}
)

// handlePlayerJoin upgrades a player's connection and hands it to the live
// connections. Admission into the lobby is the Runtime's decision.
func (h *Handler) handlePlayerJoin(w http.ResponseWriter, r *http.Request) {
	routed, ok := h.acceptRouted(w, r)
	if !ok {
		return
	}
	ctx, conn := routed.ctx, routed.conn
	served := false
	defer func() {
		if !served {
			routed.release()
		}
	}()

	attached, err := h.players.AttachPlayer(ctx, routed.sessionUUID,
		runtime.Player{UserUUID: routed.caller.UUID, DisplayName: routed.caller.DisplayName}, routed.ownerEpoch)
	if err != nil {
		logging.LogError(ctx, err)
		wsx.Abort(conn)
		return
	}
	if !admitted(ctx, conn, attached.Outcome, attached.Rejection) {
		return
	}

	player := attached.Connection
	if player.Leave == nil || player.Sample == nil || player.Command == nil || player.Resync == nil || player.ReportGameFault == nil ||
		player.NextLobbyUpdate == nil || player.Started == nil || player.NextView == nil || player.NextRosterUpdate == nil ||
		player.Ended == nil || player.Disconnect == nil {
		logging.LogError(ctx, errIncompletePlayerConnection)
		wsx.Abort(conn)
		return
	}
	session := &playerSession{liveSession: newLiveSession(conn), player: player}
	commands := playerCommandLimits
	commands.Excess = session.commandRateLimited
	loops := session.serveRequests(map[string]request{
		playerSample:    {handle: session.sample, limits: playerSampleLimits},
		playerCommand:   {handle: session.command, limits: commands},
		playerResync:    {handle: session.resync, limits: resyncLimits},
		playerLeave:     {handle: session.leave, limits: playerLeaveLimits},
		playerGameFault: {handle: session.gameFault, limits: playerGameFaultLimits},
	})
	served = true
	disconnect := func() {
		player.Disconnect()
		routed.release()
	}
	h.live.Serve(ctx, conn, disconnect, append(loops,
		sendSession(session.liveSession, sessionStream[runtime.PlayerStart, runtime.OwnView]{
			sessionUUID:     routed.sessionUUID,
			nextLobbyUpdate: player.NextLobbyUpdate,
			started:         player.Started,
			startMessage:    playerStartedMessage,
			nextView:        player.NextView,
			viewMessage:     ownViewMessage,
		}),
		sendRoster(session.liveSession, player.NextRosterUpdate),
		session.closeOnRuntimeEnd(player.Ended),
	)...)
}

// playerSession is one served player connection.
type playerSession struct {
	*liveSession
	player runtime.PlayerConnection
}

func (s *playerSession) sample(ctx context.Context, data []byte) (live.End, bool) {
	var request playerSampleRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Sequence <= 0 || !isJSONObject(request.Inputs) {
		return live.CloseMalformed(s.conn), true
	}
	if err := s.player.Sample(ctx, runtime.Sample{Sequence: request.Sequence, Inputs: request.Inputs}); err != nil {
		if errors.Is(err, runtime.ErrInvalidSample) {
			// The game does not declare these inputs: a protocol error.
			return live.CloseMalformed(s.conn), true
		}
		return live.CloseServerFailure(s.conn, err), true
	}
	// A SAMPLE is never answered; its acknowledgement comes with the views.
	return live.End{}, false
}

func (s *playerSession) command(ctx context.Context, data []byte) (live.End, bool) {
	var request playerCommandRequest
	if err := json.Unmarshal(data, &request); err != nil ||
		request.Token == "" || len(request.Token) > maxCommandTokenLength || request.Name == "" {
		return live.CloseMalformed(s.conn), true
	}
	payload := []byte(request.Payload)
	if string(payload) == "null" {
		payload = nil
	}
	result, err := s.player.Command(ctx, runtime.Command{Token: request.Token, Name: request.Name, Payload: payload})
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, playerCommandResultMessage{
		Type:     playerCommandResult,
		Token:    request.Token,
		Outcome:  string(result.Outcome),
		Response: gameJSON(result.Response),
	})
}

// commandRateLimited answers a COMMAND sent more often than allowed without
// handing it to the Runtime, so its token stays unused for a retry.
func (s *playerSession) commandRateLimited(ctx context.Context, data []byte) (live.End, bool) {
	var request playerCommandRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Token == "" || len(request.Token) > maxCommandTokenLength {
		return live.CloseMalformed(s.conn), true
	}
	return live.WriteMessage(ctx, s.conn, playerCommandResultMessage{
		Type:    playerCommandResult,
		Token:   request.Token,
		Outcome: string(runtime.CommandRateLimited),
	})
}

func (s *playerSession) resync(ctx context.Context, _ []byte) (live.End, bool) {
	if err := s.player.Resync(ctx); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	// The FULL view it asked for comes as any other view.
	return live.End{}, false
}

// gameFault hands a game fault report to the Runtime, which keeps it for
// auditing. It is never answered: the client already stopped the game.
func (s *playerSession) gameFault(ctx context.Context, data []byte) (live.End, bool) {
	var request playerGameFaultRequest
	kind, known := runtime.GameFaultKind(""), false
	if err := json.Unmarshal(data, &request); err == nil {
		kind, known = gameFaultKinds[request.Kind]
	}
	if !known || !isJSONObject(request.Detail) || len(request.Detail) > maxGameFaultDetailBytes {
		return live.CloseMalformed(s.conn), true
	}
	if err := s.player.ReportGameFault(ctx, runtime.GameFault{Kind: kind, Detail: request.Detail}); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.End{}, false
}

func (s *playerSession) leave(ctx context.Context, _ []byte) (live.End, bool) {
	result, err := s.player.Leave(ctx)
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, outcomeMessage{Type: playerLeaveResult, Outcome: string(result.Outcome)})
}

// playerStartedMessage tells the player which Session started, who plays,
// which participant they are, and the frontend to play with.
func playerStartedMessage(sessionUUID string, start runtime.PlayerStart) any {
	return playerSessionStartedMessage{
		Type:          liveSessionStarted,
		SessionUUID:   sessionUUID,
		ParticipantID: start.ParticipantID,
		Participants:  participantsJSON(start.SessionStart),
		Frontend:      frontendJSON(start.SessionStart),
	}
}

// sendRoster returns a loop that sends the running game's roster updates,
// all after the start the client was sent; none when it was never sent.
func sendRoster(s *liveSession, nextRosterUpdate func(ctx context.Context) (runtime.RosterUpdate, bool)) live.Loop {
	// Counted when the loop is built, so closeOnRuntimeEnd can never miss it.
	s.streams.Add(1)
	return func(ctx context.Context) (live.End, bool) {
		defer s.streams.Done()
		select {
		case <-s.startSent:
		case <-s.sessionSent:
		case <-ctx.Done():
			return live.End{}, false
		}
		if !s.started.Load() {
			return live.End{}, false
		}
		end, ended, _ := sendAll(ctx, s, nextRosterUpdate, func(update runtime.RosterUpdate) (any, bool) {
			return rosterMessage(update), false
		})
		return end, ended
	}
}

// rosterMessage is a roster update on the wire.
func rosterMessage(update runtime.RosterUpdate) any {
	members := make([]rosterMemberMessage, 0, len(update.Members))
	for _, member := range update.Members {
		members = append(members, rosterMemberMessage{participantMessage: participantJSON(member.Participant), Status: string(member.Status)})
	}
	return playerRosterUpdateMessage{Type: playerRosterUpdate, Members: members}
}

// ownViewMessage is the player's own view, with how far their SAMPLEs were
// consumed and the logical errors and events the game produced for them.
func ownViewMessage(view runtime.OwnView) any {
	return playerOwnViewMessage{
		Type:                          livePlayerView,
		ParticipantID:                 view.ParticipantID,
		Full:                          view.Full,
		Revision:                      view.Revision,
		State:                         gameJSON(view.State),
		ConsumedThroughSampleSequence: view.ConsumedThroughSampleSequence,
		Errors:                        gameJSONs(view.Errors),
		Events:                        gameJSONs(view.Events),
	}
}

// gameJSONs are values only the game defines, as JSON; nil when none.
func gameJSONs(values [][]byte) []json.RawMessage {
	var encoded []json.RawMessage
	for _, value := range values {
		encoded = append(encoded, gameJSON(value))
	}
	return encoded
}

// isJSONObject reports whether raw, as decoded from a field, is an object.
func isJSONObject(raw json.RawMessage) bool {
	return len(raw) > 0 && raw[0] == '{'
}
