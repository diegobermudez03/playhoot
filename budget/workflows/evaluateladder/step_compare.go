package evaluateladder

import (
	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	"github.com/diegobermudez03/playhoot/budget/internal/snapshot"
)

// stepCompare compares the numbers against caps and thresholds and merges
// the result with the current step; false when nothing changes.
func stepCompare(snap snapshot.Snapshot) (budget.LadderState, bool) {
	return controls.Next(snap.Ladder, controls.Evaluate(snap.Signals()))
}
