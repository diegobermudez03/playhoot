// Package identity is Identity's public contract: the types and values a
// caller depends on without importing the implementation behind them. It
// intentionally imports nothing beyond the standard library.
//
// The contract is partial: only what callers already need is declared, and
// more is added as Identity's capabilities are accepted. A user's UUID and a
// credential token are plain strings in Identity's methods; they only get a
// named type if they ever carry behavior of their own.
package identity

// Operation names a coarse operation Identity decides whether a caller may
// perform at all. Object-level permissions (for example, whether a caller
// created a specific Session) are not decided by Identity.
type Operation string

const (
	// OperationCreateSession is creating a new Session. It requires a real
	// account, not a guest-only identity.
	OperationCreateSession Operation = "CREATE_SESSION"
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
