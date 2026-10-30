package identity

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type startLoginRequest struct {
	Provider      string `json:"provider"`
	CodeChallenge string `json:"code_challenge"`
}

type startLoginResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}

type completeLoginRequest struct {
	Code                 string `json:"code"`
	CodeVerifier         string `json:"code_verifier"`
	ReplacesRefreshToken string `json:"replaces_refresh_token"`
}

const (
	maxLoginBodyBytes = 4 << 10
	// The provider's codes and the one-time code travel in the callback's and
	// the completion route's query strings, which are never logged.
	codeProviderUnavailable  = "provider_unavailable"
	codeInvalidCodeChallenge = "invalid_code_challenge"
	codeLoginCodeInvalid     = "login_code_invalid"
)

// handleStartLogin starts an account's sign-in; the client sends the person
// to the authorization URL and keeps the verifier of its code challenge.
func (h *Handler) handleStartLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req startLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.logins.Starter.StartLogin(ctx, req.Provider, req.CodeChallenge)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "starting the sign-in failed")
		return
	}
	switch result.Outcome {
	case identitypkg.LoginStarted:
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, startLoginResponse{AuthorizationURL: result.AuthorizationURL})
	case identitypkg.LoginProviderUnavailable:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeProviderUnavailable, "this sign-in provider is not available")
	case identitypkg.LoginInvalidCodeChallenge:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidCodeChallenge, "code_challenge must be an S256 hash")
	default:
		logging.LogFields(ctx, logging.Field("start_login_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "starting the sign-in failed")
	}
}

// handleLoginCallback takes the provider's answer and sends the browser to
// the client's sign-in completion route: with a one-time code, or with why
// the sign-in did not happen. Never with credentials.
func (h *Handler) handleLoginCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	result, err := h.logins.Callbacks.HandleCallback(ctx, r.PathValue("provider"), query.Get("state"), query.Get("code"), query.Get("error"))
	completion := url.Values{}
	switch {
	case err != nil:
		logging.LogError(ctx, err)
		completion.Set("error", "unavailable")
	case result.Outcome == identitypkg.LoginCodeIssued:
		completion.Set("code", result.LoginCode)
	case result.Outcome == identitypkg.LoginCancelled:
		completion.Set("error", "cancelled")
	case result.Outcome == identitypkg.LoginUnavailable:
		completion.Set("error", "unavailable")
	default:
		completion.Set("error", "failed")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, h.logins.CompleteURL+"?"+completion.Encode(), http.StatusFound)
}

// handleCompleteLogin exchanges the one-time code and the client's verifier
// for the account's credentials.
func (h *Handler) handleCompleteLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req completeLoginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	result, err := h.logins.Completer.CompleteLogin(ctx, req.Code, req.CodeVerifier, req.ReplacesRefreshToken)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "completing the sign-in failed")
		return
	}
	switch result.Outcome {
	case identitypkg.LoginCompleted:
		if !complete(result.Credentials) {
			logging.LogFields(ctx, logging.Field("complete_login_error", "completed without complete credentials"))
			httpx.WriteError(w, http.StatusInternalServerError, "completing the sign-in failed")
			return
		}
		logging.LogFields(ctx, logging.Field("user_uuid", result.Credentials.User.UUID))
		writeCredentials(w, http.StatusOK, result.Credentials)
	case identitypkg.LoginCodeInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeLoginCodeInvalid, "the sign-in code is not valid; start the sign-in again")
	default:
		logging.LogFields(ctx, logging.Field("complete_login_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "completing the sign-in failed")
	}
}

// handleLocalAuthorize serves the local provider's sign-in page, only where
// the local provider is enabled.
func (h *Handler) handleLocalAuthorize(w http.ResponseWriter, r *http.Request) {
	page, ok := h.logins.LocalPages.LocalAuthorizePage(r.URL.Query().Get("state"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}
