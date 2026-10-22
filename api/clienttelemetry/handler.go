// Package clienttelemetry is the route group taking the operational telemetry
// players' clients send (load times, reconnections, classified errors) into
// the platform's metrics, so what players experience can be seen server side.
package clienttelemetry

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

const (
	// maxBodyBytes bounds a batch.
	maxBodyBytes = 16 << 10
	// maxEvents bounds the events of a batch; a client sends a batch every
	// few tens of seconds, coalescing counts.
	maxEvents = 100
)

type reportRequest struct {
	Events []json.RawMessage `json:"events"`
}

// Handler exposes the client telemetry endpoint.
type Handler struct {
	authorizer authorizer
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer) *Handler {
	return &Handler{authorizer: authorizer}
}

// RESTRoutes returns this group's endpoint. Any identity may report, guests
// included; the route's request limits bound how often.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern:       "POST /v1/client-telemetry",
			Handler:       h.handleReport,
			Authorization: auth.Require(h.authorizer, identity.OperationReportClientTelemetry),
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

// handleReport records every event of the batch the catalog knows and drops
// the rest, counting them. A body that is not a batch is 400; anything else
// is 204, whatever its events held, so a client never retries telemetry.
func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req reportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Events) > maxEvents {
		httpx.WriteError(w, http.StatusBadRequest, "too many events")
		return
	}
	for _, raw := range req.Events {
		recordEvent(ctx, raw)
	}
	w.WriteHeader(http.StatusNoContent)
}
