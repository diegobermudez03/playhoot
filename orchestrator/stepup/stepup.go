// Package stepup is how a sensitive admin flow requires a fresh second
// factor: before writing anything, it spends the caller's single-use proof
// on its own action name, and stops if the proof cannot be spent.
package stepup

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/identity"
)

// proofConsumer spends step-up proofs.
type proofConsumer interface {
	ConsumeStepUpProof(ctx context.Context, userUUID string, proof string, action string) (identity.StepUpOutcome, error)
}

// ErrRefused means the proof is unknown, expired or already spent: the
// administrator must verify a new code. Wrapped errors name the outcome.
var ErrRefused = errors.New("step-up proof refused")

// Gate spends proofs for flows.
type Gate struct {
	proofs proofConsumer
}

// New constructs a Gate over Identity's proofs.
func New(proofs proofConsumer) *Gate {
	return &Gate{proofs: proofs}
}

// Require spends adminUUID's proof on action. It returns nil only when the
// proof was live and is now spent; ErrRefused when it was not.
func (g *Gate) Require(ctx context.Context, adminUUID string, proof string, action string) error {
	outcome, err := g.proofs.ConsumeStepUpProof(ctx, adminUUID, proof, action)
	if err != nil {
		return err
	}
	if outcome != identity.StepUpConsumed {
		return fmt.Errorf("%w: %s", ErrRefused, outcome)
	}
	return nil
}
