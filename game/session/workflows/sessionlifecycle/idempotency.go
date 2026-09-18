package sessionlifecycle

import (
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
)

// Session lifecycle idempotency operation/outcome labels
// (`docs/engineering/standards/idempotency.md`).
const (
	operationCreate = "CREATE"
	operationJoin   = "JOIN"
	operationLeave  = "LEAVE"

	outcomeCreated = "CREATED"
	outcomeJoined  = "JOINED"
	outcomeLeft    = "LEFT"

	// Rejection outcomes recorded only for a business rejection discovered
	// *after* a new token's claim already succeeded (WORK-0001's Transaction
	// Ownership: an idempotency completion - including a deterministic
	// rejection outcome - must still commit together with the business
	// error it accompanies), so a same-token retry replays that rejection
	// consistently instead of being re-evaluated against a possibly
	// different current state
	// (`docs/engineering/standards/idempotency.md`'s Completed Outcomes vs.
	// Transient Failures). A rejection discovered *before* any claim is
	// attempted (lobby expiration; Leave's not-in-LOBBY check) never reaches
	// session_requests at all - there is no token-scoped outcome to record.
	outcomeAlreadyJoined = "ALREADY_JOINED"
	outcomeLobbyFull     = "LOBBY_FULL"
	outcomeActorNotFound = "ACTOR_NOT_FOUND"
)

// createRequestPayload is CREATE's meaningful-field idempotency payload (the
// host UserUUID is already implied by the idempotency identity itself).
type createRequestPayload struct {
	GameUUID string `json:"game_uuid"`
}

// joinRequestPayload is JOIN's meaningful-field idempotency payload
// (GAME-ADR-0021).
type joinRequestPayload struct {
	JoinCode    uint   `json:"join_code"`
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
}

// leaveRequestPayload is LEAVE's meaningful-field idempotency payload.
type leaveRequestPayload struct {
	SessionUUID string `json:"session_uuid"`
	UserUUID    string `json:"user_uuid"`
}

func marshalPayload(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshaling idempotency request payload: %s", err)
	}
	return string(b), nil
}

// interpretExistingCreateClaim decides what an already-claimed CREATE
// identity means for the incoming request: replay, conflict, or
// still-in-flight (`docs/engineering/standards/idempotency.md`'s Token
// Semantics). This is Manager policy - the repository only reports the
// existing row.
func interpretExistingCreateClaim(existing *internalrepo.RequestRow, incoming createRequestPayload) (CreatedSession, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return CreatedSession{}, session.ErrIdempotencyInFlight
	}

	var stored createRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return CreatedSession{}, fmt.Errorf("decoding stored create request payload: %s", err)
	}
	if stored != incoming {
		return CreatedSession{}, session.ErrIdempotencyConflict
	}

	if existing.ResponsePayload == nil {
		return CreatedSession{}, fmt.Errorf("completed create idempotency record missing response payload")
	}
	var result CreatedSession
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return CreatedSession{}, fmt.Errorf("decoding stored create response payload: %s", err)
	}
	return result, nil
}

func interpretExistingJoinClaim(existing *internalrepo.RequestRow, incoming joinRequestPayload) (JoinResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return JoinResult{}, session.ErrIdempotencyInFlight
	}

	var stored joinRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return JoinResult{}, fmt.Errorf("decoding stored join request payload: %s", err)
	}
	if stored != incoming {
		return JoinResult{}, session.ErrIdempotencyConflict
	}

	switch existing.Outcome {
	case outcomeAlreadyJoined:
		return JoinResult{}, session.ErrAlreadyJoined
	case outcomeLobbyFull:
		return JoinResult{}, session.ErrLobbyFull
	}

	if existing.ResponsePayload == nil {
		return JoinResult{}, fmt.Errorf("completed join idempotency record missing response payload")
	}
	var result JoinResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return JoinResult{}, fmt.Errorf("decoding stored join response payload: %s", err)
	}
	return result, nil
}

func interpretExistingLeaveClaim(existing *internalrepo.RequestRow, incoming leaveRequestPayload) (LeaveResult, error) {
	if existing.Status != internalrepo.RequestStatusCompleted {
		return LeaveResult{}, session.ErrIdempotencyInFlight
	}

	var stored leaveRequestPayload
	if err := json.Unmarshal([]byte(existing.RequestPayload), &stored); err != nil {
		return LeaveResult{}, fmt.Errorf("decoding stored leave request payload: %s", err)
	}
	if stored != incoming {
		return LeaveResult{}, session.ErrIdempotencyConflict
	}

	if existing.Outcome == outcomeActorNotFound {
		return LeaveResult{}, session.ErrActorNotFound
	}

	if existing.ResponsePayload == nil {
		return LeaveResult{}, fmt.Errorf("completed leave idempotency record missing response payload")
	}
	var result LeaveResult
	if err := json.Unmarshal([]byte(*existing.ResponsePayload), &result); err != nil {
		return LeaveResult{}, fmt.Errorf("decoding stored leave response payload: %s", err)
	}
	return result, nil
}
