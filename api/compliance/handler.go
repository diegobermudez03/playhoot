// Package compliance is the route group of Compliance: a person's
// residence-and-age declaration, the texts people accept and their
// acceptances, and admin's market matrix.
package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// declarations records residence-and-age declarations.
type declarations interface {
	RecordDeclaration(ctx context.Context, declaration compliance.Declaration) (compliance.DeclarationResult, error)
}

// acceptances serves texts and records acceptances.
type acceptances interface {
	GetAcceptanceText(ctx context.Context, kind compliance.TextKind, language string, userUUID string) (compliance.AcceptanceText, error)
	RecordAcceptance(ctx context.Context, request compliance.AcceptanceRequest) (compliance.AcceptanceOutcome, error)
}

// texts is admin's publication of the texts people accept.
type texts interface {
	AdminListTexts(ctx context.Context) ([]compliance.TextCatalogEntry, error)
	AdminPublishText(ctx context.Context, kind compliance.TextKind, language string, content string) (compliance.PublishedText, error)
}

// marketRules is admin's market matrix.
type marketRules interface {
	ReadMarketRules(ctx context.Context, country string) (compliance.MarketRules, error)
	AdminSetMarketRule(ctx context.Context, adminUUID string, rule compliance.MarketRule) (compliance.SetMarketRuleOutcome, error)
}

const (
	maxBodyBytes = 4 << 10
	// maxTextBodyBytes holds a whole acceptance text, which is much larger
	// than any other body this group takes.
	maxTextBodyBytes = 512 << 10
	defaultLanguage  = "en"
)

// Handler exposes the compliance endpoints.
type Handler struct {
	authorizer   authorizer
	declarations declarations
	acceptances  acceptances
	marketRules  marketRules
	texts        texts
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, declarations declarations, acceptances acceptances, marketRules marketRules, texts texts) *Handler {
	return &Handler{authorizer: authorizer, declarations: declarations, acceptances: acceptances, marketRules: marketRules, texts: texts}
}

// RESTRoutes returns this group's endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	admin := auth.Require(h.authorizer, identity.OperationManageMarkets)
	manageTexts := auth.Require(h.authorizer, identity.OperationManageTexts)
	return []routing.Route{
		{Pattern: "POST /v1/compliance/declaration", Handler: h.handleDeclaration, Authorization: auth.Require(h.authorizer, identity.OperationDeclareResidenceAndAge)},
		{Pattern: "POST /v1/compliance/acceptances", Handler: h.handleAcceptance, Authorization: auth.Require(h.authorizer, identity.OperationAcceptTexts)},
		{Pattern: "GET /v1/compliance/texts/{kind}", Handler: h.handleText, Authorization: auth.Optional(h.authorizer, identity.OperationAcceptTexts)},
		{Pattern: "GET /admin/v1/markets", Handler: h.handleReadMarkets, Authorization: admin},
		{Pattern: "PUT /admin/v1/markets", Handler: h.handleSetMarket, Authorization: admin},
		{Pattern: "GET /admin/v1/texts", Handler: h.handleListTexts, Authorization: manageTexts},
		{Pattern: "PUT /admin/v1/texts", Handler: h.handlePublishText, Authorization: manageTexts},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

type declarationRequest struct {
	Country     string `json:"country"`
	Subdivision string `json:"subdivision"`
	MeetsAge    *bool  `json:"meets_age"`
}

type declarationResponse struct {
	Outcome      string `json:"outcome"`
	ThresholdAge int    `json:"threshold_age"`
}

func (h *Handler) handleDeclaration(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req declarationRequest
	if !decode(w, r, &req) {
		return
	}
	if req.MeetsAge == nil {
		httpx.WriteCodedError(w, http.StatusBadRequest, string(compliance.DeclarationInvalid), "meets_age is required")
		return
	}
	result, err := h.declarations.RecordDeclaration(r.Context(), compliance.Declaration{
		UserUUID: caller.UUID, Country: strings.ToUpper(strings.TrimSpace(req.Country)), Subdivision: strings.ToUpper(strings.TrimSpace(req.Subdivision)),
		MeetsAge: *req.MeetsAge, Origin: compliance.DeclaredByUser,
	})
	if failed(w, r, err) {
		return
	}
	switch result.Outcome {
	case compliance.DeclarationConflict:
		httpx.WriteCodedError(w, http.StatusConflict, string(result.Outcome), "a different declaration is already recorded; contact support to correct it")
	case compliance.DeclarationInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(result.Outcome), "the country, or the state or province it needs, is not valid")
	default:
		httpx.WriteJSON(w, http.StatusOK, declarationResponse{Outcome: string(result.Outcome), ThresholdAge: result.ThresholdAge})
	}
}

type acceptanceRequest struct {
	Kind     string `json:"kind"`
	Version  int    `json:"version"`
	Language string `json:"language"`
	Action   string `json:"action"`
	Context  string `json:"context"`
}

type outcomeResponse struct {
	Outcome string `json:"outcome"`
}

