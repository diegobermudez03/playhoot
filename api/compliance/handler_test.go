package compliance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
)

// stubAuthorizer: "admin" is the administrator, "account" an account, "guest"
// a guest, anything else unauthenticated.
type stubAuthorizer struct{}

func (stubAuthorizer) Authorize(_ context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	levels := map[string]identity.Level{"admin": identity.LevelAdmin, "account": identity.LevelAccount, "guest": identity.LevelGuest}
	required := map[identity.Operation]identity.Level{
		identity.OperationManageMarkets: identity.LevelAdmin, identity.OperationDeclareResidenceAndAge: identity.LevelAccount,
		identity.OperationAcceptTexts: identity.LevelGuest, identity.OperationManageTexts: identity.LevelAdmin,
	}
	rank := map[identity.Level]int{identity.LevelGuest: 1, identity.LevelAccount: 2, identity.LevelAdmin: 3}
	level, ok := levels[token]
	if !ok {
		return identity.User{}, identity.AccessUnauthenticated, nil
	}
	user := identity.User{UUID: token + "-uuid", Level: level}
	if rank[level] < rank[required[operation]] {
		return user, identity.AccessForbidden, nil
	}
	return user, identity.AccessAllowed, nil
}

type stubCompliance struct {
	declaration compliance.Declaration
	declared    compliance.DeclarationOutcome
	acceptance  compliance.AcceptanceRequest
	accepted    compliance.AcceptanceOutcome
	textUser    string
	textLang    string
	ruleAdmin   string
	rule        compliance.MarketRule
	ruleOutcome compliance.SetMarketRuleOutcome
	catalog     []compliance.TextCatalogEntry
	published   compliance.PublishedText
	publishKind compliance.TextKind
	publishLang string
	publishText string
}

func (s *stubCompliance) AdminListTexts(context.Context) ([]compliance.TextCatalogEntry, error) {
	return s.catalog, nil
}

func (s *stubCompliance) AdminPublishText(_ context.Context, kind compliance.TextKind, language string, content string) (compliance.PublishedText, error) {
	s.publishKind, s.publishLang, s.publishText = kind, language, content
	return s.published, nil
}

func (s *stubCompliance) RecordDeclaration(_ context.Context, declaration compliance.Declaration) (compliance.DeclarationResult, error) {
	s.declaration = declaration
	return compliance.DeclarationResult{Outcome: s.declared, ThresholdAge: 18}, nil
}

func (s *stubCompliance) GetAcceptanceText(_ context.Context, kind compliance.TextKind, language string, userUUID string) (compliance.AcceptanceText, error) {
	s.textUser, s.textLang = userUUID, language
	if kind != compliance.TextTerms {
		return compliance.AcceptanceText{}, compliance.ErrTextNotFound
	}
	return compliance.AcceptanceText{Kind: kind, Version: 1, Language: language, Content: "terms", PublishedAt: time.Now(), Accepted: userUUID != ""}, nil
}

func (s *stubCompliance) RecordAcceptance(_ context.Context, request compliance.AcceptanceRequest) (compliance.AcceptanceOutcome, error) {
	s.acceptance = request
	return s.accepted, nil
}

func (s *stubCompliance) ReadMarketRules(context.Context, string) (compliance.MarketRules, error) {
	return compliance.MarketRules{Current: []compliance.MarketRule{{Country: "CO", Function: compliance.FunctionTeach, Enabled: true, MinAge: 18}}}, nil
}

func (s *stubCompliance) AdminSetMarketRule(_ context.Context, adminUUID string, rule compliance.MarketRule) (compliance.SetMarketRuleOutcome, error) {
	s.ruleAdmin, s.rule = adminUUID, rule
	return s.ruleOutcome, nil
}

