// Package notifications is the route group of Notifications: a user's
// notifications and email preferences, the unsubscribe link, the email
// providers' webhooks and the administrator's outbound email panel.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// entryLister lists a user's entries.
type entryLister interface {
	ListEntries(ctx context.Context, userUUID string, filter notifications.EntryFilter) (notifications.EntryPage, error)
}

// readMarker marks an entry read.
type readMarker interface {
	MarkRead(ctx context.Context, userUUID, entryUUID string) (notifications.MarkReadOutcome, error)
}

// preferenceKeeper reads and sets preferences.
type preferenceKeeper interface {
	ReadPreferences(ctx context.Context, userUUID string) (notifications.Preferences, error)
	SetPreferences(ctx context.Context, userUUID string, p notifications.Preferences) (notifications.Preferences, error)
}

// suppressor records unsubscribes and provider feedback.
type suppressor interface {
	Unsubscribe(ctx context.Context, token string) (notifications.SuppressionOutcome, error)
	ProviderWebhook(ctx context.Context, provider notifications.Provider, header map[string][]string, body []byte) (int, error)
}

// emailPanel is the administrator's outbound email panel.
type emailPanel interface {
	AdminEmailStatus(ctx context.Context) (notifications.EmailStatus, error)
	AdminSetEmailMode(ctx context.Context, adminUUID string, mode notifications.Mode, reason string) (notifications.SetModeOutcome, error)
}

// Dependencies are the operations the endpoints invoke.
type Dependencies struct {
	Entries     entryLister
	Reads       readMarker
	Preferences preferenceKeeper
	Suppression suppressor
	Panel       emailPanel
}

const (
	maxBodyBytes    = 4 << 10
	maxWebhookBytes = 64 << 10
	unavailable     = "notifications unavailable"
)

// Anonymous routes are limited by client address only.
var (
	unsubscribeLimits = routing.RequestLimits{PerClient: []ratelimit.Limit{{Count: 30, Per: time.Minute}}}
	webhookLimits     = routing.RequestLimits{PerClient: []ratelimit.Limit{{Count: 60, Per: time.Minute}}}
)

// Handler exposes the notification endpoints.
type Handler struct {
	authorizer authorizer
	deps       Dependencies
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, deps Dependencies) *Handler {
	return &Handler{authorizer: authorizer, deps: deps}
}

// RESTRoutes returns this group's endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	reader := auth.Require(h.authorizer, identity.OperationReadNotifications)
	owner := auth.Require(h.authorizer, identity.OperationSetNotificationPreferences)
	admin := auth.Require(h.authorizer, identity.OperationManageEmail)
	return []routing.Route{
		{Pattern: "GET /v1/notifications", Handler: h.handleList, Authorization: reader},
		{Pattern: "POST /v1/notifications/{entryUUID}/read", Handler: h.handleRead, Authorization: reader},
		{Pattern: "GET /v1/notifications/preferences", Handler: h.handleGetPreferences, Authorization: owner},
		{Pattern: "PUT /v1/notifications/preferences", Handler: h.handleSetPreferences, Authorization: owner},
		{Pattern: "GET /v1/email/unsubscribe/{token}", Handler: h.handleUnsubscribe, Limits: &unsubscribeLimits},
		{Pattern: "POST /v1/webhooks/email/{provider}", Handler: h.handleWebhook, Limits: &webhookLimits},
		{Pattern: "GET /admin/v1/email", Handler: h.handleStatus, Authorization: admin},
		{Pattern: "PUT /admin/v1/email/mode", Handler: h.handleSetMode, Authorization: admin},
	}
}

// WebSocketRoutes returns none.
func (h *Handler) WebSocketRoutes() []routing.Route { return nil }

type entryJSON struct {
	EntryUUID     string         `json:"entry_uuid"`
	SourceDomain  string         `json:"source_domain"`
	Type          string         `json:"type"`
	ExternalToken string         `json:"external_token"`
	Skeleton      map[string]any `json:"skeleton"`
	Email         bool           `json:"email"`
	Necessary     bool           `json:"necessary"`
	ReadAt        *string        `json:"read_at"`
	CreatedAt     string         `json:"created_at"`
}

type listResponse struct {
	Unread  int         `json:"unread"`
	Entries []entryJSON `json:"entries"`
}

type preferencesJSON struct {
	AI        bool `json:"ai_email"`
	Capacity  bool `json:"capacity_email"`
	Review    bool `json:"review_email"`
	Marketing bool `json:"marketing_email"`
}

