package program

// ChildWorkflowSlotDeclaration declares a statically named, durable
// child-workflow location owned by each instance of the enclosing
// workflow, refering to exactly one statically declared workflow type by
// name.
//
// A child slot may hold at most one child workflow instance at a time and
// remains addressable across all of the parent instance's state
// transitions — spawning a child in one state and handling its completion
// in a state reached later through GotoControl requires no redeclaration
// or transfer of the slot. A child slot belongs to exactly one parent
// workflow instance, is never a user-visible workflow handle stored in
// workflow-local state, cannot be transferred to another parent, and
// disappears when the parent workflow terminates.
//
// A child slot is conceptually, at runtime, empty, running, or
// completed-awaiting-join; this package does not model those runtime
// states, as they are future engine implementation details.
//
// This package does not validate child-slot name uniqueness within a
// workflow or that Workflow refers to an existing workflow declaration; it
// preserves duplicate and invalid declarations so the future compiler can
// report them deterministically.
type ChildWorkflowSlotDeclaration struct {
	Name     string
	Workflow string
}