func (h *Handler) handleAcceptance(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req acceptanceRequest
	if !decode(w, r, &req) {
		return
	}
	outcome, err := h.acceptances.RecordAcceptance(r.Context(), compliance.AcceptanceRequest{
		UserUUID: caller.UUID, Kind: compliance.TextKind(req.Kind), Version: req.Version, Language: req.Language,
		Action: compliance.AcceptanceAction(req.Action), Context: req.Context,
	})
	if failed(w, r, err) {
		return
	}
	switch outcome {
	case compliance.AcceptanceUnknownText:
		httpx.WriteCodedError(w, http.StatusNotFound, string(outcome), "no such text version")
	case compliance.AcceptanceNotWithdrawable:
		httpx.WriteCodedError(w, http.StatusConflict, string(outcome), "this acceptance is necessary and cannot be withdrawn here")
	case compliance.AcceptanceInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(outcome), "invalid acceptance")
	default:
		httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
	}
}

type textResponse struct {
	Kind        string `json:"kind"`
	Version     int    `json:"version"`
	Language    string `json:"language"`
	Content     string `json:"content"`
	PublishedAt string `json:"published_at"`
	Accepted    bool   `json:"accepted"`
}

func (h *Handler) handleText(w http.ResponseWriter, r *http.Request) {
	userUUID := ""
	if caller, ok := auth.Caller(r.Context()); ok {
		userUUID = caller.UUID
	}
	text, err := h.acceptances.GetAcceptanceText(r.Context(), compliance.TextKind(r.PathValue("kind")), language(r), userUUID)
	if errors.Is(err, compliance.ErrTextNotFound) {
		httpx.WriteCodedError(w, http.StatusNotFound, "text_not_found", "no such text")
		return
	}
	if failed(w, r, err) {
		return
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+string(text.Kind)+`-v`+strconv.Itoa(text.Version)+`-`+text.Language+`.txt"`)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(text.Content))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, textResponse{
		Kind: string(text.Kind), Version: text.Version, Language: text.Language, Content: text.Content,
		PublishedAt: text.PublishedAt.UTC().Format(time.RFC3339), Accepted: text.Accepted,
	})
}

// language is the language query parameter, else the first supported one
// the browser prefers, else English.
func language(r *http.Request) string {
	if requested := r.URL.Query().Get("language"); requested != "" {
		return requested
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		primary, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if slices.Contains(compliance.Languages, primary) {
			return primary
		}
	}
	return defaultLanguage
}

type marketRuleJSON struct {
	Country           string `json:"country"`
	Subdivision       string `json:"subdivision"`
	Function          string `json:"function"`
	Enabled           bool   `json:"enabled"`
	MinAge            int    `json:"min_age"`
	ConditionsNote    string `json:"conditions_note"`
	EffectiveAt       string `json:"effective_at"`
	CreatedByUserUUID string `json:"created_by_user_uuid,omitempty"`
	Reason            string `json:"reason"`
	CreatedAt         string `json:"created_at,omitempty"`
}

type marketsResponse struct {
	Current   []marketRuleJSON `json:"current"`
	Scheduled []marketRuleJSON `json:"scheduled"`
	History   []marketRuleJSON `json:"history"`
}

func (h *Handler) handleReadMarkets(w http.ResponseWriter, r *http.Request) {
	matrix, err := h.marketRules.ReadMarketRules(r.Context(), strings.ToUpper(r.URL.Query().Get("country")))
	if failed(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, marketsResponse{Current: toJSON(matrix.Current), Scheduled: toJSON(matrix.Scheduled), History: toJSON(matrix.History)})
}

func (h *Handler) handleSetMarket(w http.ResponseWriter, r *http.Request) {
	caller, ok := callerOf(w, r)
	if !ok {
		return
	}
	var req marketRuleJSON
	if !decode(w, r, &req) {
		return
	}
	var effectiveAt time.Time
	if req.EffectiveAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.EffectiveAt)
		if err != nil {
			httpx.WriteCodedError(w, http.StatusBadRequest, string(compliance.MarketRuleInvalid), "effective_at must be RFC 3339")
			return
		}
		effectiveAt = parsed
	}
	outcome, err := h.marketRules.AdminSetMarketRule(r.Context(), caller.UUID, compliance.MarketRule{
		Country: strings.ToUpper(req.Country), Subdivision: strings.ToUpper(req.Subdivision), Function: compliance.Function(req.Function),
		Enabled: req.Enabled, MinAge: req.MinAge, ConditionsNote: strings.TrimSpace(req.ConditionsNote), EffectiveAt: effectiveAt, Reason: strings.TrimSpace(req.Reason),
	})
	if failed(w, r, err) {
		return
	}
	switch outcome {
	case compliance.MarketRuleConditionsPending:
		httpx.WriteCodedError(w, http.StatusConflict, string(outcome), "clear the pending conditions note once the conditions are met")
	case compliance.MarketRuleInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(outcome), "invalid market rule")
	default:
		httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
	}
}

func toJSON(rules []compliance.MarketRule) []marketRuleJSON {
	out := make([]marketRuleJSON, len(rules))
	for i, rule := range rules {
		out[i] = marketRuleJSON{
			Country: rule.Country, Subdivision: rule.Subdivision, Function: string(rule.Function), Enabled: rule.Enabled,
			MinAge: rule.MinAge, ConditionsNote: rule.ConditionsNote, EffectiveAt: rule.EffectiveAt.UTC().Format(time.RFC3339),
			CreatedByUserUUID: rule.CreatedByUserUUID, Reason: rule.Reason, CreatedAt: rule.CreatedAt.UTC().Format(time.RFC3339),
		}
	}
	return out
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

func failed(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	logging.LogError(r.Context(), err)
	httpx.WriteError(w, http.StatusServiceUnavailable, "compliance unavailable")
	return true
}
