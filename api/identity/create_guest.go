package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type createGuestRequest struct {
	DisplayName    string `json:"display_name"`
	IdempotencyKey string `json:"idempotency_key"`
}

type createGuestResponse struct {
	UserUUID string `json:"user_uuid"`
	Token    string `json:"token"`
}

const (
	maxCreateGuestBodyBytes = 4 << 10
	// maxDisplayNameLength only bounds what reaches Identity, which owns the
	// rules of a valid name.
	maxDisplayNameLength = 64
)

// handleCreateGuest creates a guest User and returns their credential. The
// idempotency key must be a UUID: with no caller to scope it, it must be
// unguessable, since replaying it returns the guest's token.
func (h *Handler) handleCreateGuest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createGuestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateGuestBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	idempotencyKey, err := uuid.Parse(req.IdempotencyKey)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key must be a valid UUID")
		return
	}
	if req.DisplayName == "" || utf8.RuneCountInString(req.DisplayName) > maxDisplayNameLength {
		httpx.WriteError(w, http.StatusBadRequest, "display_name is required and must be at most 64 characters")
		return
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", idempotencyKey.String()))

	created, err := h.guests.CreateGuest(ctx, req.DisplayName, idempotencyKey.String())
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identitypkg.ErrIdempotencyConflict) {
			httpx.WriteError(w, http.StatusConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "creating guest failed")
		return
	}

	switch created.Outcome {
	case identitypkg.GuestCreated:
		if created.UserUUID == "" || created.Token == "" {
			logging.LogFields(ctx, logging.Field("create_guest_error", "created without user or token"))
			httpx.WriteError(w, http.StatusInternalServerError, "creating guest failed")
			return
		}
		logging.LogFields(ctx, logging.Field("user_uuid", created.UserUUID))
		httpx.WriteJSON(w, http.StatusCreated, createGuestResponse{UserUUID: created.UserUUID, Token: created.Token})
	case identitypkg.GuestInvalidDisplayName:
		httpx.WriteError(w, http.StatusBadRequest, "display_name is not allowed")
	default:
		logging.LogFields(ctx, logging.Field("create_guest_outcome", string(created.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "creating guest failed")
	}
}
