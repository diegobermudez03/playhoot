// Package marketgate is the entry gate flows put in front of a function the
// market matrix controls: it asks Compliance whether the person may use it
// where they are, and turns a refusal into a declaration they owe or a
// service restriction.
package marketgate

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/compliance"
)

//go:generate mockgen -package=marketgate -destination=marketgate_mock_test.go . accessChecker

// accessChecker is Compliance's market-and-age decision.
type accessChecker interface {
	CheckMarketAccess(ctx context.Context, request compliance.AccessRequest) (compliance.AccessDecision, error)
}

// DeclarationRequiredError means the person must declare their residence and
// age before Function is available to them.
type DeclarationRequiredError struct {
	Function compliance.Function
}

func (e *DeclarationRequiredError) Error() string {
	return fmt.Sprintf("declaration required for %s", e.Function)
}

// RestrictedError means the function is not offered to the person where they
// are. Reason is worded as a service restriction.
type RestrictedError struct {
	Function compliance.Function
	Reason   compliance.BlockReason
}

func (e *RestrictedError) Error() string {
	return fmt.Sprintf("%s restricted: %s", e.Function, e.Reason)
}

// Gate checks one function before a flow performs it.
type Gate struct {
	checker accessChecker
}

// New constructs a Gate on Compliance's check.
func New(checker accessChecker) *Gate {
	return &Gate{checker: checker}
}

// Require returns nil when userUUID may use function where they are.
// approximateCountry is what the caller's network address suggests, empty
// when unknown; the place Compliance uses is the declared residence first.
func (g *Gate) Require(ctx context.Context, userUUID string, function compliance.Function, approximateCountry string) error {
	decision, err := g.checker.CheckMarketAccess(ctx, compliance.AccessRequest{
		UserUUID: userUUID, Function: function, ApproximateCountry: approximateCountry,
	})
	if err != nil {
		return fmt.Errorf("checking market access: %s", err)
	}
	switch decision.Outcome {
	case compliance.AccessAllowed:
		return nil
	case compliance.AccessNeedsDeclaration:
		return &DeclarationRequiredError{Function: function}
	case compliance.AccessBlocked:
		return &RestrictedError{Function: function, Reason: decision.Reason}
	default:
		return fmt.Errorf("unknown market access outcome %q", decision.Outcome)
	}
}
