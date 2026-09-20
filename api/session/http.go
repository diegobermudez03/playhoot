package session

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play"
)

// handleCreateSession is a request entry point: it starts this request's
// own log (nothing upstream of an HTTP handler in this system has already
// started one) and flushes it exactly once, when the request finishes.
// Everything Create calls downstream - Coordinator, SessionRuntime,
// sessionlifecycle.Manager - logs into this same request's log via the ctx
// this handler passes down, rather than starting one of its own.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := logging.Start(r.Context())
	defer logging.FinishRequestLog(ctx, slog.Default(), "api.session.CreateSession")

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

	created, err := h.coord.Create(ctx, req.GameUUID, req.HostUserUUID, req.IdempotencyKey)
	if err != nil {
		logging.LogError(ctx, err)
		writeDomainError(w, err)
		return
	}
	logging.LogFields(ctx, logging.Field("session_uuid", string(created.SessionUUID)))

	httpx.WriteJSON(w, http.StatusCreated, createSessionResponse{
		SessionUUID:    string(created.SessionUUID),
		JoinCode:       created.JoinCode,
		LobbyExpiresAt: created.LobbyExpiresAt,
	})
}

// writeDomainError maps a sentinel business-outcome error to an HTTP
// status; anything else is an unexpected failure.
func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, play.ErrGameNotFound),
		errors.Is(err, play.ErrJoinCodeInvalid),
		errors.Is(err, play.ErrSessionNotFound),
		errors.Is(err, play.ErrInteractionNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, play.ErrIdempotencyKeyRequired),
		errors.Is(err, play.ErrDefinitionDoesNotCompile):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, play.ErrIdempotencyConflict):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, play.ErrIdempotencyInFlight):
		httpx.WriteError(w, http.StatusTooManyRequests, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
