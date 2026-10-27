package identity

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
)

type listOperationsResponse struct {
	Operations []operationLevelResponse `json:"operations"`
}

type operationLevelResponse struct {
	Operation     string `json:"operation"`
	RequiredLevel string `json:"required_level"`
}

// operationsCacheControl lets clients and caches reuse the catalog for 5
// minutes: it only changes with a deployment.
const operationsCacheControl = "public, max-age=300"

// handleListOperations returns every operation with the minimum
// authentication level it requires. It needs no token: an anonymous client
// reads it to know where to send a person before they choose an operation.
func (h *Handler) handleListOperations(w http.ResponseWriter, r *http.Request) {
	operations := h.operations.ListOperations(r.Context())

	response := listOperationsResponse{Operations: make([]operationLevelResponse, 0, len(operations))}
	for _, operation := range operations {
		response.Operations = append(response.Operations, operationLevelResponse{
			Operation:     string(operation.Operation),
			RequiredLevel: string(operation.RequiredLevel),
		})
	}
	w.Header().Set("Cache-Control", operationsCacheControl)
	httpx.WriteJSON(w, http.StatusOK, response)
}
