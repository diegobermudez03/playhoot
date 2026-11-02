package simulategame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/boundaryplan"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/gamemanifest"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/livelimits"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// Rejection reasons that are not a live limit's name.
const (
	reasonInvalid = "invalid"
	kindCommand   = "command"
	kindInputs    = "inputs"
)

// run is one simulation being played: the game's confirmed state, the
// timeline of admitted operations not yet executed, and the live limits as
// a Session would keep them, all on the virtual clock.
type run struct {
	sandbox   gameSandbox
	contextID string
	rules     gamemanifest.Rules
	steps     []runtime.ScenarioStep
	observeAt []int64
	duration  int64
	tick      int64
	began     time.Time
	now       func() time.Time

	players         []string
	state           json.RawMessage
	executedThrough int64
	pending         []boundaryplan.Operation
	nextOrder       uint64

	inputs              map[string]inputs.Snapshot
	transitions         map[string]*ratelimit.Limiter
	participantCommands map[string]*ratelimit.Limiter
	sessionCommands     *ratelimit.Limiter
	boundaryRate        *ratelimit.Limiter
	pendingCommands     map[string]int
	queued              map[string]int

	output int
	result runtime.SimulationResult
}

func newRun(host gameSandbox, contextID string, rules gamemanifest.Rules, scenario runtime.Scenario, now func() time.Time) *run {
	steps := slices.Clone(scenario.Steps)
	// By time, keeping the given order of steps at the same time.
	slices.SortStableFunc(steps, func(a, b runtime.ScenarioStep) int { return compare(a.AtMs, b.AtMs) })
	observeAt := slices.Clone(scenario.ObserveAtMs)
	slices.Sort(observeAt)
	r := &run{
		sandbox:             host,
		contextID:           contextID,
		rules:               rules,
		steps:               steps,
		observeAt:           observeAt,
		duration:            scenario.DurationMs,
		tick:                livelimits.TickInterval.Milliseconds(),
		began:               now(),
		now:                 now,
		players:             participants(scenario.Players),
		inputs:              make(map[string]inputs.Snapshot, scenario.Players),
		transitions:         make(map[string]*ratelimit.Limiter, scenario.Players),
		participantCommands: make(map[string]*ratelimit.Limiter, scenario.Players),
		sessionCommands:     ratelimit.New(livelimits.SessionCommands()...),
		boundaryRate:        ratelimit.New(livelimits.Boundaries()...),
		pendingCommands:     make(map[string]int, scenario.Players),
		queued:              make(map[string]int, scenario.Players),
	}
	for _, player := range r.players {
		r.inputs[player] = rules.Inputs.Defaults()
		r.transitions[player] = ratelimit.New(livelimits.SampleTransitions()...)
		r.participantCommands[player] = ratelimit.New(livelimits.ParticipantCommands()...)
	}
	r.result = runtime.SimulationResult{Outcome: runtime.SimulationRan, Players: r.players, Seed: scenario.Seed, Stopped: runtime.SimulationStoppedDuration}
	return r
}

