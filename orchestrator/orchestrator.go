// Package orchestrator is the coordination layer for cross-domain write
// workflows (ARCHITECTURE.md -> Cross-Domain Writes). It has no workflows
// yet: each one is added as a method on Orchestrator, declaring its own
// narrow interfaces for the domain capabilities it calls.
package orchestrator

// Orchestrator coordinates cross-domain write workflows. It owns no
// business entity.
type Orchestrator struct{}

// New constructs an Orchestrator.
func New() *Orchestrator {
	return &Orchestrator{}
}
