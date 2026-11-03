package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// previewAttacher attaches a draft owner's connection to their preview.
type previewAttacher interface {
	AttachPreview(ctx context.Context, previewUUID string, ownerUUID string) (runtime.AttachPreviewResult, error)
}

const (
	previewStarted   = "preview_started"
	previewActingAs  = "acting_as"
	previewPaused    = "paused"
	previewResumed   = "resumed"
	previewEnded     = "preview_ended"
	previewError     = "error"
	previewActAs     = "act_as"
	previewPause     = "pause"
	previewResume    = "resume"
	previewEnd       = "end"
	previewNotFound  = "not_found"
	previewReplaced  = "replaced"
	codeInvalidData  = "invalid_game_data"
	codeUnknownActor = "unknown_participant"
)

// A preview's owner switches, pauses and ends rarely: these bound a client
// sending them in a loop. Its game traffic takes a player's limits.
var previewControlLimits = live.LaneLimits{
	Rate:     []ratelimit.Limit{{Count: 10, Per: time.Second}, {Count: 300, Per: time.Minute}},
	Capacity: 8,
}

type previewActAsRequest struct {
	ParticipantID string `json:"participant_id"`
}

type previewStartedMessage struct {
	Type         string          `json:"type"`
	PreviewUUID  string          `json:"preview_uuid"`
	Revision     int64           `json:"revision"`
	Participants []string        `json:"participants"`
	ActingAs     string          `json:"acting_as"`
	Paused       bool            `json:"paused"`
	Frontend     frontendMessage `json:"frontend"`
}

type previewActingAsMessage struct {
	Type          string `json:"type"`
	ParticipantID string `json:"participant_id"`
}

type previewNoticeMessage struct {
	Type string `json:"type"`
}

type previewEndedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type previewErrorMessage struct {
	Type   string `json:"type"`
	Code   string `json:"code"`
	Token  string `json:"token,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// handlePreviewConnect upgrades a draft owner's connection to their preview.
// Anyone else is told it does not exist, as an ended one is told it ended.
func (h *Handler) handlePreviewConnect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		// Only possible if the route was registered without its authorization.
		logging.LogFields(ctx, logging.Field("wiring_error", "preview connection reached without an authorized caller"))
		httpx.WriteError(w, http.StatusInternalServerError, "connecting to preview failed")
		return
	}
	conn, release, ok := h.upgrade(w, r, caller.UUID)
	if !ok {
		return
	}
	ctx = context.WithoutCancel(ctx)
	served := false
	defer func() {
		if !served {
			release()
		}
	}()

	previewUUID, err := uuid.Parse(r.PathValue("previewUUID"))
	if err != nil {
		reject(ctx, conn, rejectedMessage{Reason: previewNotFound})
		return
	}
	logging.LogFields(ctx, logging.Field("preview_uuid", previewUUID.String()))
	attached, err := h.previews.AttachPreview(ctx, previewUUID.String(), caller.UUID)
	if err != nil {
		logging.LogError(ctx, err)
		wsx.Abort(conn)
		return
	}
	switch attached.Outcome {
	case runtime.PreviewAttached:
	case runtime.PreviewAlreadyEnded:
		if end, ended := live.WriteMessage(ctx, conn, previewEndedMessage{Type: previewEnded, Reason: attached.EndReason}); !ended {
			_ = conn.Close(websocket.StatusNormalClosure, previewEnded)
		} else {
			logging.LogFields(ctx, logging.Field("preview_end_write", end.Detail))
		}
		return
	default:
		reject(ctx, conn, rejectedMessage{Reason: previewNotFound})
		return
	}

	preview := attached.Connection
	if end, ended := live.WriteMessage(ctx, conn, previewStartedMessageOf(attached.Start)); ended {
		logging.LogFields(ctx, logging.Field("preview_start_write", end.Detail))
		preview.Disconnect()
		return
	}
	session := &previewSession{liveSession: newLiveSession(conn), preview: preview}
	commands := playerCommandLimits
	commands.Excess = session.commandRateLimited
	noAudit := func(context.Context, string) {}
	loops := session.serveRequests(map[string]request{
		playerSample:  {handle: session.sample, limits: playerSampleLimits},
		playerCommand: {handle: session.command, limits: commands},
		playerResync:  {handle: session.resync, limits: resyncLimits},
		previewActAs:  {handle: session.actAs, limits: previewControlLimits, lane: previewActAs},
		previewPause:  {handle: session.pause, limits: previewControlLimits, lane: previewActAs},
		previewResume: {handle: session.resume, limits: previewControlLimits, lane: previewActAs},
		previewEnd:    {handle: session.end, limits: previewControlLimits, lane: previewActAs},
	}, noAudit)
	served = true
	disconnect := func() {
		preview.Disconnect()
		release()
	}
	h.live.Serve(ctx, conn, disconnect, append(loops,
		session.sendViews(attached.Start.ActingAs),
		session.closeOnPreviewEnd(),
	)...)
}

func previewStartedMessageOf(start runtime.PreviewStart) previewStartedMessage {
	return previewStartedMessage{
		Type: previewStarted, PreviewUUID: start.PreviewUUID, Revision: start.Revision, Participants: start.Participants,
		ActingAs: start.ActingAs, Paused: start.Paused, Frontend: frontendJSON(runtime.SessionStart{Frontend: start.Frontend}),
	}
}

// previewSession is one served preview connection.
type previewSession struct {
	*liveSession
	preview runtime.PreviewConnection
}

// sendViews returns a loop that sends the acted participant's views, telling
// the client the participant it acts as before the first view of a new one.
func (s *previewSession) sendViews(actingAs string) live.Loop {
	s.streams.Add(1)
	return func(ctx context.Context) (live.End, bool) {
		defer s.streams.Done()
		for {
			view, ok := s.preview.NextView(ctx)
			if !ok {
				return live.End{}, false
			}
			if view.ParticipantID != actingAs {
				actingAs = view.ParticipantID
				if end, ended := live.WriteMessage(ctx, s.conn, previewActingAsMessage{Type: previewActingAs, ParticipantID: actingAs}); ended {
					return end, ended
				}
			}
			if end, ended := live.WriteMessage(ctx, s.conn, ownViewMessage(view)); ended || ctx.Err() != nil {
				return end, ended
			}
		}
	}
}

// closeOnPreviewEnd closes the connection once the Runtime ends it, after
// every view was delivered, telling the client why the preview ended.
func (s *previewSession) closeOnPreviewEnd() live.Loop {
	return func(ctx context.Context) (live.End, bool) {
		end, ok := s.preview.Ended(ctx)
		if !ok {
			return live.End{}, false
		}
		delivered := make(chan struct{})
		go func() {
			s.streams.Wait()
			close(delivered)
		}()
		select {
		case <-delivered:
		case <-ctx.Done():
			return live.End{}, false
		}
		s.answering.Lock()
		defer s.answering.Unlock()

		reason := end.EndCause
		if end.Reason == runtime.EndReplaced {
			reason = previewReplaced
		}
		if reason != "" {
			if end, ended := live.WriteMessage(ctx, s.conn, previewEndedMessage{Type: previewEnded, Reason: reason}); ended {
				return end, ended
			}
		}
		_ = s.conn.Close(websocket.StatusNormalClosure, string(end.Reason))
		return live.End{Cause: live.CauseEndedByOwner, Detail: string(end.Reason)}, true
	}
}

// sample plays a SAMPLE as the acted participant. One the game does not
// declare is told back instead of closing the connection: a preview is where
// such mistakes are found.
func (s *previewSession) sample(ctx context.Context, data []byte) (live.End, bool) {
	var request playerSampleRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Sequence <= 0 || !isJSONObject(request.Inputs) {
		return live.CloseMalformed(s.conn), true
	}
	err := s.preview.Sample(ctx, runtime.Sample{Sequence: request.Sequence, Inputs: request.Inputs})
	if errors.Is(err, runtime.ErrInvalidSample) {
		return live.WriteMessage(ctx, s.conn, previewErrorMessage{Type: previewError, Code: codeInvalidData, Detail: err.Error()})
	}
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.End{}, false
}

// command plays a COMMAND as the acted participant; one the game does not
// declare is told back, unanswered as a command.
func (s *previewSession) command(ctx context.Context, data []byte) (live.End, bool) {
	var request playerCommandRequest
	if err := json.Unmarshal(data, &request); err != nil ||
		request.Token == "" || len(request.Token) > maxCommandTokenLength || request.Name == "" {
		return live.CloseMalformed(s.conn), true
	}
	payload := []byte(request.Payload)
	if string(payload) == "null" {
		payload = nil
	}
	result, err := s.preview.Command(ctx, runtime.Command{Token: request.Token, Name: request.Name, Payload: payload})
	if errors.Is(err, runtime.ErrInvalidCommand) {
		return live.WriteMessage(ctx, s.conn, previewErrorMessage{Type: previewError, Code: codeInvalidData, Token: request.Token, Detail: err.Error()})
	}
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, playerCommandResultMessage{
		Type: playerCommandResult, Token: request.Token, Outcome: string(result.Outcome), Response: gameJSON(result.Response),
	})
}

// commandRateLimited answers a COMMAND beyond the limits without handing it
// to the Runtime, as for players.
func (s *previewSession) commandRateLimited(ctx context.Context, data []byte) (live.End, bool) {
	var request playerCommandRequest
	if err := json.Unmarshal(data, &request); err != nil || request.Token == "" || len(request.Token) > maxCommandTokenLength {
		return live.CloseMalformed(s.conn), true
	}
	return live.WriteMessage(ctx, s.conn, playerCommandResultMessage{Type: playerCommandResult, Token: request.Token, Outcome: string(runtime.CommandRateLimited)})
}

func (s *previewSession) resync(ctx context.Context, _ []byte) (live.End, bool) {
	if err := s.preview.Resync(ctx); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.End{}, false
}

// actAs switches the participant played as; acting_as and a FULL view come
// as views do.
func (s *previewSession) actAs(ctx context.Context, data []byte) (live.End, bool) {
	var request previewActAsRequest
	if err := json.Unmarshal(data, &request); err != nil || request.ParticipantID == "" {
		return live.CloseMalformed(s.conn), true
	}
	err := s.preview.ActAs(ctx, request.ParticipantID)
	if errors.Is(err, runtime.ErrUnknownParticipant) {
		return live.WriteMessage(ctx, s.conn, previewErrorMessage{Type: previewError, Code: codeUnknownActor, Detail: request.ParticipantID})
	}
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.End{}, false
}

func (s *previewSession) pause(ctx context.Context, _ []byte) (live.End, bool) {
	if err := s.preview.Pause(ctx); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, previewNoticeMessage{Type: previewPaused})
}

func (s *previewSession) resume(ctx context.Context, _ []byte) (live.End, bool) {
	if err := s.preview.Resume(ctx); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, previewNoticeMessage{Type: previewResumed})
}

// end ends the preview; preview_ended follows when the Runtime ends the
// connection.
func (s *previewSession) end(ctx context.Context, _ []byte) (live.End, bool) {
	if err := s.preview.End(ctx); err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.End{}, false
}
