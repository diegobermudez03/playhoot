package session

import (
	"encoding/json"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
)

// handleCreateSession assumes r's context already carries a started
// request log (Server's own restObservability wraps every REST route
// with one) - it only ever calls logging.LogFields/LogError, never Start
// or FinishRequestLog itself. It still validates and logs the wire
// request - that contract is real - but does not call any domain package
// yet, so it always answers 501: there is nothing behind this endpoint
// to call.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx,
		logging.Field("game_uuid", req.GameUUID),
		logging.Field("host_user_uuid", req.HostUserUUID),
	)

	httpx.WriteError(w, http.StatusNotImplemented, "session creation is not implemented yet")
}