func compare(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// play starts the game and advances the virtual clock tick by tick: every
// command asks for its own boundary at its arrival, as a live COMMAND does,
// then the tick runs its boundary. It stops at the duration, when the game
// finishes or its script fails, or out of budget.
func (r *run) play(ctx context.Context) error {
	started, err := r.sandbox.Start(ctx, r.contextID, sandbox.StartInput{Participants: r.players, RandomSeed: r.result.Seed})
	if err != nil {
		return fmt.Errorf("starting the game: %w", err)
	}
	if started.Outcome != sandbox.ScriptSucceeded {
		r.scriptFailed(0, started.ScriptError)
		return nil
	}
	r.state = started.State
	if err := r.observeUpTo(ctx, 0); err != nil {
		return r.stopping(err)
	}

	next := 0
	for current := int64(0); current < r.duration; current += r.tick {
		if r.now().Sub(r.began) > WallBudget {
			r.result.Stopped = runtime.SimulationStoppedBudget
			return nil
		}
		tick := current + r.tick
		for ; next < len(r.steps) && r.steps[next].AtMs < tick; next++ {
			step := r.steps[next]
			if !r.admit(step) || step.Command == nil {
				continue
			}
			// A live COMMAND asks for a boundary at once, unless the Session
			// already ran as many as its rate allows: then it waits for the
			// tick.
			if allowed, _ := r.boundaryRate.Allow(virtualTime(step.AtMs)); !allowed {
				continue
			}
			if done, err := r.boundary(ctx, step.AtMs); done || err != nil {
				return r.stopping(err)
			}
		}
		// A tick always runs, and counts against the boundary rate.
		r.boundaryRate.Record(virtualTime(tick))
		if done, err := r.boundary(ctx, tick); done || err != nil {
			return r.stopping(err)
		}
		if err := r.observeUpTo(ctx, tick); err != nil {
			return r.stopping(err)
		}
	}
	return nil
}

// stopping is what play answers when a boundary or an observation ended the
// simulation: nothing for a full output, the error otherwise.
func (r *run) stopping(err error) error {
	if errors.Is(err, errOutputFull) {
		r.result.Stopped = runtime.SimulationStoppedBudget
		return nil
	}
	return err
}

// admit places step in the timeline as a live Session admits a participant's
// SAMPLE or COMMAND, or records why it was refused. It reports whether the
// step was placed.
func (r *run) admit(step runtime.ScenarioStep) bool {
	at := virtualTime(step.AtMs)
	refuse := func(kind, reason, detail string) bool {
		r.result.Rejected = append(r.result.Rejected, runtime.SimulationRejection{AtMs: step.AtMs, Player: step.Player, Kind: kind, Reason: reason, Detail: detail})
		return false
	}
	if step.Command != nil {
		payload, err := r.rules.Commands.Normalize(step.Command.Name, step.Command.Payload)
		if err != nil {
			return refuse(kindCommand, reasonInvalid, err.Error())
		}
		if limit := r.commandRoom(step.Player, at); limit != "" {
			return refuse(kindCommand, limit, "")
		}
		r.pendingCommands[step.Player]++
		r.enqueue(boundaryplan.Operation{ParticipantID: step.Player, SessionTimeMs: step.AtMs, Command: &boundaryplan.Command{Name: step.Command.Name, Payload: payload}})
		return true
	}
	snapshot, err := r.rules.Inputs.Normalize(step.Inputs)
	if err != nil {
		return refuse(kindInputs, reasonInvalid, err.Error())
	}
	if snapshot == r.inputs[step.Player] {
		// A refresh of the same inputs is no transition.
		return false
	}
	limit := r.timelineRoom(step.Player)
	if limit == "" {
		if allowed, _ := r.transitions[step.Player].Allow(at); !allowed {
			limit = livelimits.SampleRate
		}
	}
	if limit != "" {
		return refuse(kindInputs, limit, "")
	}
	r.inputs[step.Player] = snapshot
	r.enqueue(boundaryplan.Operation{ParticipantID: step.Player, SessionTimeMs: step.AtMs, Inputs: []byte(snapshot)})
	return true
}

// commandRoom is the live limit refusing one more command of player, or "".
func (r *run) commandRoom(player string, at time.Time) string {
	if r.pendingCommands[player] >= livelimits.PendingCommands {
		return livelimits.CommandPending
	}
	if limit := r.timelineRoom(player); limit != "" {
		return limit
	}
	if allowed, _ := r.participantCommands[player].Allow(at); !allowed {
		return livelimits.CommandParticipant
	}
	if allowed, _ := r.sessionCommands.Allow(at); !allowed {
		return livelimits.CommandSession
	}
	return ""
}

// timelineRoom is the live limit refusing one more operation of player
// waiting in the timeline, or "".
func (r *run) timelineRoom(player string) string {
	switch {
	case r.queued[player] >= livelimits.PendingPerParticipant:
		return livelimits.TimelineParticipant
	case len(r.pending) >= livelimits.PendingPerSession:
		return livelimits.TimelineSession
	}
	return ""
}

func (r *run) enqueue(operation boundaryplan.Operation) {
	r.nextOrder++
	operation.Order = r.nextOrder
	r.queued[operation.ParticipantID]++
	r.pending = append(r.pending, operation)
}

// boundary runs the boundary at atMs, as boundaryplan plans it, and again at
// once while the operation limit left some waiting and the boundary rate
// allows, as a live Session asks for its next boundary. done means the game
// finished or its script failed.
func (r *run) boundary(ctx context.Context, atMs int64) (done bool, err error) {
	for {
		planned := boundaryplan.Plan(r.pending, atMs, r.executedThrough, r.rules.Update, livelimits.BoundaryOperations)
		if planned.Taken == 0 && !planned.Update {
			return false, nil
		}
		taken := r.pending[:planned.Taken]
		batch, orders := boundaryplan.Batch(taken, planned, r.executedThrough)
		executed, err := r.sandbox.Execute(ctx, r.contextID, batch)
		if err != nil {
			return false, fmt.Errorf("executing a boundary: %w", err)
		}
		if executed.Outcome != sandbox.ScriptSucceeded {
			r.scriptFailed(planned.SessionTimeMs, executed.ScriptError)
			return true, nil
		}
		reachedMs := planned.SessionTimeMs
		if executed.Finished {
			count, finishedMs, ok := boundaryplan.FinishedAt(taken, batch, orders, executed.FinishedOperation)
			if !ok {
				return false, fmt.Errorf("the game finished at operation %d of a batch of %d", executed.FinishedOperation, len(batch.Operations))
			}
			taken, reachedMs = taken[:count], finishedMs
			batch.Operations = batch.Operations[:executed.FinishedOperation+1]
		}
		if err := r.record(batch, executed); err != nil {
			return false, err
		}
		for _, operation := range taken {
			r.queued[operation.ParticipantID]--
			if operation.Command != nil {
				r.pendingCommands[operation.ParticipantID]--
			}
		}
		r.pending = slices.Clone(r.pending[len(taken):])
		r.state = executed.State
		r.executedThrough = reachedMs
		r.result.SimulatedMs = reachedMs
		r.result.Boundaries++
		if executed.Finished {
			r.result.Finished = true
			r.result.FinishedAtMs = &reachedMs
			r.result.Stopped = runtime.SimulationStoppedFinished
			// What the game finished before never happened.
			r.pending = nil
			return true, nil
		}
		// Only a boundary the operation limit split leaves some waiting.
		if len(r.pending) == 0 {
			return false, nil
		}
		if allowed, _ := r.boundaryRate.Allow(virtualTime(atMs)); !allowed {
			return false, nil
		}
	}
}

// record adds what one executed batch answered to the result: each
// command's response and the game's errors and events, at the time of the
// call that returned them.
func (r *run) record(batch sandbox.Batch, executed sandbox.ExecutionResult) error {
	responses := 0
	for _, operation := range batch.Operations {
		if operation.Command == nil || responses >= len(executed.CommandResponses) {
			continue
		}
		command := runtime.SimulatedCommand{AtMs: operation.SessionTimeMs, Player: operation.Command.ParticipantID, Name: operation.Command.Name, Response: executed.CommandResponses[responses]}
		responses++
		if err := r.add(command); err != nil {
			return err
		}
		r.result.Commands = append(r.result.Commands, command)
	}
	for _, returned := range executed.Errors {
		if returned.Operation < 0 || returned.Operation >= len(batch.Operations) {
			continue
		}
		placed := runtime.SimulatedError{AtMs: batch.Operations[returned.Operation].SessionTimeMs, Player: returned.ParticipantID, Error: returned.Error}
		if err := r.add(placed); err != nil {
			return err
		}
		r.result.Errors = append(r.result.Errors, placed)
	}
	for _, emitted := range executed.Events {
		if emitted.Operation < 0 || emitted.Operation >= len(batch.Operations) {
			continue
		}
		placed := runtime.SimulatedEvent{AtMs: batch.Operations[emitted.Operation].SessionTimeMs, To: emitted.Participants, Event: emitted.Event}
		if err := r.add(placed); err != nil {
			return err
		}
		r.result.Events = append(r.result.Events, placed)
	}
	return nil
}

// observeUpTo takes the observations asked for at or before atMs not taken
// yet: the game as it is at atMs.
func (r *run) observeUpTo(ctx context.Context, atMs int64) error {
	for len(r.observeAt) > 0 && r.observeAt[0] <= atMs {
		observation, err := r.observe(ctx, r.observeAt[0], atMs)
		if err != nil {
			return err
		}
		r.observeAt = r.observeAt[1:]
		if err := r.add(observation); err != nil {
			return err
		}
		r.result.Observations = append(r.result.Observations, observation)
	}
	return nil
}

// observeFinal takes the game as the simulation left it. A game whose start
// failed has nothing to observe.
func (r *run) observeFinal(ctx context.Context) error {
	if r.state == nil {
		return nil
	}
	at := r.executedThrough
	if !r.result.Finished && r.result.ScriptError == nil && r.result.Stopped == runtime.SimulationStoppedDuration {
		at = max(at, r.result.SimulatedMs)
	}
	final, err := r.observe(ctx, at, at)
	if err != nil && r.result.Stopped == runtime.SimulationStoppedBudget {
		// The budget may have stopped the context mid-call: the state
		// confirmed until then is still known, the views are not.
		r.result.Final = runtime.SimulationObservation{RequestedAtMs: at, AtMs: at, State: r.state}
		return nil
	}
	if err != nil {
		return err
	}
	r.result.Final = final
	return nil
}

// observe is the game's state and every participant's view at atMs.
func (r *run) observe(ctx context.Context, requestedMs int64, atMs int64) (runtime.SimulationObservation, error) {
	observation := runtime.SimulationObservation{RequestedAtMs: requestedMs, AtMs: atMs, State: r.state, Views: make(map[string]json.RawMessage, len(r.players))}
	if r.result.ScriptError != nil {
		return observation, nil
	}
	for _, player := range r.players {
		projected, err := r.sandbox.Project(ctx, r.contextID, player)
		if err != nil {
			return runtime.SimulationObservation{}, fmt.Errorf("projecting %s: %w", player, err)
		}
		if projected.Outcome != sandbox.ScriptSucceeded {
			r.scriptFailed(atMs, projected.ScriptError)
			return observation, nil
		}
		observation.Views[player] = projected.View
	}
	return observation, nil
}

// scriptFailed records that the game's script failed at atMs; the
// simulation stops there.
func (r *run) scriptFailed(atMs int64, message string) {
	r.result.ScriptError = &runtime.SimulationScriptError{AtMs: atMs, Message: message}
	r.result.Stopped = runtime.SimulationStoppedScriptError
}

// add counts value against MaxOutputBytes.
func (r *run) add(value any) error {
	r.output += encodedSize(value)
	if r.output > MaxOutputBytes {
		return errOutputFull
	}
	return nil
}