type outcomeResponse struct {
	Outcome string `json:"outcome"`
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	filter := notifications.EntryFilter{SourceDomain: q.Get("domain"), Type: notifications.Type(q.Get("type"))}
	for name, target := range map[string]*time.Time{"from": &filter.From, "to": &filter.To, "before": &filter.Before} {
		if raw := q.Get(name); raw != "" {
			parsed, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, name+" must be RFC 3339")
				return
			}
			*target = parsed
		}
	}
	if raw := q.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 {
			httpx.WriteError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		filter.Limit = limit
	}
	page, err := h.deps.Entries.ListEntries(r.Context(), caller.UUID, filter)
	if failed(w, r, err) {
		return
	}
	resp := listResponse{Unread: page.Unread, Entries: make([]entryJSON, len(page.Entries))}
	for i, e := range page.Entries {
		resp.Entries[i] = entryJSON{
			EntryUUID: e.EntryUUID, SourceDomain: e.SourceDomain, Type: string(e.Type), ExternalToken: e.ExternalToken,
			Skeleton: e.Skeleton, Email: e.Email, Necessary: e.Necessary, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if e.ReadAt != nil {
			readAt := e.ReadAt.UTC().Format(time.RFC3339Nano)
			resp.Entries[i].ReadAt = &readAt
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleRead(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	outcome, err := h.deps.Reads.MarkRead(r.Context(), caller.UUID, r.PathValue("entryUUID"))
	if failed(w, r, err) {
		return
	}
	if outcome == notifications.UnknownEntry {
		httpx.WriteCodedError(w, http.StatusNotFound, string(outcome), "no such notification")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
}

func (h *Handler) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	p, err := h.deps.Preferences.ReadPreferences(r.Context(), caller.UUID)
	if failed(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, toPreferencesJSON(p))
}

func (h *Handler) handleSetPreferences(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req preferencesJSON
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := h.deps.Preferences.SetPreferences(r.Context(), caller.UUID, notifications.Preferences{AI: req.AI, Capacity: req.Capacity, Review: req.Review, Marketing: req.Marketing})
	if failed(w, r, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPreferencesJSON(p))
}

func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	outcome, err := h.deps.Suppression.Unsubscribe(r.Context(), r.PathValue("token"))
	if failed(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if outcome == notifications.SuppressionInvalid {
		httpx.WriteCodedError(w, http.StatusNotFound, string(outcome), "this unsubscribe link is not valid")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: "unsubscribed"})
}

type webhookResponse struct {
	Recorded int `json:"recorded"`
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	provider := notifications.Provider(r.PathValue("provider"))
	if provider != notifications.ProviderResend && provider != notifications.ProviderSES {
		httpx.WriteError(w, http.StatusNotFound, "unknown provider")
		return
	}
	recorded, err := h.deps.Suppression.ProviderWebhook(r.Context(), provider, r.Header, body)
	if errors.Is(err, notifications.ErrWebhookRejected) {
		logging.LogError(r.Context(), err)
		httpx.WriteError(w, http.StatusUnauthorized, "webhook not accepted")
		return
	}
	if failed(w, r, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, webhookResponse{Recorded: recorded})
}

type usageJSON struct {
	Provider     string  `json:"provider"`
	Today        int     `json:"today"`
	Month        int     `json:"month"`
	EstimatedUSD float64 `json:"estimated_usd"`
}

type statusResponse struct {
	Mode       string         `json:"mode"`
	ModeSince  string         `json:"mode_since"`
	ModeReason string         `json:"mode_reason"`
	Usage      []usageJSON    `json:"usage"`
	Deliveries map[string]int `json:"deliveries_30d"`
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.deps.Panel.AdminEmailStatus(r.Context())
	if failed(w, r, err) {
		return
	}
	resp := statusResponse{Mode: string(status.Mode), ModeSince: status.ModeSince.UTC().Format(time.RFC3339), ModeReason: status.ModeReason, Deliveries: map[string]int{}}
	for _, u := range status.Usage {
		resp.Usage = append(resp.Usage, usageJSON{Provider: string(u.Provider), Today: u.Today, Month: u.Month, EstimatedUSD: u.EstimatedUSD})
	}
	for state, count := range status.Deliveries {
		resp.Deliveries[string(state)] = count
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type modeRequest struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
}

func (h *Handler) handleSetMode(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req modeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	outcome, err := h.deps.Panel.AdminSetEmailMode(r.Context(), caller.UUID, notifications.Mode(req.Mode), req.Reason)
	if failed(w, r, err) {
		return
	}
	if outcome == notifications.ModeInvalid {
		httpx.WriteCodedError(w, http.StatusBadRequest, string(outcome), "mode must be automatic, resend-only or ses-only, with a reason")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
}

func toPreferencesJSON(p notifications.Preferences) preferencesJSON {
	return preferencesJSON{AI: p.AI, Capacity: p.Capacity, Review: p.Review, Marketing: p.Marketing}
}

func callerOf(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	caller, ok := auth.Caller(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
	}
	return caller, ok
}

func failed(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	logging.LogError(r.Context(), err)
	httpx.WriteError(w, http.StatusServiceUnavailable, unavailable)
	return true
}
