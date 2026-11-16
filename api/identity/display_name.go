package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type setDisplayNameRequest struct {
	DisplayName string `json:"display_name"`
}

// changeLimitResponse says when the next name change is possible, so the
// screen can say it instead of only refusing.
type changeLimitResponse struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	NextChangeAt string `json:"next_change_at"`
}

// codeChangeLimitReached means the rolling window allows no further change
// yet.
const codeChangeLimitReached = "change_limit_reached"

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
	case identitypkg.DisplayNameLimitReached:
		httpx.WriteJSON(w, http.StatusTooManyRequests, changeLimitResponse{
			Code:         codeChangeLimitReached,
			Message:      "you have changed your name as often as allowed for now",
			NextChangeAt: result.NextChangeAt.UTC().Format(time.RFC3339),
		})
	default:
		logging.LogFields(ctx, logging.Field("set_display_name_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "choosing the name failed")
	}
}
