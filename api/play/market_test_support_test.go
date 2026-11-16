package play

import (
	"context"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/orchestrator/marketgate"
)

// marketStub answers every market check with one decision, so a test says
// what the gate finds without a Compliance database.
type marketStub struct {
	decision compliance.AccessDecision
	err      error
}

func (s *marketStub) CheckMarketAccess(context.Context, compliance.AccessRequest) (compliance.AccessDecision, error) {
	return s.decision, s.err
}

// allowingMarket is a gate that admits every interactive entry.
func allowingMarket() *marketgate.Gate {
	return marketgate.New(&marketStub{decision: compliance.AccessDecision{Outcome: compliance.AccessAllowed}})
}
