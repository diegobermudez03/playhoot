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

// Admin wire messages, besides the ones every live connection shares.
const (
	adminSelectPerspective       = "select_perspective"
	adminStartSession            = "start_session"
	adminCancelSession           = "cancel_session"
	adminSelectPerspectiveResult = "select_perspective_result"
	adminStartSessionResult      = "start_session_result"
	adminCancelSessionResult     = "cancel_session_result"
)

type adminSelectPerspectiveRequest struct {
	ParticipantID string `json:"participant_id"`
}

type adminPlayerViewMessage struct {
	Type          string          `json:"type"`
	ParticipantID string          `json:"participant_id"`
	Full          bool            `json:"full"`
	State         json.RawMessage `json:"state"`
}

var errIncompleteAdminConnection = errors.New("admin connection is missing operations")

// handleAdminJoin upgrades the creator's admin connection and hands it to the
// live connections. Admission is the Runtime's decision: an open WebSocket is
// not yet admitted.
func (h *Handler) handleAdminJoin(w http.ResponseWriter, r *http.Request) {
	routed, ok := h.acceptRouted(w, r)
	if !ok {
		return
	}
	ctx, conn := routed.ctx, routed.conn

	attached, err := h.admins.AttachAdmin(ctx, routed.sessionUUID, routed.userUUID, routed.ownerEpoch)
	if err != nil {
		logging.LogError(ctx, err)
		wsx.Abort(conn)
		return
	}
	if !admitted(ctx, conn, attached.Outcome, attached.Rejection) {
		return
	}

	admin := attached.Connection
	if admin.SelectPerspective == nil || admin.StartSession == nil || admin.CancelSession == nil ||
		admin.NextLobbyUpdate == nil || admin.NextView == nil || admin.Ended == nil || admin.Disconnect == nil {
		logging.LogError(ctx, errIncompleteAdminConnection)
		wsx.Abort(conn)
		return
	}
	session := &adminSession{liveSession: newLiveSession(conn), admin: admin}
	loops := session.serveRequests(map[string]live.Handler{
		adminSelectPerspective: session.selectPerspective,
		adminStartSession:      session.startSession,
		adminCancelSession:     session.cancelSession,
	})
	h.live.Serve(ctx, conn, admin.Disconnect, append(loops,
		session.sendLobbyUpdates(admin.NextLobbyUpdate),
		sendStream(session.liveSession, admin.NextView, session.viewMessage),
		session.closeOnRuntimeEnd(admin.Ended),
	)...)
}

// adminSession is one served admin connection.
type adminSession struct {
	*liveSession
	admin runtime.AdminConnection
}

func (s *adminSession) selectPerspective(ctx context.Context, data []byte) (live.End, bool) {
	var request adminSelectPerspectiveRequest
	if err := json.Unmarshal(data, &request); err != nil || request.ParticipantID == "" {
		return live.CloseMalformed(s.conn), true
	}
	result, err := s.admin.SelectPerspective(ctx, request.ParticipantID)
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, outcomeMessage{Type: adminSelectPerspectiveResult, Outcome: string(result.Outcome)})
}

func (s *adminSession) startSession(ctx context.Context, _ []byte) (live.End, bool) {
	result, err := s.admin.StartSession(ctx)
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, outcomeMessage{Type: adminStartSessionResult, Outcome: string(result.Outcome)})
}

func (s *adminSession) cancelSession(ctx context.Context, _ []byte) (live.End, bool) {
	result, err := s.admin.CancelSession(ctx)
	if err != nil {
		return live.CloseServerFailure(s.conn, err), true
	}
	return live.WriteMessage(ctx, s.conn, outcomeMessage{Type: adminCancelSessionResult, Outcome: string(result.Outcome)})
}

// viewMessage is a view of the selected participant, without the SAMPLE
// acknowledgement only their own connection receives.
func (s *adminSession) viewMessage(view runtime.PlayerView) (any, bool) {
	return adminPlayerViewMessage{
		Type:          livePlayerView,
		ParticipantID: view.ParticipantID,
		Full:          view.Full,
		State:         gameJSON(view.State),
	}, false
}
