// Package sessionlifecycle is the Session lifecycle workflow: one Manager
// exposing Create/Join/Leave (and, from Slice 2, Start) as its steps, per
// `docs/engineering/standards/domain-logic-placement.md`'s Preferred
// Workflow Package Shape. The Manager decides business/lifecycle policy
// (transaction scope, admission, expiration, idempotency-replay meaning);
// its narrow `internal/repo` persistence layer reports facts and performs
// the mutations the Manager requests.
package sessionlifecycle

import "time"

// GameUUID is a public Game identity (Create's input).
type GameUUID string

// UserUUID is an already-authenticated caller identity (GAME-ADR-0005).
// Session lifecycle operations never resolve credentials themselves.
type UserUUID string

// SessionUUID is a Session's public identity.
type SessionUUID string

// JoinCode is a lobby's human-facing numeric join code.
type JoinCode uint

// DisplayName is a Session-scoped display-name snapshot supplied by the
// trusted application/integration layer.
type DisplayName string

// IdempotencyKey is a caller-supplied opaque token scoping one logical
// command within its (UserUUID, operation) stream
// (`docs/engineering/standards/idempotency.md`).
type IdempotencyKey string

// CreatedSession is Create's logical outcome, also the shape persisted as
// the idempotency record's replayable response payload.
type CreatedSession struct {
	SessionUUID    SessionUUID `json:"session_uuid"`
	JoinCode       JoinCode    `json:"join_code"`
	LobbyExpiresAt time.Time   `json:"lobby_expires_at"`
}

// JoinResult is Join's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload.
type JoinResult struct {
	SessionUUID SessionUUID `json:"session_uuid"`
	DisplayName DisplayName `json:"display_name"`
}

// LeaveResult is Leave's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload.
type LeaveResult struct {
	SessionUUID SessionUUID `json:"session_uuid"`
}
