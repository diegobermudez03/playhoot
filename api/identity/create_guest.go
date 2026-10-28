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

const (
	maxCreateGuestBodyBytes = 4 << 10
	// maxDisplayNameLength only bounds what reaches Identity, which owns the
	// rules of a valid name.
	maxDisplayNameLength = 64
)

// Error codes of guest creation clients branch on.
const (
	codeInvalidDisplayName  = "invalid_display_name"
	codeIdempotencyConflict = "idempotency_conflict"
	codeIdempotencyKeySpent = "idempotency_key_spent"
)

// handleCreateGuest creates a guest User and returns their credentials. The
// idempotency key must be a UUID: with no caller to scope it, it must be
// unguessable, since replaying it returns the guest's credentials; for the
// same reason it is never logged.
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
	created, err := h.guests.CreateGuest(ctx, req.DisplayName, idempotencyKey.String())
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identitypkg.ErrIdempotencyConflict) {
			httpx.WriteCodedError(w, http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "creating guest failed")
		return
	}

	switch created.Outcome {
	case identitypkg.GuestCreated:
		if !complete(created.Credentials) {
			logging.LogFields(ctx, logging.Field("create_guest_error", "created without complete credentials"))
			httpx.WriteError(w, http.StatusInternalServerError, "creating guest failed")
			return
		}
		logging.LogFields(ctx, logging.Field("user_uuid", created.Credentials.User.UUID))
		writeCredentials(w, http.StatusCreated, created.Credentials)
	case identitypkg.GuestInvalidDisplayName:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidDisplayName, "display_name is not allowed")
	case identitypkg.GuestIdempotencyKeySpent:
		httpx.WriteCodedError(w, http.StatusConflict, codeIdempotencyKeySpent, "idempotency key already used; start a new attempt")
	default:
		logging.LogFields(ctx, logging.Field("create_guest_outcome", string(created.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "creating guest failed")
	}
}
