// Package identity is Identity's public contract. It imports only the standard
// library and grows as Identity's capabilities are accepted.
package identity

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
