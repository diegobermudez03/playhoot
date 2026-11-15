// Package queues is the route group of admin's dead letter queues: every
// domain's dead tasks in one list, and retrying or cancelling one of them.
package queues

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// deadTaskReader lists every domain's dead tasks.
type deadTaskReader interface {
	ListDeadTasks(ctx context.Context) (txqueue.DeadTasks, error)
}

// deadTaskManager retries or cancels a dead task.
type deadTaskManager interface {
	RetryDeadTask(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error)
	CancelDeadTask(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error)
}

const codeTaskNotFound = "task_not_found"

type deadTaskResponse struct {
	Domain       string          `json:"domain"`
	TaskUUID     string          `json:"task_uuid"`
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	Attempts     int             `json:"attempts"`
	LastError    string          `json:"last_error"`
	AdminRetries int             `json:"admin_retries"`
	CreatedAt    time.Time       `json:"created_at"`
	DiedAt       time.Time       `json:"died_at"`
}

type deadTasksResponse struct {
	Tasks []deadTaskResponse `json:"tasks"`
	// Unavailable names the domains whose queue could not be read.
	Unavailable []string `json:"unavailable"`
}

type outcomeResponse struct {
	Outcome string `json:"outcome"`
}

// Handler exposes the dead letter queue endpoints.
type Handler struct {
	authorizer authorizer
	reader     deadTaskReader
	manager    deadTaskManager
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, reader deadTaskReader, manager deadTaskManager) *Handler {
	return &Handler{authorizer: authorizer, reader: reader, manager: manager}
}

// RESTRoutes returns this group's endpoints, all for the administrator.
func (h *Handler) RESTRoutes() []routing.Route {
	admin := auth.Require(h.authorizer, identity.OperationManageQueues)
	return []routing.Route{
		{Pattern: "GET /admin/v1/queues/dead", Handler: h.handleList, Authorization: admin},
		{Pattern: "POST /admin/v1/queues/dead/{taskUUID}/retry", Handler: h.handleRetry, Authorization: admin},
		{Pattern: "POST /admin/v1/queues/dead/{taskUUID}/cancel", Handler: h.handleCancel, Authorization: admin},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dead, err := h.reader.ListDeadTasks(ctx)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "listing dead tasks failed")
		return
	}
	response := deadTasksResponse{Tasks: make([]deadTaskResponse, len(dead.Tasks)), Unavailable: dead.Unavailable}
	if response.Unavailable == nil {
		response.Unavailable = []string{}
	}
	for i, task := range dead.Tasks {
		response.Tasks[i] = deadTaskResponse{
			Domain: task.Domain, TaskUUID: task.UUID, Type: task.Type, Payload: task.Payload, Attempts: task.Attempts,
			LastError: task.LastError, AdminRetries: task.AdminRetries, CreatedAt: task.CreatedAt, DiedAt: task.DiedAt,
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) handleRetry(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, h.manager.RetryDeadTask)
}

func (h *Handler) handleCancel(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, h.manager.CancelDeadTask)
}

// change applies a retry or cancel. A task that is no longer dead answers
// 200 with outcome not_dead: repeating the request changes nothing.
func (h *Handler) change(w http.ResponseWriter, r *http.Request, apply func(ctx context.Context, taskUUID string) (txqueue.DeadTaskOutcome, error)) {
	ctx := r.Context()
	taskUUID := r.PathValue("taskUUID")
	if _, err := uuid.Parse(taskUUID); err != nil {
		httpx.WriteCodedError(w, http.StatusNotFound, codeTaskNotFound, "no such task")
		return
	}
	outcome, err := apply(ctx, taskUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "changing the task failed")
		return
	}
	if outcome == txqueue.DeadTaskNotFound {
		httpx.WriteCodedError(w, http.StatusNotFound, codeTaskNotFound, "no such task")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, outcomeResponse{Outcome: string(outcome)})
}
