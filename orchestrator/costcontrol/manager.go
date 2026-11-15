// Package costcontrol is the administrator's sensitive budget actions:
// changing caps and rates, the emergency cutoff and the reopening. Each
// first spends a fresh step-up proof on its own action, and writes nothing
// when the proof is refused.
package costcontrol

import (
	"context"

	"github.com/diegobermudez03/playhoot/budget"
)

// Step-up action names, recorded on the spent proof.
const (
	ActionSetControls = "budget.set-controls"
	ActionCutoff      = "budget.cutoff"
	ActionReopen      = "budget.reopen"
)

// gate spends step-up proofs.
type gate interface {
	Require(ctx context.Context, adminUUID string, proof string, action string) error
}

// controlsSetter is Budget's caps and rates change.
type controlsSetter interface {
	AdminSetControls(ctx context.Context, adminUUID string, change budget.Controls) (budget.SetControlsOutcome, error)
}

// ladderHand is Budget's manual cutoff and reopening.
type ladderHand interface {
	AdminCutoff(ctx context.Context, adminUUID string, reason string) (budget.CutoffOutcome, error)
	Reopen(ctx context.Context, adminUUID string, categories []budget.Category, reason string) (budget.ReopenOutcome, error)
}

// Manager runs the three flows.
type Manager struct {
	gate     gate
	controls controlsSetter
	ladder   ladderHand
}

// New constructs a Manager.
func New(gate gate, controls controlsSetter, ladder ladderHand) *Manager {
	return &Manager{gate: gate, controls: controls, ladder: ladder}
}

// SetControls spends proof, then appends the caps and rates (flow
// adjust-cost-controls). A refused proof returns stepup.ErrRefused.
func (m *Manager) SetControls(ctx context.Context, adminUUID string, proof string, change budget.Controls) (budget.SetControlsOutcome, error) {
	if err := m.gate.Require(ctx, adminUUID, proof, ActionSetControls); err != nil {
		return "", err
	}
	return m.controls.AdminSetControls(ctx, adminUUID, change)
}

// Cutoff spends proof, then shuts everything down (flow emergency-cutoff).
func (m *Manager) Cutoff(ctx context.Context, adminUUID string, proof string, reason string) (budget.CutoffOutcome, error) {
	if err := m.gate.Require(ctx, adminUUID, proof, ActionCutoff); err != nil {
		return "", err
	}
	return m.ladder.AdminCutoff(ctx, adminUUID, reason)
}

// Reopen spends proof, then takes one step of the gradual reopening (flow
// reopen-spending).
func (m *Manager) Reopen(ctx context.Context, adminUUID string, proof string, categories []budget.Category, reason string) (budget.ReopenOutcome, error) {
	if err := m.gate.Require(ctx, adminUUID, proof, ActionReopen); err != nil {
		return "", err
	}
	return m.ladder.Reopen(ctx, adminUUID, categories, reason)
}
