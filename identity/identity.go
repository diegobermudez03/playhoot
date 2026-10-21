// Package identity is Identity's public contract. It imports only the standard
// library and grows as Identity's capabilities are accepted.
package identity

import "errors"

// Operation names a coarse operation Identity decides whether a caller may
// perform at all. Object-level permissions (for example, whether a caller
// created a specific Session) are not decided by Identity.
type Operation string

const (
	// OperationCreateSession is creating a new Session. It requires a real
	// account, not a guest-only identity.
	OperationCreateSession Operation = "CREATE_SESSION"

	// OperationJoinSessionAsAdmin is opening an admin connection to a live
	// Session. It requires a real account, not a guest-only identity.
	OperationJoinSessionAsAdmin Operation = "JOIN_SESSION_AS_ADMIN"

	// OperationJoinSessionAsPlayer is opening a player connection to a live
	// Session. Guests may perform it.
	OperationJoinSessionAsPlayer Operation = "JOIN_SESSION_AS_PLAYER"
)

// User is who a credential stands for, as Identity resolves it.
type User struct {
	UUID string

	// DisplayName is how other people see the User; Identity decides it.
	DisplayName string
}

// AccessDecision is Identity's answer to whether a caller may perform an
// Operation.
type AccessDecision string

const (
	// AccessAllowed means the token is valid and its User may perform the
	// Operation.
	AccessAllowed AccessDecision = "allowed"

	// AccessUnauthenticated means the token is missing, malformed, expired
	// or otherwise not a valid proof of identity.
	AccessUnauthenticated AccessDecision = "unauthenticated"

	// AccessForbidden means the token is valid but its User may not perform
	// the Operation.
	AccessForbidden AccessDecision = "forbidden"
)

// CreateGuestOutcome is the decision a CreateGuest call reached.
type CreateGuestOutcome string

const (
	// GuestCreated means a guest User exists for the request. A retry with
	// the same idempotency key reports this same outcome and User.
	GuestCreated CreateGuestOutcome = "created"

	// GuestInvalidDisplayName means the display name breaks Identity's rules.
	GuestInvalidDisplayName CreateGuestOutcome = "invalid_display_name"
)

// CreateGuestResult is what a CreateGuest call decided. UserUUID and Token are
// set only when Outcome is GuestCreated.
type CreateGuestResult struct {
	Outcome CreateGuestOutcome

	UserUUID string

	// Token is the guest's bearer credential, which the client keeps and
	// sends like any other token.
	Token string
}

// ErrIdempotencyConflict is returned when an idempotency key is reused for a
// request that differs from the one it was first used with.
var ErrIdempotencyConflict = errors.New("idempotency key reused with a conflicting request")
