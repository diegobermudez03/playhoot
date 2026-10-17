package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Admin wire messages: every frame is a JSON object whose "type" names it.
const (
	adminSelectPerspective       = "select_perspective"
	adminCancelSession           = "cancel_session"
	adminSelectPerspectiveResult = "select_perspective_result"
	adminCancelSessionResult     = "cancel_session_result"
	adminPlayerView              = "player_view"
	adminRejected                = "rejected"
)

type adminSelectPerspectiveRequest struct {
	ParticipantID string `json:"participant_id"`
}

type adminOutcomeMessage struct {
	Type    string `json:"type"`
	Outcome string `json:"outcome"`
}

type adminPlayerViewMessage struct {
	Type          string          `json:"type"`
	ParticipantID string          `json:"participant_id"`
	Full          bool            `json:"full"`
	State         json.RawMessage `json:"state"`
}

type adminRejectedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

var errIncompleteAdminConnection = errors.New("admin connection is missing operations")

// handleAdminJoin upgrades the creator's admin connection and hands it to the
// live connections. Admission is the Runtime's decision: an open WebSocket is
// not yet admitted.
func (h *Handler) handleAdminJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userUUID, callerOK := auth.UserUUID(ctx)
	sessionUUID, ownerEpoch, routedOK := routedSessionFrom(ctx)
	if !callerOK || !routedOK {
		// Only possible if the route was registered without its middlewares.
		logging.LogFields(ctx, logging.Field("wiring_error", "admin join reached without an authorized caller or a routed session"))
		httpx.WriteError(w, http.StatusInternalServerError, "joining session failed")
		return
	}

	conn, err := wsx.Accept(w, r, h.webSocketOriginPatterns)
	if err != nil {
		// Accept already answered the request.
		logging.LogError(ctx, err)
		return
	}
	// The request's context must not be used past the upgrade; the
	// connection keeps its values (trace, request log) but not its lifetime.
	ctx = context.WithoutCancel(ctx)

	attached, err := h.admins.AttachAdmin(ctx, sessionUUID, userUUID, ownerEpoch)
	if err != nil {
		logging.LogError(ctx, err)
		wsx.Abort(conn)
		return
	}
	logging.LogFields(ctx, logging.Field("attach_outcome", string(attached.Outcome)))

	switch attached.Outcome {
	case runtime.AttachRejected:
		logging.LogFields(ctx, logging.Field("reject_reason", string(attached.Rejection)))
		_, _ = live.WriteMessage(ctx, conn, adminRejectedMessage{Type: adminRejected, Reason: string(attached.Rejection)})
		_ = conn.Close(websocket.StatusPolicyViolation, "admission rejected")
		return
	case runtime.AttachAccepted:
	default:
		logging.LogFields(ctx, logging.Field("attach_error", "unknown admission outcome"))
		wsx.Abort(conn)
		return
	}

	admin := attached.Connection
	if admin.SelectPerspective == nil || admin.CancelSession == nil || admin.NextView == nil || admin.Ended == nil || admin.Disconnect == nil {
		logging.LogError(ctx, errIncompleteAdminConnection)
		wsx.Abort(conn)
		return
	}
	session := newAdminSession(conn, admin)
	h.live.Serve(ctx, conn, admin.Disconnect, session.answerRequests, session.sendViews, session.closeOnRuntimeEnd)
}

// adminSession is one served admin connection.
type adminSession struct {
	conn  *websocket.Conn
	admin runtime.AdminConnection

	// answering is held while a request is answered, so the connection is
	// never closed between the Runtime's answer and its delivery.
	answering sync.Mutex
	// viewsSent is closed once every view the Runtime produced was sent.
	viewsSent chan struct{}
}

func newAdminSession(conn *websocket.Conn, admin runtime.AdminConnection) *adminSession {
	return &adminSession{conn: conn, admin: admin, viewsSent: make(chan struct{})}
}

// answerRequests answers each request the admin sends, in order.
func (s *adminSession) answerRequests(ctx context.Context) (live.End, bool) {
	for {
		messageType, data, end, ended := live.ReadMessage(ctx, s.conn)
		if ended || messageType == "" {
			return end, ended
		}
		if end, ended := s.answer(ctx, messageType, data); ended || ctx.Err() != nil {
			return end, ended
		}
	}
}

func (s *adminSession) answer(ctx context.Context, messageType string, data []byte) (live.End, bool) {
	s.answering.Lock()
	defer s.answering.Unlock()

	var response adminOutcomeMessage
	switch messageType {
	case adminSelectPerspective:
		var request adminSelectPerspectiveRequest
		if err := json.Unmarshal(data, &request); err != nil || request.ParticipantID == "" {
			return live.CloseMalformed(s.conn), true
		}
		result, err := s.admin.SelectPerspective(ctx, request.ParticipantID)
		if err != nil {
			return live.CloseServerFailure(s.conn, err), true
		}
		response = adminOutcomeMessage{Type: adminSelectPerspectiveResult, Outcome: string(result.Outcome)}
	case adminCancelSession:
		result, err := s.admin.CancelSession(ctx)
		if err != nil {
			return live.CloseServerFailure(s.conn, err), true
		}
		response = adminOutcomeMessage{Type: adminCancelSessionResult, Outcome: string(result.Outcome)}
	default:
		return live.CloseUnknownType(s.conn, messageType), true
	}
	return live.WriteMessage(ctx, s.conn, response)
}

// sendViews sends every view of the selected participant, in order.
func (s *adminSession) sendViews(ctx context.Context) (live.End, bool) {
	defer close(s.viewsSent)
	for {
		view, ok := s.admin.NextView(ctx)
		if !ok {
			return live.End{}, false
		}
		message := adminPlayerViewMessage{
			Type:          adminPlayerView,
			ParticipantID: view.ParticipantID,
			Full:          view.Full,
			State:         view.State,
		}
		if end, ended := live.WriteMessage(ctx, s.conn, message); ended || ctx.Err() != nil {
			return end, ended
		}
	}
}

// closeOnRuntimeEnd closes the connection once the Runtime ends it, after its
// last views and any answer in progress were delivered.
func (s *adminSession) closeOnRuntimeEnd(ctx context.Context) (live.End, bool) {
	end, ok := s.admin.Ended(ctx)
	if !ok {
		return live.End{}, false
	}
	select {
	case <-s.viewsSent:
	case <-ctx.Done():
		return live.End{}, false
	}
	s.answering.Lock()
	defer s.answering.Unlock()

	status := websocket.StatusInternalError
	if end.Reason == runtime.EndSessionEnded || end.Reason == runtime.EndDisconnected {
		status = websocket.StatusNormalClosure
	}
	_ = s.conn.Close(status, string(end.Reason))
	return live.End{Cause: live.CauseEndedByOwner, Detail: string(end.Reason)}, true
}
