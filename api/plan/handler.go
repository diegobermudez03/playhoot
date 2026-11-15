// Package plan is the route group of Plan: admin's free-limit configuration.
package plan

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/plan"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// freeLimits is admin's free-limit configuration.
type freeLimits interface {
	ReadFreeLimits(ctx context.Context) (plan.FreeLimitConfigs, error)
	AdminSetFreeLimits(ctx context.Context, adminUUID string, limits plan.Limits, effectiveAt time.Time, reason string) (plan.SetFreeLimitsOutcome, error)
}

const maxBodyBytes = 4 << 10

// Handler exposes the plan endpoints.
type Handler struct {
	authorizer authorizer
	freeLimits freeLimits
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, freeLimits freeLimits) *Handler {
	return &Handler{authorizer: authorizer, freeLimits: freeLimits}
}

// RESTRoutes returns this group's endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	admin := auth.Require(h.authorizer, identity.OperationManagePlans)
	return []routing.Route{
		{Pattern: "GET /admin/v1/free-limits", Handler: h.handleRead, Authorization: admin},
		{Pattern: "PUT /admin/v1/free-limits", Handler: h.handleSet, Authorization: admin},
	}
}

// WebSocketRoutes returns none: Plan has no live endpoints.
func (h *Handler) WebSocketRoutes() []routing.Route { return nil }

type limitsJSON struct {
	SavedGames         int `json:"saved_games"`
	CreationsPerWindow int `json:"creations_per_window"`
	SessionsPerWindow  int `json:"sessions_per_window"`
	WindowDays         int `json:"window_days"`
	PlayersPerSession  int `json:"players_per_session"`
	SimultaneousRooms  int `json:"simultaneous_rooms"`
	StorageMB          int `json:"storage_mb"`
}

type configJSON struct {
	limitsJSON
	EffectiveAt       string `json:"effective_at"`
	CreatedByUserUUID string `json:"created_by_user_uuid,omitempty"`
	Reason            string `json:"reason"`
	CreatedAt         string `json:"created_at,omitempty"`
}

type readResponse struct {
	Current   configJSON   `json:"current"`
	Scheduled []configJSON `json:"scheduled"`
	History   []configJSON `json:"history"`
}

type setRequest struct {
	limitsJSON
	EffectiveAt string `json:"effective_at"`
	Reason      string `json:"reason"`
}

type outcomeResponse struct {
	Outcome string `json:"outcome"`
}

func (h *Handler) handleRead(w http.ResponseWriter, r *http.Request) {
	configs, err := h.freeLimits.ReadFreeLimits(r.Context())
	if failed(w, r, err) {
		return
	}
	scheduled := make([]configJSON, len(configs.Scheduled))
	for i, config := range configs.Scheduled {
		scheduled[i] = toJSON(config)
	}
	history := make([]configJSON, len(configs.History))
	for i, config := range configs.History {
		history[i] = toJSON(config)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, readResponse{Current: toJSON(configs.Current), Scheduled: scheduled, History: history})
}

func (h *Handler) handleSet(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.Caller(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	var req setRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var effectiveAt time.Time
	if req.EffectiveAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.EffectiveAt)
		if err != nil {
			httpx.WriteCodedError(w, http.StatusBadRequest, string(plan.FreeLimitsInvalid), "effective_at must be RFC 3339")
			return
		}
		effectiveAt = parsed
	}
	outcome, err := h.freeLimits.AdminSetFreeLimits(r.Context(), caller.UUID, plan.Limits{
		SavedGames: req.SavedGames, CreationsPerWindow: req.CreationsPerWindow, SessionsPerWindow: req.SessionsPerWindow,
		WindowDays: req.WindowDays, PlayersPerSession: req.PlayersPerSession, SimultaneousRooms: req.SimultaneousRooms,
		StorageMB: req.StorageMB,
	}, effectiveAt, req.Reason)
	if failed(w, r, err) {
		return
	}
	if outcome == plan.FreeLimitsInvalid {
		httpx.WriteCodedError(w, http.StatusBadRequest, string(outcome), "invalid free limits")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
}

func toJSON(config plan.FreeLimitConfig) configJSON {
	l := config.Limits
	return configJSON{
		limitsJSON: limitsJSON{
			SavedGames: l.SavedGames, CreationsPerWindow: l.CreationsPerWindow, SessionsPerWindow: l.SessionsPerWindow,
			WindowDays: l.WindowDays, PlayersPerSession: l.PlayersPerSession, SimultaneousRooms: l.SimultaneousRooms,
			StorageMB: l.StorageMB,
		},
		EffectiveAt: config.EffectiveAt.UTC().Format(time.RFC3339), CreatedByUserUUID: config.CreatedByUserUUID,
		Reason: config.Reason, CreatedAt: config.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func failed(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	logging.LogError(r.Context(), err)
	httpx.WriteError(w, http.StatusServiceUnavailable, "plan unavailable")
	return true
}
