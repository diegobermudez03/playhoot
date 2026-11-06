package simulategame

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/diegobermudez03/playhoot/play/internal/business/scenariodoc"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/gamemanifest"
)

// withHolds is scenario with each hold compiled into the steps a player
// would send: its inputs at FromMs and the neutral snapshot (every input at
// its default) at ToMs, after the steps written at the same instants.
func withHolds(rules gamemanifest.Rules, scenario runtime.Scenario) runtime.Scenario {
	if len(scenario.Holds) == 0 {
		return scenario
	}
	neutral := json.RawMessage(rules.Inputs.Defaults())
	steps := slices.Clone(scenario.Steps)
	for _, hold := range scenario.Holds {
		steps = append(steps,
			runtime.ScenarioStep{AtMs: hold.FromMs, Player: hold.Player, Inputs: hold.Inputs},
			runtime.ScenarioStep{AtMs: hold.ToMs, Player: hold.Player, Inputs: neutral},
		)
	}
	scenario.Steps = steps
	return scenario
}

// observationPoints are the instants to observe: those asked for and those
// an expectation is checked at, once each.
func observationPoints(scenario runtime.Scenario) []int64 {
	points := slices.Clone(scenario.ObserveAtMs)
	for _, expectation := range scenario.Expect {
		if expectation.AtMs != nil && !slices.Contains(points, *expectation.AtMs) {
			points = append(points, *expectation.AtMs)
		}
	}
	return points
}

// checkExpectations checks scenario's expectations on what result observed:
// at their instant, or at the end when they name none. An instant after the
// game finished is checked on the final state, which no longer changes; one
// the simulation never reached (script error, budget) fails as not observed.
func checkExpectations(scenario runtime.Scenario, result *runtime.SimulationResult) {
	if len(scenario.Expect) == 0 {
		return
	}
	passed := true
	for i, expectation := range scenario.Expect {
		checked := runtime.ExpectationResult{Index: i, Expectation: expectation}
		observation, final, found := observationAt(result, expectation.AtMs)
		switch {
		case !found:
			checked.NotObserved = true
		default:
			document := observation.State
			if expectation.Of != scenariodoc.OfGlobal {
				document = observation.Views[expectation.Of]
			}
			verdict := scenariodoc.Check(expectation, document)
			checked.Passed, checked.Actual, checked.Problem = verdict.Passed, verdict.Actual, verdict.Problem
			checked.ObservedAtMs, checked.Final = observation.AtMs, final
		}
		passed = passed && checked.Passed
		result.Expectations = append(result.Expectations, checked)
	}
	result.Passed = &passed
}

// observationAt is the observation taken for atMs (nil: the final one), and
// whether it is the final one.
func observationAt(result *runtime.SimulationResult, atMs *int64) (runtime.SimulationObservation, bool, bool) {
	hasFinal := hasState(result.Final.State)
	if atMs == nil {
		return result.Final, true, hasFinal
	}
	for _, observation := range result.Observations {
		if observation.RequestedAtMs == *atMs {
			return observation, false, true
		}
	}
	if result.Finished && hasFinal {
		return result.Final, true, true
	}
	return runtime.SimulationObservation{}, false, false
}

// hasState reports whether state holds a game's state: a stored result reads
// an absent one back as null.
func hasState(state json.RawMessage) bool {
	trimmed := bytes.TrimSpace(state)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}
