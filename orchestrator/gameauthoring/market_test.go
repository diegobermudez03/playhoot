package gameauthoring

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
	asked    compliance.AccessRequest
}

func (s *marketStub) CheckMarketAccess(_ context.Context, request compliance.AccessRequest) (compliance.AccessDecision, error) {
	s.asked = request
	return s.decision, s.err
}

// allowingMarket is a gate that lets every function through.
func allowingMarket() (*marketgate.Gate, *marketStub) {
	stub := &marketStub{decision: compliance.AccessDecision{Outcome: compliance.AccessAllowed}}
	return marketgate.New(stub), stub
}
