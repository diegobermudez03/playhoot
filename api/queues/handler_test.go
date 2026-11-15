package queues

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// stubAuthorizer allows the admin token and forbids every other one, as
// Identity does for an operation that requires the admin level.
type stubAuthorizer struct{}

func (stubAuthorizer) Authorize(_ context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	if token == "admin" && operation == identity.OperationManageQueues {
		return identity.User{UUID: "admin-uuid", Level: identity.LevelAdmin}, identity.AccessAllowed, nil
	}
	return identity.User{UUID: "user-uuid", Level: identity.LevelAccount}, identity.AccessForbidden, nil
}

type stubQueues struct {
	dead    txqueue.DeadTasks
	err     error
	outcome txqueue.DeadTaskOutcome
	changed []string
}

func (s *stubQueues) ListDeadTasks(context.Context) (txqueue.DeadTasks, error) {
	return s.dead, s.err
}

func (s *stubQueues) RetryDeadTask(_ context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error) {
	s.changed = append(s.changed, "retry "+taskUUID)
	return s.outcome, s.err
}

func (s *stubQueues) CancelDeadTask(_ context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error) {
	s.changed = append(s.changed, "cancel "+taskUUID)
	return s.outcome, s.err
}

const taskUUID = "6f1c1a52-3c55-4a3e-9a51-0d4d2b7f1f10"

func TestHandler(t *testing.T) {
	serve := func(queues *stubQueues, method string, path string, token string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		for _, route := range NewHandler(stubAuthorizer{}, queues, queues).RESTRoutes() {
			mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
		}
		request := httptest.NewRequest(method, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	tests := map[string]func(t *testing.T){
		"only the administrator reaches the endpoints": func(t *testing.T) {
			queues := &stubQueues{}
			for _, call := range [][2]string{{http.MethodGet, "/admin/v1/queues/dead"}, {http.MethodPost, "/admin/v1/queues/dead/" + taskUUID + "/retry"}, {http.MethodPost, "/admin/v1/queues/dead/" + taskUUID + "/cancel"}} {
				require.Equal(t, http.StatusForbidden, serve(queues, call[0], call[1], "account").Code, call[1])
				require.Equal(t, http.StatusUnauthorized, serve(queues, call[0], call[1], "").Code, call[1])
			}
			require.Empty(t, queues.changed)
		},
		"the list shows every queue's dead tasks and the unreadable queues": func(t *testing.T) {
			died := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
			queues := &stubQueues{dead: txqueue.DeadTasks{
				Tasks:       []txqueue.DeadTask{{Domain: "identity", UUID: taskUUID, Type: "sweep-resource-grants", Payload: json.RawMessage(`{}`), Attempts: 10, LastError: "db down", CreatedAt: died.Add(-time.Hour), DiedAt: died}},
				Unavailable: []string{"game"},
			}}
			recorder := serve(queues, http.MethodGet, "/admin/v1/queues/dead", "admin")
			require.Equal(t, http.StatusOK, recorder.Code)
			require.JSONEq(t, `{"tasks":[{"domain":"identity","task_uuid":"`+taskUUID+`","type":"sweep-resource-grants","payload":{},"attempts":10,"last_error":"db down","admin_retries":0,"created_at":"2026-11-15T09:00:00Z","died_at":"2026-11-15T10:00:00Z"}],"unavailable":["game"]}`, recorder.Body.String())
		},
		"a failed list is unavailable": func(t *testing.T) {
			recorder := serve(&stubQueues{err: errors.New("down")}, http.MethodGet, "/admin/v1/queues/dead", "admin")
			require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		},
		"retry and cancel answer what they did": func(t *testing.T) {
			queues := &stubQueues{outcome: txqueue.DeadTaskRetried}
			recorder := serve(queues, http.MethodPost, "/admin/v1/queues/dead/"+taskUUID+"/retry", "admin")
			require.Equal(t, http.StatusOK, recorder.Code)
			require.JSONEq(t, `{"outcome":"retried"}`, recorder.Body.String())
			queues.outcome = txqueue.DeadTaskNotDead
			recorder = serve(queues, http.MethodPost, "/admin/v1/queues/dead/"+taskUUID+"/cancel", "admin")
			require.Equal(t, http.StatusOK, recorder.Code)
			require.JSONEq(t, `{"outcome":"not_dead"}`, recorder.Body.String())
			require.Equal(t, []string{"retry " + taskUUID, "cancel " + taskUUID}, queues.changed)
		},
		"an unknown or malformed task is not found": func(t *testing.T) {
			queues := &stubQueues{outcome: txqueue.DeadTaskNotFound}
			require.Equal(t, http.StatusNotFound, serve(queues, http.MethodPost, "/admin/v1/queues/dead/"+taskUUID+"/retry", "admin").Code)
			require.Equal(t, http.StatusNotFound, serve(queues, http.MethodPost, "/admin/v1/queues/dead/not-a-uuid/cancel", "admin").Code)
			require.Equal(t, []string{"retry " + taskUUID}, queues.changed)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