func TestHandler(t *testing.T) {
	serve := func(stub *stubCompliance, method, path, token, body string, headers ...string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		for _, route := range NewHandler(stubAuthorizer{}, stub, stub, stub, stub).RESTRoutes() {
			mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
		}
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		for i := 0; i+1 < len(headers); i += 2 {
			request.Header.Set(headers[i], headers[i+1])
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	tests := map[string]func(t *testing.T){
		"admin endpoints need the administrator": func(t *testing.T) {
			stub := &stubCompliance{}
			require.Equal(t, http.StatusForbidden, serve(stub, http.MethodGet, "/admin/v1/markets", "account", "").Code)
			require.Equal(t, http.StatusUnauthorized, serve(stub, http.MethodPut, "/admin/v1/markets", "", `{}`).Code)
			require.Equal(t, http.StatusOK, serve(stub, http.MethodGet, "/admin/v1/markets?country=co", "admin", "").Code)
		},
		"the texts panel is the administrator's, lists missing pairs and publishes a version": func(t *testing.T) {
			stub := &stubCompliance{
				catalog: []compliance.TextCatalogEntry{
					{Kind: compliance.TextTerms, Language: "es", Versions: []compliance.TextVersion{{Version: 2, PublishedAt: time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)}}},
					{Kind: compliance.TextTerms, Language: "pt", Versions: []compliance.TextVersion{}},
				},
				published: compliance.PublishedText{Outcome: compliance.TextPublished, Version: 3, PublishedAt: time.Date(2026, 11, 16, 13, 0, 0, 0, time.UTC)},
			}
			require.Equal(t, http.StatusForbidden, serve(stub, http.MethodGet, "/admin/v1/texts", "account", "").Code)

			listed := serve(stub, http.MethodGet, "/admin/v1/texts", "admin", "")
			require.Equal(t, http.StatusOK, listed.Code)
			require.JSONEq(t, `{"texts":[
				{"kind":"terms","language":"es","versions":[{"version":2,"published_at":"2026-11-16T12:00:00Z"}]},
				{"kind":"terms","language":"pt","versions":[]}]}`, listed.Body.String())

			published := serve(stub, http.MethodPut, "/admin/v1/texts", "admin", `{"kind":"terms","language":"es","content":"Tercera version"}`)
			require.Equal(t, http.StatusOK, published.Code)
			require.JSONEq(t, `{"kind":"terms","language":"es","version":3,"published_at":"2026-11-16T13:00:00Z"}`, published.Body.String())
			require.Equal(t, compliance.TextTerms, stub.publishKind)
			require.Equal(t, "Tercera version", stub.publishText)

			stub.published = compliance.PublishedText{Outcome: compliance.TextInvalid}
			require.Equal(t, http.StatusBadRequest, serve(stub, http.MethodPut, "/admin/v1/texts", "admin", `{"kind":"terms","language":"es","content":""}`).Code)
		},
		"a declaration needs an account and records the caller's answer": func(t *testing.T) {
			stub := &stubCompliance{declared: compliance.DeclarationRecorded}
			require.Equal(t, http.StatusForbidden, serve(stub, http.MethodPost, "/v1/compliance/declaration", "guest", `{}`).Code)
			response := serve(stub, http.MethodPost, "/v1/compliance/declaration", "account", `{"country":"us","subdivision":"al","meets_age":true}`)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, compliance.Declaration{UserUUID: "account-uuid", Country: "US", Subdivision: "AL", MeetsAge: true, Origin: compliance.DeclaredByUser}, stub.declaration)
			require.Equal(t, http.StatusBadRequest, serve(stub, http.MethodPost, "/v1/compliance/declaration", "account", `{"country":"CO"}`).Code)
			stub.declared = compliance.DeclarationConflict
			response = serve(stub, http.MethodPost, "/v1/compliance/declaration", "account", `{"country":"MX","meets_age":true}`)
			require.Equal(t, http.StatusConflict, response.Code)
			require.Contains(t, response.Body.String(), "conflict")
		},
		"a guest records an acceptance; refusals map to their codes": func(t *testing.T) {
			stub := &stubCompliance{accepted: compliance.AcceptanceRecorded}
			body := `{"kind":"terms","version":1,"language":"es","action":"accepted","context":"registration"}`
			require.Equal(t, http.StatusOK, serve(stub, http.MethodPost, "/v1/compliance/acceptances", "guest", body).Code)
			require.Equal(t, "guest-uuid", stub.acceptance.UserUUID)
			stub.accepted = compliance.AcceptanceNotWithdrawable
			require.Equal(t, http.StatusConflict, serve(stub, http.MethodPost, "/v1/compliance/acceptances", "guest", body).Code)
			stub.accepted = compliance.AcceptanceUnknownText
			require.Equal(t, http.StatusNotFound, serve(stub, http.MethodPost, "/v1/compliance/acceptances", "guest", body).Code)
		},
		"a text is public, in the asked or preferred language, and downloadable": func(t *testing.T) {
			stub := &stubCompliance{}
			response := serve(stub, http.MethodGet, "/v1/compliance/texts/terms", "", "", "Accept-Language", "pt-BR,en;q=0.8")
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "pt", stub.textLang)
			require.Empty(t, stub.textUser)
			serve(stub, http.MethodGet, "/v1/compliance/texts/terms?language=es", "guest", "")
			require.Equal(t, "es", stub.textLang)
			require.Equal(t, "guest-uuid", stub.textUser)
			download := serve(stub, http.MethodGet, "/v1/compliance/texts/terms?download=1", "", "")
			require.Contains(t, download.Header().Get("Content-Disposition"), "attachment")
			require.Equal(t, "terms", download.Body.String())
			require.Equal(t, http.StatusNotFound, serve(stub, http.MethodGet, "/v1/compliance/texts/cookies", "", "").Code)
		},
		"setting a rule passes the admin and maps pending conditions": func(t *testing.T) {
			stub := &stubCompliance{ruleOutcome: compliance.MarketRuleConditionsPending}
			body := `{"country":"ar","function":"teach","enabled":true,"min_age":18,"conditions_note":"AAIP","reason":"open","effective_at":"2030-01-01T00:00:00Z"}`
			require.Equal(t, http.StatusConflict, serve(stub, http.MethodPut, "/admin/v1/markets", "admin", body).Code)
			require.Equal(t, "admin-uuid", stub.ruleAdmin)
			require.Equal(t, "AR", stub.rule.Country)
			require.Equal(t, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), stub.rule.EffectiveAt)
			require.Equal(t, http.StatusBadRequest, serve(stub, http.MethodPut, "/admin/v1/markets", "admin", `{"effective_at":"tomorrow"}`).Code)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
