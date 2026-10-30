package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type setDisplayNameRequest struct {
	DisplayName string `json:"display_name"`
}

const maxSetDisplayNameBodyBytes = 2 << 10

// handleSetDisplayName names the caller. Their access token keeps the name it
// was issued with until it is renewed, which the client does right after.
func (h *Handler) handleSetDisplayName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}

	var req setDisplayNameRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSetDisplayNameBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DisplayName == "" || utf8.RuneCountInString(req.DisplayName) > maxDisplayNameLength {
		httpx.WriteError(w, http.StatusBadRequest, "display_name is required and must be at most 64 characters")
		return
	}

	result, err := h.displayNames.SetDisplayName(ctx, caller.UUID, req.DisplayName)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identitypkg.ErrUserNotFound) {
			httpx.WriteError(w, http.StatusInternalServerError, "caller not found")
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "choosing the name failed")
		return
	}
	switch result.Outcome {
	case identitypkg.DisplayNameSet:
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, meResponse{User: toUserResponse(result.User)})
	case identitypkg.DisplayNameInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidDisplayName, "display_name is not allowed")
	default:
		logging.LogFields(ctx, logging.Field("set_display_name_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "choosing the name failed")
	}
}
