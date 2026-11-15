package budget

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/orchestrator/stepup"
)

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

type stubBudget struct {
	refuse bool
	reopen budget.ReopenOutcome
	proof  string
}

func (s *stubBudget) Status(context.Context) (budget.Status, error) {
	return budget.Status{
		Ladder:          budget.LadderState{Level: budget.LevelNormal, Actor: budget.ActorAutomatic},
		Usage:           []budget.WindowUsage{{Scope: budget.ScopeGlobal, Window: budget.WindowDay, Measured: 1, Limit: 8, HasCap: true}},
		HoursToMonthCap: -1,
	}, nil
}

func (s *stubBudget) gate(proof string) error {
	s.proof = proof
	if s.refuse {
		return fmt.Errorf("%w: unknown", stepup.ErrRefused)
	}
	return nil
}

func (s *stubBudget) SetControls(_ context.Context, _ string, proof string, _ budget.Controls) (budget.SetControlsOutcome, error) {
	return budget.ControlsRecorded, s.gate(proof)
}

func (s *stubBudget) Cutoff(_ context.Context, _ string, proof string, _ string) (budget.CutoffOutcome, error) {
	return budget.CutoffApplied, s.gate(proof)
}

func (s *stubBudget) Reopen(_ context.Context, _ string, proof string, _ []budget.Category, _ string) (budget.ReopenOutcome, error) {
	return s.reopen, s.gate(proof)
}

func TestHandler(t *testing.T) {
	serve := func(stub *stubBudget, method, path, token, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		for _, route := range NewHandler(stubAuthorizer{}, stub, stub).RESTRoutes() {
			mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
		}
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}
	tests := map[string]func(t *testing.T){
		"only the administrator reads the panel": func(t *testing.T) {
			require.Equal(t, http.StatusForbidden, serve(&stubBudget{}, "GET", "/admin/v1/budget", "account", "").Code)
			response := serve(&stubBudget{}, "GET", "/admin/v1/budget", "admin", "")
			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Body.String(), `"level":"normal"`)
			require.Contains(t, response.Body.String(), `"hours_to_month_cap":null`)
		},
		"a refused step-up proof answers step_up_required": func(t *testing.T) {
			stub := &stubBudget{refuse: true}
			response := serve(stub, "POST", "/admin/v1/budget/cutoff", "admin", `{"reason":"spike","step_up_proof":"p1"}`)
			require.Equal(t, http.StatusForbidden, response.Code)
			require.Contains(t, response.Body.String(), codeStepUpRequired)
			require.Equal(t, "p1", stub.proof)
		},
		"controls need a reason and an entry before spending the proof": func(t *testing.T) {
			stub := &stubBudget{}
			require.Equal(t, http.StatusBadRequest, serve(stub, "PUT", "/admin/v1/budget/controls", "admin", `{"step_up_proof":"p"}`).Code)
			require.Empty(t, stub.proof)
			response := serve(stub, "PUT", "/admin/v1/budget/controls", "admin", `{"reason":"r","caps":[{"scope":"global","window":"day","usd_limit":5}],"step_up_proof":"p"}`)
			require.Equal(t, http.StatusOK, response.Code)
		},
		"reopen outside shutdown is a conflict": func(t *testing.T) {
			response := serve(&stubBudget{reopen: budget.ReopenNotShutDown}, "POST", "/admin/v1/budget/reopen", "admin", `{"reason":"r","step_up_proof":"p"}`)
			require.Equal(t, http.StatusConflict, response.Code)
			require.Contains(t, response.Body.String(), "not_shut_down")
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
