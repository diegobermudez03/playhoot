package me

import (
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

// deletionResponse says the identity is gone. The client has nothing left to
// authenticate with: the credentials it holds stopped working as this
// answer was written.
type deletionResponse struct {
	Deleted bool `json:"deleted"`
}

// handleDelete ends the caller's own identity. It deletes nobody else:
// there is no other identity this endpoint can name.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	_, err := h.deletion.DeleteUser(ctx, caller.UUID, identity.DeleteRequested)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identity.ErrUserNotFound) {
			// The credential named a User that is not there: nothing is
			// left to delete, which is what the caller asked for.
			w.Header().Set("Cache-Control", "no-store")
			httpx.WriteJSON(w, http.StatusOK, deletionResponse{Deleted: true})
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "deleting the identity failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, deletionResponse{Deleted: true})
}
