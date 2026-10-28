package identity

import (
	"encoding/json"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

const (
	maxRefreshBodyBytes = 2 << 10
	codeRefreshInvalid  = "refresh_invalid"
)

// handleRefresh renews the caller's credentials with their refresh token: a
// new access token and the refresh token to keep. It needs no access token.
func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req refreshRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRefreshBodyBytes)).Decode(&req); err != nil || req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "a refresh_token is required")
		return
	}

	result, err := h.refreshes.Refresh(ctx, req.RefreshToken)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "renewing credentials failed")
		return
	}
	switch result.Outcome {
	case identitypkg.RefreshRenewed:
		if !complete(result.Credentials) {
			logging.LogFields(ctx, logging.Field("refresh_error", "renewed without complete credentials"))
			httpx.WriteError(w, http.StatusInternalServerError, "renewing credentials failed")
			return
		}
		logging.LogFields(ctx, logging.Field("user_uuid", result.Credentials.User.UUID))
		writeCredentials(w, http.StatusOK, result.Credentials)
	case identitypkg.RefreshInvalid:
		httpx.WriteCodedError(w, http.StatusUnauthorized, codeRefreshInvalid, "the refresh token is no longer valid")
	default:
		logging.LogFields(ctx, logging.Field("refresh_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "renewing credentials failed")
	}
}
