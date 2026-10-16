package session

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/ownerrouting"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// handleAdminJoin upgrades the creator's admin connection and serves it.
// Admission is the Runtime's decision: an open WebSocket is not yet admitted.
func (h *Handler) handleAdminJoin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	defer wsx.FinishHandshake(ctx)

	userUUID, callerOK := auth.UserUUID(ctx)
	sessionUUID, ownerEpoch, routedOK := ownerrouting.OwnedSession(ctx)
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

	attached, err := h.admins.AttachAdmin(ctx, sessionUUID, userUUID, ownerEpoch)
	if err != nil {
		logging.LogError(ctx, err)
		wsx.Abort(conn)
		return
	}
	logging.LogFields(ctx, logging.Field("attach_outcome", string(attached.Outcome)))

	known := attached.Outcome == runtime.AttachAccepted || attached.Outcome == runtime.AttachRejected
	if !known || attached.Connection.Inbound == nil || attached.Connection.Outbound == nil {
		logging.LogFields(ctx, logging.Field("attach_error", "admission result cannot be served"))
		wsx.Abort(conn)
		return
	}

	wsx.FinishHandshake(ctx)
	wsx.Serve(ctx, conn, attached.Connection)
}
