package costcontrol

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/orchestrator/stepup"
)

type fakeGate struct {
	refuse  bool
	actions []string
}

func (g *fakeGate) Require(_ context.Context, _ string, _ string, action string) error {
	g.actions = append(g.actions, action)
	if g.refuse {
		return fmt.Errorf("%w: unknown", stepup.ErrRefused)
	}
	return nil
}

type fakeBudget struct{ calls int }

func (b *fakeBudget) AdminSetControls(context.Context, string, budget.Controls) (budget.SetControlsOutcome, error) {
	b.calls++
	return budget.ControlsRecorded, nil
}

func (b *fakeBudget) AdminCutoff(context.Context, string, string) (budget.CutoffOutcome, error) {
	b.calls++
	return budget.CutoffApplied, nil
}

func (b *fakeBudget) Reopen(context.Context, string, []budget.Category, string) (budget.ReopenOutcome, error) {
	b.calls++
	return budget.ReopenStarted, nil
}

func TestManager(t *testing.T) {
	ctx := context.Background()
	flows := map[string]func(m *Manager) error{
		ActionSetControls: func(m *Manager) error {
			_, err := m.SetControls(ctx, "admin", "proof", budget.Controls{})
			return err
		},
		ActionCutoff: func(m *Manager) error {
			_, err := m.Cutoff(ctx, "admin", "proof", "reason")
			return err
		},
		ActionReopen: func(m *Manager) error {
			_, err := m.Reopen(ctx, "admin", "proof", nil, "reason")
			return err
		},
	}
	tests := map[string]func(t *testing.T){
		"a refused proof writes nothing": func(t *testing.T) {
			for action, run := range flows {
				gate, b := &fakeGate{refuse: true}, &fakeBudget{}
				err := run(New(gate, b, b))
				require.ErrorIs(t, err, stepup.ErrRefused, action)
				require.Zero(t, b.calls, action)
			}
		},
		"each flow spends the proof on its own action, then writes": func(t *testing.T) {
			for action, run := range flows {
				gate, b := &fakeGate{}, &fakeBudget{}
				require.NoError(t, run(New(gate, b, b)))
				require.Equal(t, []string{action}, gate.actions)
				require.Equal(t, 1, b.calls)
			}
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
