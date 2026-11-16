package identity

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type startEmailLoginRequest struct {
	Email    string `json:"email"`
	Language string `json:"language"`
}

type completeEmailLoginRequest struct {
	Email                string `json:"email"`
	Code                 string `json:"code"`
	ReplacesRefreshToken string `json:"replaces_refresh_token"`
}

const (
	codeInvalidAddress = "invalid_address"
	codeInvalidCode    = "invalid_code"
	codeAttemptGone    = "attempt_gone"
)

// handleStartEmailLogin asks for a one-time code to be sent to an address.
// Its answer is the same whether or not the address has an account, and
// whether or not a code was actually sent, so nothing can be learned from
// it. A guest calling it (with their access token) becomes the account if
// completing the code creates one.
func (h *Handler) handleStartEmailLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req startEmailLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	guestUserUUID := ""
	if caller, ok := auth.Caller(ctx); ok && caller.Level == identitypkg.LevelGuest {
		guestUserUUID = caller.UUID
	}
	result, err := h.emailLogins.Starter.StartEmailLogin(ctx, req.Email, requestLanguage(r, req.Language), guestUserUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "asking for the code failed")
		return
	}
	writeStartAcknowledgement(w, result)
}

// writeStartAcknowledgement answers a code request: the same body for every
// address, or the refusal of a value that is not an address at all.
func writeStartAcknowledgement(w http.ResponseWriter, result identitypkg.StartEmailLoginResult) {
	w.Header().Set("Cache-Control", "no-store")
	if result.Outcome == identitypkg.EmailLoginInvalidAddress {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidAddress, "that is not an email address")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct{}{})
}

// handleCompleteEmailLogin exchanges an address and its code for the
// account's credentials, registering the account if the address had none.
func (h *Handler) handleCompleteEmailLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req completeEmailLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.emailLogins.Completer.CompleteEmailLogin(ctx, req.Email, req.Code, req.ReplacesRefreshToken)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "completing the sign-in failed")
		return
	}
	switch result.Outcome {
	case identitypkg.EmailLoginCompleted:
		if !complete(result.Credentials) {
			logging.LogFields(ctx, logging.Field("complete_email_login_error", "completed without complete credentials"))
			httpx.WriteError(w, http.StatusInternalServerError, "completing the sign-in failed")
			return
		}
		logging.LogFields(ctx, logging.Field("user_uuid", result.Credentials.User.UUID))
		writeCredentials(w, http.StatusOK, result.Credentials)
	case identitypkg.EmailLoginInvalidCode:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidCode, "that code is not the one sent to this address")
	case identitypkg.EmailLoginAttemptGone:
		httpx.WriteCodedError(w, http.StatusGone, codeAttemptGone, "that code is no longer valid; ask for a new one")
	default:
		logging.LogFields(ctx, logging.Field("complete_email_login_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "completing the sign-in failed")
	}
}

// requestLanguage is the language the code's email is written in: the one
// the client asked in, else the first one Playhoot speaks that the browser
// prefers, else none, which Notifications answers with its default.
func requestLanguage(r *http.Request, requested string) string {
	if identitypkg.ValidLanguage(requested) {
		return requested
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		primary, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if slices.Contains(identitypkg.Languages, identitypkg.Language(primary)) {
			return primary
		}
	}
	return ""
}
