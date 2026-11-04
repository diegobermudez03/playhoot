package oauth

import (
	"encoding/json"

	"io"
	"net/http"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

const (
	maxRegistrationBodyBytes = 16 << 10
	maxTokenBodyBytes        = 16 << 10
)

func encodeJSON(w io.Writer, body any) error {
	return json.NewEncoder(w).Encode(body)
}

// registrationRequest is a client's registration (RFC 7591 §2).
type registrationRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

// registrationResponse is the registered client (RFC 7591 §3.2.1).
type registrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

// handleRegister registers an AI application (dynamic client registration).
func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req registrationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRegistrationBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthErrorBody{Error: "invalid_client_metadata", Description: "the body must be a JSON client registration"})
		return
	}
	client, err := h.server.RegisterClient(ctx, identity.AIClientMetadata{
		ClientName: req.ClientName, ClientURI: req.ClientURI, RedirectURIs: req.RedirectURIs, GrantTypes: req.GrantTypes,
		ResponseTypes: req.ResponseTypes, TokenEndpointAuthMethod: req.TokenEndpointAuthMethod, Scope: req.Scope,
	})
	if err != nil {
		writeOAuthError(ctx, w, err)
		return
	}
	writeJSON(w, http.StatusCreated, registrationResponse{
		ClientID: client.ClientID, ClientIDIssuedAt: client.ClientIDIssuedAt.Unix(), ClientName: client.ClientName, ClientURI: client.ClientURI,
		RedirectURIs: client.RedirectURIs, GrantTypes: client.GrantTypes, ResponseTypes: client.ResponseTypes,
		TokenEndpointAuthMethod: client.TokenEndpointAuthMethod, Scope: client.Scope,
	})
}

// handleAuthorize checks an authorization request and sends the browser to
// the page its User decides on, or back to the application with the error.
// A client or redirect URI that cannot be trusted gets an error page and is
// never redirected to.
func (h *Handler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	started, err := h.server.StartAuthorization(ctx, identity.AIAuthorizationParams{
		ResponseType: query.Get("response_type"), ClientID: query.Get("client_id"), RedirectURI: query.Get("redirect_uri"),
		State: query.Get("state"), CodeChallenge: query.Get("code_challenge"), CodeChallengeMethod: query.Get("code_challenge_method"),
		Scope: query.Get("scope"), Resource: query.Get("resource"),
	})
	if err != nil {
		logging.LogError(ctx, err)
		http.Error(w, "The authorization request could not be handled. Try again later.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	switch started.Outcome {
	case identity.AIAuthorizationStarted, identity.AIAuthorizationRedirectError:
		http.Redirect(w, r, started.RedirectURL, http.StatusFound)
	default:
		message := "This application's authorization request is not valid."
		if started.Error != nil {
			message += " " + started.Error.Description + "."
		}
		http.Error(w, message, http.StatusBadRequest)
	}
}

// tokenResponse is a successful token response (RFC 6749 §5.1).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// clientIDOf is the public client's id: in the form, or as the HTTP Basic
// user some clients send it as.
func clientIDOf(r *http.Request) string {
	if id := r.PostForm.Get("client_id"); id != "" {
		return id
	}
	if user, _, ok := r.BasicAuth(); ok {
		return user
	}
	return ""
}

// handleToken exchanges an authorization code or a refresh token.
func (h *Handler) handleToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxTokenBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthErrorBody{Error: "invalid_request", Description: "the body must be form-encoded"})
		return
	}
	form := r.PostForm
	tokens, err := h.server.ExchangeToken(ctx, identity.AITokenParams{
		GrantType: form.Get("grant_type"), ClientID: clientIDOf(r), Code: form.Get("code"), RedirectURI: form.Get("redirect_uri"),
		CodeVerifier: form.Get("code_verifier"), RefreshToken: form.Get("refresh_token"), Resource: form.Get("resource"),
	})
	if err != nil {
		writeOAuthError(ctx, w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: tokens.AccessToken, TokenType: "Bearer", ExpiresIn: tokens.ExpiresIn, RefreshToken: tokens.RefreshToken, Scope: tokens.Scope})
}

// handleRevoke revokes the connection a token belongs to (RFC 7009): it
// always answers 200, also for an unknown token.
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxTokenBodyBytes)
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthErrorBody{Error: "invalid_request", Description: "the body must be form-encoded"})
		return
	}
	if err := h.server.Revoke(ctx, r.PostForm.Get("token"), clientIDOf(r)); err != nil {
		writeOAuthError(ctx, w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}
