package identity

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type methodResponse struct {
	MethodID string `json:"method_id"`
	Issuer   string `json:"issuer"`
	Address  string `json:"address"`
	Name     string `json:"name"`
	AddedAt  string `json:"added_at"`
}

type methodsResponse struct {
	Methods []methodResponse `json:"methods"`
}

type linkEmailRequest struct {
	Email    string `json:"email"`
	Language string `json:"language"`
	Code     string `json:"code"`
}

type linkProviderRequest struct {
	Provider      string `json:"provider"`
	CodeChallenge string `json:"code_challenge"`
	Code          string `json:"code"`
	CodeVerifier  string `json:"code_verifier"`
}

const (
	codeMethodInUse  = "method_in_use"
	codeLastMethod   = "last_method"
	codeNoSuchMethod = "no_such_method"
)

// handleListMethods answers the caller's own ways into their account.
func (h *Handler) handleListMethods(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	methods, err := h.methods.Reader.ListMethods(ctx, caller.UUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "reading the sign-in methods failed")
		return
	}
	answer := methodsResponse{Methods: make([]methodResponse, 0, len(methods))}
	for _, method := range methods {
		answer.Methods = append(answer.Methods, methodResponse{
			MethodID: method.MethodID, Issuer: string(method.Issuer), Address: method.Address,
			Name: method.Name, AddedAt: method.AddedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, answer)
}

// handleStartLinkEmail sends a code to the address the caller is adding.
func (h *Handler) handleStartLinkEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, req, ok := callerAndBody[linkEmailRequest](w, r)
	if !ok {
		return
	}
	result, err := h.emailLogins.Linker.StartLinkEmail(ctx, caller.UUID, req.Email, requestLanguage(r, req.Language))
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "asking for the code failed")
		return
	}
	writeStartAcknowledgement(w, result)
}

// handleCompleteLinkEmail verifies the code and adds the address.
func (h *Handler) handleCompleteLinkEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, req, ok := callerAndBody[linkEmailRequest](w, r)
	if !ok {
		return
	}
	result, err := h.emailLogins.Linker.CompleteLinkEmail(ctx, caller.UUID, req.Email, req.Code)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "linking the address failed")
		return
	}
	writeLinkOutcome(w, r, result)
}

// handleStartLinkProvider answers where the caller proves the provider
// identity they are adding is theirs.
func (h *Handler) handleStartLinkProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, req, ok := callerAndBody[linkProviderRequest](w, r)
	if !ok {
		return
	}
	result, err := h.logins.Linker.StartLinkProvider(ctx, caller.UUID, req.Provider, req.CodeChallenge)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "starting the link failed")
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
		logging.LogFields(ctx, logging.Field("start_link_provider_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "starting the link failed")
	}
}

// handleCompleteLinkProvider validates the one-time code the callback
// issued and answers whether the identity became another way in.
func (h *Handler) handleCompleteLinkProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, req, ok := callerAndBody[linkProviderRequest](w, r)
	if !ok {
		return
	}
	result, err := h.logins.Linker.CompleteLinkProvider(ctx, caller.UUID, req.Code, req.CodeVerifier)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "linking the provider failed")
		return
	}
	writeLinkOutcome(w, r, result)
}

// handleRemoveMethod unlinks one method, unless it is the only one left.
func (h *Handler) handleRemoveMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	outcome, err := h.methods.Remover.RemoveMethod(ctx, caller.UUID, r.PathValue("methodID"))
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "removing the sign-in method failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch outcome {
	case identitypkg.MethodRemoved:
		httpx.WriteJSON(w, http.StatusOK, struct{}{})
	case identitypkg.MethodUnknown:
		httpx.WriteCodedError(w, http.StatusNotFound, codeNoSuchMethod, "this account has no such sign-in method")
	case identitypkg.MethodLastOne:
		httpx.WriteCodedError(w, http.StatusConflict, codeLastMethod, "add another way in before removing this one")
	default:
		logging.LogFields(ctx, logging.Field("remove_method_outcome", string(outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "removing the sign-in method failed")
	}
}

// writeLinkOutcome answers what completing a link decided.
func writeLinkOutcome(w http.ResponseWriter, r *http.Request, result identitypkg.LinkMethodResult) {
	w.Header().Set("Cache-Control", "no-store")
	switch result.Outcome {
	case identitypkg.MethodLinked:
		httpx.WriteJSON(w, http.StatusOK, struct{}{})
	case identitypkg.MethodInUse:
		httpx.WriteCodedError(w, http.StatusConflict, codeMethodInUse, "that way in belongs to another account; sign in with it instead")
	case identitypkg.MethodInvalidCode:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalidCode, "that code is not the one sent")
	case identitypkg.MethodAttemptGone:
		httpx.WriteCodedError(w, http.StatusGone, codeAttemptGone, "that attempt is no longer valid; start again")
	default:
		logging.LogFields(r.Context(), logging.Field("link_method_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "linking failed")
	}
}

// callerAndBody resolves the caller and reads the request body; it answers
// the client itself when either is missing.
func callerAndBody[T any](w http.ResponseWriter, r *http.Request) (identitypkg.User, T, bool) {
	var body T
	caller, ok := auth.Caller(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return caller, body, false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return caller, body, false
	}
	return caller, body, true
}
