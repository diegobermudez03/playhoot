package session

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

const (
	maxCreateSessionBodyBytes = 4 << 10
	maxIdempotencyKeyLength   = 128
)

var (
	createResultLabel = metrics.NewLabel("result", "created", "game_unavailable", "idempotency_conflict", "invalid_request", "error")

	createResultCreated             = createResultLabel.Value("created")
	createResultGameUnavailable     = createResultLabel.Value("game_unavailable")
	createResultIdempotencyConflict = createResultLabel.Value("idempotency_conflict")
	createResultInvalidRequest      = createResultLabel.Value("invalid_request")
	createResultError               = createResultLabel.Value("error")

	createSessionTotal = metrics.NewCounter(
		"session_create_requests_total",
		"Authorized create-session requests, by outcome.",
		createResultLabel,
	)
	createSessionDuration = metrics.NewHistogram(
		"session_create_duration_seconds",
		"Time spent handling an authorized create-session request.",
		metrics.LatencyBuckets(),
		createResultLabel,
	)
)

// handleCreateSession assumes r's context already carries a started request
// log (Server's own restObservability wraps every REST route with one) and
// that auth.Require already authorized the caller - it only ever calls
// logging.LogFields/LogError, never Start or FinishRequestLog itself.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	started := time.Now()
	result := createResultError
	defer func() {
		createSessionTotal.Inc(ctx, result)
		createSessionDuration.ObserveDuration(ctx, time.Since(started), result)
	}()

	userUUID, ok := auth.UserUUID(ctx)
	if !ok {
		// Reaching the handler without the middleware's caller is a wiring
		// bug, never an anonymous caller to serve.
		logging.LogFields(ctx, logging.Field("wiring_error", "create session reached without an authorized caller"))
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
		return
	}

	var req createSessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateSessionBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		result = createResultInvalidRequest
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx,
		logging.Field("game_uuid", req.GameUUID),
		logging.Field("idempotency_key", req.IdempotencyKey),
	)

	gameUUID, err := uuid.Parse(req.GameUUID)
	if err != nil {
		result = createResultInvalidRequest
		httpx.WriteError(w, http.StatusBadRequest, "game_uuid must be a valid UUID")
		return
	}
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		result = createResultInvalidRequest
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}

	created, err := h.creator.CreateSession(ctx,
		userUUID,
		sessionpkg.GameUUID(gameUUID.String()),
		sessionpkg.IdempotencyKey(req.IdempotencyKey),
	)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			result = createResultIdempotencyConflict
			httpx.WriteError(w, http.StatusConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
		return
	}

	switch created.Outcome {
	case sessionpkg.CreateSessionCreated:
		result = createResultCreated
		logging.LogFields(ctx, logging.Field("session_uuid", string(created.SessionUUID)))
		httpx.WriteJSON(w, http.StatusCreated, createSessionResponse{
			SessionUUID:    string(created.SessionUUID),
			JoinCode:       string(created.JoinCode),
			LobbyExpiresAt: created.ExpiresAt,
		})
	case sessionpkg.CreateSessionGameUnavailable:
		result = createResultGameUnavailable
		httpx.WriteError(w, http.StatusNotFound, "game not found or not currently playable")
	default:
		logging.LogFields(ctx, logging.Field("create_session_outcome", string(created.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
	}
}
