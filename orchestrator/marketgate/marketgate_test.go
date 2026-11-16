package marketgate

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
)

type checkerStub struct {
	decision compliance.AccessDecision
	err      error
	asked    compliance.AccessRequest
}

func (s *checkerStub) CheckMarketAccess(_ context.Context, request compliance.AccessRequest) (compliance.AccessDecision, error) {
	s.asked = request
	return s.decision, s.err
}

func TestRequire(t *testing.T) {
	ctx := context.Background()
	const person = "0f8fad5b-d9cb-469f-a165-70867728950e"

	t.Run("an allowed function passes and carries what was asked", func(t *testing.T) {
		stub := &checkerStub{decision: compliance.AccessDecision{Outcome: compliance.AccessAllowed}}

		require.NoError(t, New(stub).Require(ctx, person, compliance.FunctionCreateGames, "CO"))
		require.Equal(t, compliance.AccessRequest{
			UserUUID: person, Function: compliance.FunctionCreateGames, ApproximateCountry: "CO",
		}, stub.asked)
	})

	t.Run("a missing declaration names the function to declare for", func(t *testing.T) {
		stub := &checkerStub{decision: compliance.AccessDecision{Outcome: compliance.AccessNeedsDeclaration}}

		err := New(stub).Require(ctx, person, compliance.FunctionCreateGames, "")

		var required *DeclarationRequiredError
		require.ErrorAs(t, err, &required)
		require.Equal(t, compliance.FunctionCreateGames, required.Function)
	})

	t.Run("a blocked market carries its service-restriction reason", func(t *testing.T) {
		stub := &checkerStub{decision: compliance.AccessDecision{Outcome: compliance.AccessBlocked, Reason: compliance.BlockAgeRequirement}}

		err := New(stub).Require(ctx, person, compliance.FunctionInteractiveUse, "")

		var restricted *RestrictedError
		require.ErrorAs(t, err, &restricted)
		require.Equal(t, compliance.BlockAgeRequirement, restricted.Reason)
	})

	t.Run("a check that fails is an error, never a silent pass", func(t *testing.T) {
		stub := &checkerStub{err: errors.New("down")}

		err := New(stub).Require(ctx, person, compliance.FunctionInteractiveUse, "")

		require.Error(t, err)
		var required *DeclarationRequiredError
		require.NotErrorIs(t, err, required)
	})
}
