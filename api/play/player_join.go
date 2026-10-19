package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Player wire messages, besides the ones every live connection shares.
const (
	playerLeave         = "leave"
	playerSample        = "sample"
	playerCommand       = "command"
	playerResync        = "resync"
	playerLeaveResult   = "leave_result"
	playerCommandResult = "command_result"
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

type playerCommandResultMessage struct {
	Type     string          `json:"type"`
	Token    string          `json:"token"`
	Outcome  string          `json:"outcome"`
	Response json.RawMessage `json:"response,omitempty"`
}

type playerSessionStartedMessage struct {
	Type          string               `json:"type"`
	ParticipantID string               `json:"participant_id"`
	Participants  []participantMessage `json:"participants"`
	Frontend      frontendMessage      `json:"frontend"`
}

type playerOwnViewMessage struct {
	Type                          string            `json:"type"`
	ParticipantID                 string            `json:"participant_id"`
	Full                          bool              `json:"full"`
	State                         json.RawMessage   `json:"state"`
	ConsumedThroughSampleSequence int64             `json:"consumed_through_sample_sequence"`
	Errors                        []json.RawMessage `json:"errors,omitempty"`
	Events                        []json.RawMessage `json:"events,omitempty"`
}

var errIncompletePlayerConnection = errors.New("player connection is missing operations")

// handlePlayerJoin upgrades a player's connection and hands it to the live
// connections. Admission into the lobby is the Runtime's decision.
func (h *Handler) handlePlayerJoin(w http.ResponseWriter, r *http.Request) {
	routed, ok := h.acceptRouted(w, r)
	if !ok {
		return
	}
	ctx, conn := routed.ctx, routed.conn

	attached, err := h.players.AttachPlayer(ctx, routed.sessionUUID, routed.userUUID, routed.ownerEpoch)
	if err != nil {
		logging.LogError(ctx, err)
		wsx.Abort(conn)
		return
	}
	if !admitted(ctx, conn, attached.Outcome, attached.Rejection) {
		return
	}

	player := attached.Connection
	if player.Leave == nil || player.Sample == nil || player.Command == nil || player.Resync == nil ||
		player.NextLobbyUpdate == nil || player.Started == nil || player.NextView == nil || player.Ended == nil || player.Disconnect == nil {
		logging.LogError(ctx, errIncompletePlayerConnection)
		wsx.Abort(conn)
		return
	}
	session := &playerSession{liveSession: newLiveSession(conn), player: player}
	loops := session.serveRequests(map[string]live.Handler{
		playerSample:  session.sample,
		playerCommand: session.command,
		playerResync:  session.resync,
		playerLeave:   session.leave,
	})
	h.live.Serve(ctx, conn, player.Disconnect, append(loops,
		sendSession(session.liveSession, sessionStream[runtime.PlayerStart, runtime.OwnView]{
			nextLobbyUpdate: player.NextLobbyUpdate,
			started:         player.Started,
			startMessage:    playerStartedMessage,
			nextView:        player.NextView,
			viewMessage:     ownViewMessage,
		}),
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

func (s *playerSession) resync(ctx context.Context, _ []byte) (live.End, bool) {
	if err := s.player.Resync(ctx); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	// The FULL view it asked for comes as any other view.
	return live.End{}, false
}

func (s *playerSession) leave(ctx context.Context, _ []byte) (live.End, bool) {
	result, err := s.player.Leave(ctx)
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, outcomeMessage{Type: playerLeaveResult, Outcome: string(result.Outcome)})
}

// playerStartedMessage tells the player who plays, which participant they
// are, and the frontend to play with.
func playerStartedMessage(start runtime.PlayerStart) any {
	return playerSessionStartedMessage{
		Type:          liveSessionStarted,
		ParticipantID: start.ParticipantID,
		Participants:  participantsJSON(start.SessionStart),
		Frontend:      frontendJSON(start.SessionStart),
	}
}

// ownViewMessage is the player's own view, with how far their SAMPLEs were
// consumed and the logical errors and events the game produced for them.
func ownViewMessage(view runtime.OwnView) any {
	return playerOwnViewMessage{
		Type:                          livePlayerView,
		ParticipantID:                 view.ParticipantID,
		Full:                          view.Full,
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
