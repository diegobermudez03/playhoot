// Package budget is the route group of Budget: the administrator's control
// panel, and the cost-control actions that take a fresh step-up proof.
package budget

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/stepup"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// statusReader is Budget's control panel.
type statusReader interface {
	Status(ctx context.Context) (budget.Status, error)
}

// costControls are the step-up flows.
type costControls interface {
	SetControls(ctx context.Context, adminUUID string, proof string, change budget.Controls) (budget.SetControlsOutcome, error)
	Cutoff(ctx context.Context, adminUUID string, proof string, reason string) (budget.CutoffOutcome, error)
	Reopen(ctx context.Context, adminUUID string, proof string, categories []budget.Category, reason string) (budget.ReopenOutcome, error)
}

const (
	maxBodyBytes       = 16 << 10
	codeStepUpRequired = "step_up_required"
	codeInvalid        = "invalid"
	codeNotShutDown    = string(budget.ReopenNotShutDown)
	codeAlreadyOpen    = string(budget.ReopenAlreadyOpen)
	stepUpRequiredText = "verify a new second-factor code with purpose step_up"
	budgetUnavailable  = "budget unavailable"
)

// Handler exposes the budget endpoints.
type Handler struct {
	authorizer authorizer
	status     statusReader
	controls   costControls
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, status statusReader, controls costControls) *Handler {
	return &Handler{authorizer: authorizer, status: status, controls: controls}
}

// RESTRoutes returns this group's endpoints, all for the administrator.
func (h *Handler) RESTRoutes() []routing.Route {
	admin := auth.Require(h.authorizer, identity.OperationManageBudget)
	return []routing.Route{
		{Pattern: "GET /admin/v1/budget", Handler: h.handleStatus, Authorization: admin},
		{Pattern: "PUT /admin/v1/budget/controls", Handler: h.handleControls, Authorization: admin},
		{Pattern: "POST /admin/v1/budget/cutoff", Handler: h.handleCutoff, Authorization: admin},
		{Pattern: "POST /admin/v1/budget/reopen", Handler: h.handleReopen, Authorization: admin},
	}
}

// WebSocketRoutes returns none: Budget has no live endpoints.
func (h *Handler) WebSocketRoutes() []routing.Route { return nil }

type capJSON struct {
	Scope          string  `json:"scope"`
	Window         string  `json:"window"`
	USDLimit       float64 `json:"usd_limit"`
	MaxConcurrency int     `json:"max_concurrency,omitempty"`
	MaxVolume      int     `json:"max_volume,omitempty"`
}

type rateJSON struct {
	Category   string  `json:"category"`
	Service    string  `json:"service"`
	Unit       string  `json:"unit"`
	USDPerUnit float64 `json:"usd_per_unit"`
}

type usageJSON struct {
	Scope     string   `json:"scope"`
	Window    string   `json:"window"`
	Measured  float64  `json:"measured_usd"`
	Estimated float64  `json:"estimated_usd"`
	Reserved  float64  `json:"reserved_usd"`
	Limit     *float64 `json:"limit_usd"`
}

type ladderJSON struct {
	Level            string   `json:"level"`
	PausedCategories []string `json:"paused_categories"`
	ReopenClosed     []string `json:"reopen_closed"`
	Reason           string   `json:"reason"`
	Actor            string   `json:"actor"`
	Since            string   `json:"since,omitempty"`
}

type statusResponse struct {
	Ladder             ladderJSON  `json:"ladder"`
	Caps               []capJSON   `json:"caps"`
	Rates              []rateJSON  `json:"rates"`
	Usage              []usageJSON `json:"usage"`
	ActiveReservations int         `json:"active_reservations"`
	LastHourUSD        float64     `json:"last_hour_usd"`
	HoursToMonthCap    *float64    `json:"hours_to_month_cap"`
	RateMissing        []string    `json:"rate_missing"`
}

type controlsRequest struct {
	Caps        []capJSON  `json:"caps"`
	Rates       []rateJSON `json:"rates"`
	EffectiveAt string     `json:"effective_at"`
	Reason      string     `json:"reason"`
	StepUpProof string     `json:"step_up_proof"`
}

type cutoffRequest struct {
	Reason      string `json:"reason"`
	StepUpProof string `json:"step_up_proof"`
}

type reopenRequest struct {
	Categories  []string `json:"categories"`
	Reason      string   `json:"reason"`
	StepUpProof string   `json:"step_up_proof"`
}

