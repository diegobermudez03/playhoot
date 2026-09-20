package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/play"
)

// coordinatorAPI is the narrow play.Coordinator contract this package
// depends on - every type it exposes is play-owned/primitive, never a
// `game` type, consistent with api depending only on play's exported API.
type coordinatorAPI interface {
	Create(ctx context.Context, gameUUID, hostUserUUID, idempotencyKey string) (play.CreatedSession, error)
	Join(ctx context.Context, joinCode uint, userUUID, displayName, idempotencyKey string) (play.JoinResult, error)
	Bind(sessionUUID play.SessionUUID, userUUID play.UserUUID, conn play.Conn) (unbind func())
	Start(ctx context.Context, sessionUUID play.SessionUUID, userUUID play.UserUUID, idempotencyKey string) (play.StartOutcome, []play.Event, error)
	AnswerInteraction(ctx context.Context, sessionUUID play.SessionUUID, interactionUUID play.InteractionUUID, userUUID play.UserUUID, answer []byte) (play.AnswerOutcome, []play.Event, error)
	Deliver(sessionUUID play.SessionUUID, events []play.Event)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	created, err := s.coord.Create(r.Context(), req.GameUUID, req.HostUserUUID, req.IdempotencyKey)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, createSessionResponse{
		SessionUUID:    string(created.SessionUUID),
		JoinCode:       created.JoinCode,
		LobbyExpiresAt: created.LobbyExpiresAt,
	})
}

func (s *Server) handleJoinSession(w http.ResponseWriter, r *http.Request) {
	var req joinSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := s.coord.Join(r.Context(), req.JoinCode, req.UserUUID, req.DisplayName, req.IdempotencyKey)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, joinSessionResponse{
		Outcome:     string(result.Outcome),
		SessionUUID: string(result.SessionUUID),
		DisplayName: result.DisplayName,
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
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, play.ErrIdempotencyKeyRequired),
		errors.Is(err, play.ErrDefinitionDoesNotCompile):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, play.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, play.ErrIdempotencyInFlight):
		writeError(w, http.StatusTooManyRequests, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Message: message})
}
