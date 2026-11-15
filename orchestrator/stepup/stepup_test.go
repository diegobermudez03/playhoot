package stepup

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

type fakeProofs struct {
	outcome identity.StepUpOutcome
	err     error
	spent   []string
}

func (f *fakeProofs) ConsumeStepUpProof(_ context.Context, userUUID string, proof string, action string) (identity.StepUpOutcome, error) {
	f.spent = append(f.spent, userUUID+"/"+proof+"/"+action)
	return f.outcome, f.err
}

func TestRequire(t *testing.T) {
	type test struct {
		proofs      *fakeProofs
		expectedErr error
	}
	tests := map[string]func(t *testing.T) test{
		"a_live_proof_lets_the_flow_go_on": func(t *testing.T) test {
			return test{proofs: &fakeProofs{outcome: identity.StepUpConsumed}}
		},
		"a_spent_proof_stops_it": func(t *testing.T) test {
			return test{proofs: &fakeProofs{outcome: identity.StepUpUsed}, expectedErr: ErrRefused}
		},
		"an_expired_proof_stops_it": func(t *testing.T) test {
			return test{proofs: &fakeProofs{outcome: identity.StepUpExpired}, expectedErr: ErrRefused}
		},
		"an_unknown_proof_stops_it": func(t *testing.T) test {
			return test{proofs: &fakeProofs{outcome: identity.StepUpUnknown}, expectedErr: ErrRefused}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			err := New(tc.proofs).Require(context.Background(), "admin", "proof", "budget.reopen")
			if tc.expectedErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.expectedErr)
			}
			require.Equal(t, []string{"admin/proof/budget.reopen"}, tc.proofs.spent)
		})
	}

	failing := &fakeProofs{err: errors.New("db down")}
	err := New(failing).Require(context.Background(), "admin", "proof", "budget.reopen")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRefused)
}
