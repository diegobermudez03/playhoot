package engineservice_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

// TestStartTurn_MatchesLiveNewSnapshotAndStep proves StartTurn's Outputs
// for a session's mandatory first turn are exactly what live, in-memory
// NewSnapshot+Step already produce - StartTurn is a genuine encapsulation
// of that same mechanism, not a different one.
func TestStartTurn_MatchesLiveNewSnapshotAndStep(t *testing.T) {
	p, diags := engineservice.Compile(counterProgramDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	input := engine.InitializationInput{}

	liveSnap, startSignal, err := runtime.NewSnapshot(p, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	liveCommit, err := runtime.Step(p, liveSnap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	outputs, err := engineservice.StartTurn(p, input, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(outputs, liveCommit.Outputs) {
		t.Fatalf("StartTurn outputs %+v do not match live outputs %+v", outputs, liveCommit.Outputs)
	}
}

// TestAdvanceTurn_MatchesLiveExecutionAcrossMultipleTurns proves
// AdvanceTurn's internal replay reproduces exactly what live, in-memory
// continued execution (holding the Snapshot across calls) produces, at
// every turn of a multi-turn sequence: a caller supplying only the
// durable signal log gets the same result a caller holding a Snapshot
// would have gotten, without ever holding one itself.
func TestAdvanceTurn_MatchesLiveExecutionAcrossMultipleTurns(t *testing.T) {
	p, diags := engineservice.Compile(counterProgramDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	input := engine.InitializationInput{}

	liveSnap, startSignal, err := runtime.NewSnapshot(p, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	liveCommit, err := runtime.Step(p, liveSnap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	liveSnap = liveCommit.Snapshot

	signals := []engine.Signal{
		{Kind: engine.SignalKindIntent, Intent: "Increment"},
		{Kind: engine.SignalKindIntent, Intent: "Increment"},
		{Kind: engine.SignalKindIntent, Intent: "Increment"},
		{Kind: engine.SignalKindIntent, Intent: "Finish"},
	}

	var priorSignals []engine.Signal
	for i, sig := range signals {
		liveCommit, err = runtime.Step(p, liveSnap, sig, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected live error at signal %d: %v", i, err)
		}
		liveSnap = liveCommit.Snapshot

		outputs, err := engineservice.AdvanceTurn(p, input, priorSignals, sig, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected AdvanceTurn error at signal %d: %v", i, err)
		}
		if !reflect.DeepEqual(outputs, liveCommit.Outputs) {
			t.Fatalf("signal %d: AdvanceTurn outputs %+v do not match live outputs %+v", i, outputs, liveCommit.Outputs)
		}
		priorSignals = append(priorSignals, sig)
	}
}

// TestAdvanceTurn_StepChainExceededSurfacesExecutionError proves the
// internally-chained-Step bound (engine.Limits.MaxStepsPerTurn) is
// enforced inside engineservice itself and surfaces through the same
// ExecutionErrorCode mechanism every other execution failure already
// uses - a caller never counts steps itself. A limit of zero rejects
// even the first internally-chained Step of a Turn, which is enough to
// exercise the bound deterministically without depending on anything
// that produces InternalSignals (nothing in the engine currently does).
func TestAdvanceTurn_StepChainExceededSurfacesExecutionError(t *testing.T) {
	p, diags := engineservice.Compile(counterProgramDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	limits := engine.DefaultLimits()
	limits.MaxStepsPerTurn = 0

	_, err := engineservice.StartTurn(p, engine.InitializationInput{}, limits)
	execErr, ok := err.(*engineservice.ExecutionError)
	if !ok || execErr.Code != engineservice.ExecutionErrorStepChainExceeded {
		t.Fatalf("expected engineservice.ExecutionErrorStepChainExceeded, got %v", err)
	}
}

// TestAdvanceTurn_ReplayDivergenceDistinguishableFromNewSignalRejection
// proves AdvanceTurn's two failure categories stay distinguishable: a
// diverging element of priorSignals (a data-integrity condition - every
// element already succeeded once, so it must always replay identically)
// must never be reported the same way as newSignal itself being an
// ordinary, expected rejection - a caller needs this distinction to
// decide whether to alert or simply decline.
func TestAdvanceTurn_ReplayDivergenceDistinguishableFromNewSignalRejection(t *testing.T) {
	p, diags := engineservice.Compile(counterProgramDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	input := engine.InitializationInput{}
	increment := engine.Signal{Kind: engine.SignalKindIntent, Intent: "Increment"}
	unmatched := engine.Signal{Kind: engine.SignalKindIntent, Intent: "NotARealIntent"}

	// A prior signal that no longer matches anything - simulating durable
	// history that fails to replay identically.
	_, err := engineservice.AdvanceTurn(p, input, []engine.Signal{unmatched}, increment, engine.DefaultLimits())
	if !errors.Is(err, engineservice.ErrReplayDivergence) {
		t.Fatalf("expected ErrReplayDivergence for a diverging prior signal, got %v", err)
	}

	// newSignal itself failing must surface exactly as Step already
	// surfaces it, never wrapped in ErrReplayDivergence.
	_, err = engineservice.AdvanceTurn(p, input, nil, unmatched, engine.DefaultLimits())
	if !errors.Is(err, engineservice.ErrSignalRejected) {
		t.Fatalf("expected ErrSignalRejected for an ordinary new-signal rejection, got %v", err)
	}
	if errors.Is(err, engineservice.ErrReplayDivergence) {
		t.Fatal("an ordinary new-signal rejection must not be reported as ErrReplayDivergence")
	}
}
