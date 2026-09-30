package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/session"
)

// handleCreateSession assumes r's context already carries a started
// request log (Server's own restObservability wraps every REST route
// with one) - it only ever calls logging.LogFields/LogError, never Start
// or FinishRequestLog itself.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx,
		logging.Field("game_uuid", req.GameUUID),
		logging.Field("host_user_uuid", req.HostUserUUID),
	)

	created, err := h.creator.CreateSession(ctx,
		sessionpkg.GameUUID(req.GameUUID),
		sessionpkg.UserUUID(req.HostUserUUID),
		sessionpkg.IdempotencyKey(req.IdempotencyKey),
	)
	if err != nil {
		writeCreateSessionError(w, ctx, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, createSessionResponse{
		SessionUUID:    string(created.SessionUUID),
		JoinCode:       uint(created.JoinCode),
		LobbyExpiresAt: created.LobbyExpiresAt,
	})
}

// writeCreateSessionError maps CreateSession's sentinel errors to the HTTP
// status a client should branch on; anything else is an unexpected
// failure.
func writeCreateSessionError(w http.ResponseWriter, ctx context.Context, err error) {
	logging.LogError(ctx, err)
	switch {
	case errors.Is(err, sessionpkg.ErrGameNotFound):
		httpx.WriteError(w, http.StatusNotFound, "game not found or not currently playable")
	case errors.Is(err, sessionpkg.ErrIdempotencyKeyRequired):
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required")
	case errors.Is(err, sessionpkg.ErrIdempotencyConflict):
		httpx.WriteError(w, http.StatusConflict, "idempotency key reused with a conflicting request")
	case errors.Is(err, sessionpkg.ErrIdempotencyInFlight):
		httpx.WriteError(w, http.StatusConflict, "idempotency key claim is still in flight")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
	}
}

// handleGetFrontendScript returns short-lived signed access to the frontend
// script of the version the Session is pinned to.
//
// TEMPORARY: the caller is identified by the user_uuid query parameter, the
// same interim approach POST /sessions takes with host_user_uuid. It exists
// only until an identity package and middleware derive the caller from an
// authenticated request; at that point this parameter is removed and the
// caller comes from the request context. Until then anyone can claim any
// user_uuid, so this endpoint must not be exposed beyond trusted callers.
func (h *Handler) handleGetFrontendScript(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionUUID := r.PathValue("session_uuid")
	userUUID := r.URL.Query().Get("user_uuid")
	logging.LogFields(ctx,
		logging.Field("session_uuid", sessionUUID),
		logging.Field("user_uuid", userUUID),
	)
	if userUUID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user_uuid is required")
		return
	}

	result, err := h.content.GetFrontendScriptAccess(ctx, sessionpkg.SessionUUID(sessionUUID), sessionpkg.UserUUID(userUUID))
	writeContentAccess(w, ctx, result, err)
}

// handleGetAsset returns short-lived signed access to one asset declared by
// the version the Session is pinned to, addressed by the game's own logical
// asset key. user_uuid is temporary in the same way as
// handleGetFrontendScript's - see its comment.
func (h *Handler) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionUUID := r.PathValue("session_uuid")
	key := r.PathValue("key")
	userUUID := r.URL.Query().Get("user_uuid")
	logging.LogFields(ctx,
		logging.Field("session_uuid", sessionUUID),
		logging.Field("user_uuid", userUUID),
		logging.Field("asset_key", key),
	)
	if userUUID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user_uuid is required")
		return
	}

	result, err := h.content.GetAssetAccess(ctx, sessionpkg.SessionUUID(sessionUUID), sessionpkg.UserUUID(userUUID), key)
	writeContentAccess(w, ctx, result, err)
}

// writeContentAccess maps a content access result to the HTTP status a client
// should branch on. The signed URL itself is deliberately never logged.
func writeContentAccess(w http.ResponseWriter, ctx context.Context, result sessionpkg.ContentAccessResult, err error) {
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrSessionNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "session not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "granting content access failed")
		return
	}

	switch result.Outcome {
	case sessionpkg.ContentAccessOutcomeGranted:
		httpx.WriteJSON(w, http.StatusOK, contentAccessResponse{
			URL:         result.URL,
			ExpiresAt:   result.ExpiresAt,
			SHA256:      result.SHA256,
			ContentType: result.ContentType,
			Size:        result.Size,
		})
	case sessionpkg.ContentAccessOutcomeNotAParticipant:
		httpx.WriteError(w, http.StatusForbidden, "not a participant of this session")
	case sessionpkg.ContentAccessOutcomeContentNotFound:
		httpx.WriteError(w, http.StatusNotFound, "content not found for this session's game version")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "granting content access failed")
	}
}
