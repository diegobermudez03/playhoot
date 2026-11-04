package games

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
)

// The operations below are the games endpoints' logic, without HTTP: each
// takes its typed request and the caller, and answers its typed value or a
// Refusal. An HTTP handler and an MCP tool are both thin translators around
// one, so validation, limits and outcome mapping exist once.

// Refusal is why an operation did not do what it was asked: the HTTP status
// its route answers with, the body ({code?, message} plus, for some
// refusals, their diagnostics or current revision), and, when set, how many
// seconds to wait before retrying.
// The MCP tools answer the same status and body.
type Refusal struct {
	Status     int
	Body       any
	RetryAfter string
}

// Caller is whom an operation acts for and where the changes it makes come
// from (web, api or mcp).
type Caller struct {
	UserUUID string
	Origin   string
}

func refuse(status int, message string) *Refusal {
	return &Refusal{Status: status, Body: httpx.ErrorResponse{Message: message}}
}

func refuseCoded(status int, code string, message string) *Refusal {
	return &Refusal{Status: status, Body: httpx.ErrorResponse{Code: code, Message: message}}
}

// writeRefusal answers an HTTP request with refusal.
func writeRefusal(w http.ResponseWriter, refusal *Refusal) {
	if refusal.RetryAfter != "" {
		w.Header().Set("Retry-After", refusal.RetryAfter)
	}
	httpx.WriteJSON(w, refusal.Status, refusal.Body)
}

// gameOf is a game uuid in its canonical form; one nobody can have is
// refused like an unknown game.
func gameOf(ctx context.Context, raw string) (string, *Refusal) {
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return "", refuse(http.StatusNotFound, "game not found")
	}
	logging.LogFields(ctx, logging.Field("game_uuid", parsed.String()))
	return parsed.String(), nil
}

// idempotencyKeyRefusal refuses a missing or too long idempotency key;
// field names where the request carries it.
func idempotencyKeyRefusal(key string, field string) *Refusal {
	if key == "" || len(key) > maxIdempotencyKeyLength {
		return refuse(http.StatusBadRequest, field+" is required and must be at most 128 characters")
	}
	return nil
}

// revisionRefusal refuses a revision given that is not a positive integer.
func revisionRefusal(revision *int64) *Refusal {
	if revision != nil && *revision < 1 {
		return refuse(http.StatusBadRequest, "revision must be a positive integer")
	}
	return nil
}

// accessRefusal is the refusal of a caller Game Management did not let edit
// the game, or nil when the caller may go on.
func accessRefusal(access game.EditableVersionOutcome) *Refusal {
	switch access {
	case game.EditableVersionFound:
		return nil
	case game.EditableVersionNotEditable:
		return refuseCoded(http.StatusConflict, codeNotEditable, "the game has no version being built")
	default:
		// not_found: unknown, deleted, or another person's game, never told
		// apart.
		return refuse(http.StatusNotFound, "game not found")
	}
}
