package plan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/plan"
)

// stubAuthorizer: "admin" is the administrator, "account" an account, anything
// else unauthenticated.
type stubAuthorizer struct{}

func (stubAuthorizer) Authorize(_ context.Context, token string, _ identity.Operation) (identity.User, identity.AccessDecision, error) {
	switch token {
	case "admin":
		return identity.User{UUID: "admin-uuid", Level: identity.LevelAdmin}, identity.AccessAllowed, nil
	case "account":
		return identity.User{UUID: "account-uuid", Level: identity.LevelAccount}, identity.AccessForbidden, nil
	}
	return identity.User{}, identity.AccessUnauthenticated, nil
}

type stubFreeLimits struct {
	admin       string
	limits      plan.Limits
	effectiveAt time.Time
	reason      string
	outcome     plan.SetFreeLimitsOutcome
}

func (s *stubFreeLimits) ReadFreeLimits(context.Context) (plan.FreeLimitConfigs, error) {
	return plan.FreeLimitConfigs{Current: plan.FreeLimitConfig{Limits: plan.Limits{SavedGames: 3, WindowDays: 30}}}, nil
}

func (s *stubFreeLimits) AdminSetFreeLimits(_ context.Context, adminUUID string, limits plan.Limits, effectiveAt time.Time, reason string) (plan.SetFreeLimitsOutcome, error) {
	s.admin, s.limits, s.effectiveAt, s.reason = adminUUID, limits, effectiveAt, reason
	return s.outcome, nil
}

func TestHandler(t *testing.T) {
	serve := func(stub *stubFreeLimits, method, token, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		for _, route := range NewHandler(stubAuthorizer{}, stub).RESTRoutes() {
			mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
		}
		request := httptest.NewRequest(method, "/admin/v1/free-limits", strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	tests := map[string]func(t *testing.T){
		"both endpoints need the administrator": func(t *testing.T) {
			stub := &stubFreeLimits{}
			require.Equal(t, http.StatusForbidden, serve(stub, http.MethodGet, "account", "").Code)
			require.Equal(t, http.StatusUnauthorized, serve(stub, http.MethodPut, "", `{}`).Code)
			response := serve(stub, http.MethodGet, "admin", "")
			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Body.String(), `"saved_games":3`)
		},
		"setting passes the admin and maps an invalid outcome": func(t *testing.T) {
			stub := &stubFreeLimits{outcome: plan.FreeLimitsRecorded}
			body := `{"saved_games":2,"creations_per_window":1,"sessions_per_window":4,"window_days":30,"players_per_session":8,"simultaneous_rooms":1,"storage_mb":50,"reason":"trim","effective_at":"2030-01-01T00:00:00Z"}`
			require.Equal(t, http.StatusOK, serve(stub, http.MethodPut, "admin", body).Code)
			require.Equal(t, "admin-uuid", stub.admin)
			require.Equal(t, plan.Limits{SavedGames: 2, CreationsPerWindow: 1, SessionsPerWindow: 4, WindowDays: 30, PlayersPerSession: 8, SimultaneousRooms: 1, StorageMB: 50}, stub.limits)
			require.Equal(t, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), stub.effectiveAt)
			require.Equal(t, "trim", stub.reason)

			stub.outcome = plan.FreeLimitsInvalid
			require.Equal(t, http.StatusBadRequest, serve(stub, http.MethodPut, "admin", body).Code)
			require.Equal(t, http.StatusBadRequest, serve(stub, http.MethodPut, "admin", `{"effective_at":"tomorrow"}`).Code)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
