package identity

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
)

type meResponse struct {
	User userResponse `json:"user"`
}

// handleMe returns who the caller's access token stands for, as the
// authorization middleware resolved it.
func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.Caller(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, meResponse{User: toUserResponse(user)})
}
