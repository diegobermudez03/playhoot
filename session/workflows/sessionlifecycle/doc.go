// Package sessionlifecycle is Session Runtime's workflow package. A
// workflow's Manager exposes its operations as steps, decides
// business/lifecycle policy (transaction scope, admission, idempotency
// meaning), and delegates persistence to its own internal/repo, which
// reports facts and performs the mutations the Manager requests.
//
// The previous implementation was removed when Session Runtime was
// redesigned as a real-time runtime; this package keeps the layout
// (Manager plus internal/repo) for the new workflows to follow.
package sessionlifecycle