type outcomeResponse struct {
	Outcome string `json:"outcome"`
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.status.Status(r.Context())
	if failed(w, r, err) {
		return
	}
	response := statusResponse{
		Ladder: ladderJSON{
			Level: string(status.Ladder.Level), PausedCategories: names(status.Ladder.PausedCategories),
			ReopenClosed: names(status.Ladder.ReopenClosed), Reason: status.Ladder.Reason, Actor: status.Ladder.Actor,
		},
		Caps: []capJSON{}, Rates: []rateJSON{}, Usage: []usageJSON{}, ActiveReservations: status.ActiveReservations,
		LastHourUSD: status.LastHourUSD, RateMissing: names(status.RateMissing),
	}
	if !status.Ladder.Since.IsZero() {
		response.Ladder.Since = status.Ladder.Since.UTC().Format(time.RFC3339)
	}
	if status.HoursToMonthCap >= 0 {
		hours := status.HoursToMonthCap
		response.HoursToMonthCap = &hours
	}
	for _, c := range status.Caps {
		response.Caps = append(response.Caps, capJSON{Scope: c.Scope, Window: string(c.Window), USDLimit: c.USDLimit, MaxConcurrency: c.MaxConcurrency, MaxVolume: c.MaxVolume})
	}
	for _, rate := range status.Rates {
		response.Rates = append(response.Rates, rateJSON{Category: string(rate.Category), Service: rate.Service, Unit: rate.Unit, USDPerUnit: rate.USDPerUnit})
	}
	for _, u := range status.Usage {
		row := usageJSON{Scope: u.Scope, Window: string(u.Window), Measured: u.Measured, Estimated: u.Estimated, Reserved: u.Reserved}
		if u.HasCap {
			limit := u.Limit
			row.Limit = &limit
		}
		response.Usage = append(response.Usage, row)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) handleControls(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req controlsRequest
	if !decode(w, r, &req) {
		return
	}
	change := budget.Controls{Reason: req.Reason}
	if req.EffectiveAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.EffectiveAt)
		if err != nil {
			httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "effective_at must be RFC 3339")
			return
		}
		change.EffectiveAt = parsed
	}
	for _, c := range req.Caps {
		change.Caps = append(change.Caps, budget.Cap{Scope: c.Scope, Window: budget.Window(c.Window), USDLimit: c.USDLimit, MaxConcurrency: c.MaxConcurrency, MaxVolume: c.MaxVolume})
	}
	for _, rate := range req.Rates {
		change.Rates = append(change.Rates, budget.Rate{Category: budget.Category(rate.Category), Service: rate.Service, Unit: rate.Unit, USDPerUnit: rate.USDPerUnit})
	}
	if req.Reason == "" || len(change.Caps)+len(change.Rates) == 0 {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "a reason and at least one cap or rate are required")
		return
	}
	outcome, err := h.controls.SetControls(r.Context(), caller.UUID, req.StepUpProof, change)
	if stepUpRefused(w, err) || failed(w, r, err) {
		return
	}
	if outcome == budget.ControlsInvalid {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "invalid caps or rates")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
}

func (h *Handler) handleCutoff(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req cutoffRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Reason == "" {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "a reason is required")
		return
	}
	outcome, err := h.controls.Cutoff(r.Context(), caller.UUID, req.StepUpProof, req.Reason)
	if stepUpRefused(w, err) || failed(w, r, err) {
		return
	}
	if outcome == budget.CutoffInvalid {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "invalid reason")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
}

func (h *Handler) handleReopen(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req reopenRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Reason == "" {
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "a reason is required")
		return
	}
	categories := make([]budget.Category, len(req.Categories))
	for i, c := range req.Categories {
		categories[i] = budget.Category(c)
	}
	outcome, err := h.controls.Reopen(r.Context(), caller.UUID, req.StepUpProof, categories, req.Reason)
	if stepUpRefused(w, err) || failed(w, r, err) {
		return
	}
	switch outcome {
	case budget.ReopenInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, codeInvalid, "leaving shutdown takes no categories; later steps name closed ones")
	case budget.ReopenNotShutDown:
		httpx.WriteCodedError(w, http.StatusConflict, codeNotShutDown, "there is no shutdown or reopening in progress")
	case budget.ReopenAlreadyOpen:
		httpx.WriteCodedError(w, http.StatusConflict, codeAlreadyOpen, "a named category is already open")
	default:
		httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
	}
}

func callerOf(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	caller, ok := auth.Caller(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
	}
	return caller, ok
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(into); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func stepUpRefused(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, stepup.ErrRefused) {
		return false
	}
	httpx.WriteCodedError(w, http.StatusForbidden, codeStepUpRequired, stepUpRequiredText)
	return true
}

func failed(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	logging.LogError(r.Context(), err)
	httpx.WriteError(w, http.StatusServiceUnavailable, budgetUnavailable)
	return true
}

func names(list []budget.Category) []string {
	out := make([]string, len(list))
	for i, c := range list {
		out[i] = string(c)
	}
	return out
}
