package game

// This file is a draft / scratch example, NOT a real implementation.
// It exists purely to sketch how a consumer (a session/application layer)
// is expected to use game/v1/program + game/v1/program/gameservice +
// game/v1/engine/engineservice together, end to end. Nothing here is
// meant to compile-and-run correctly as-is (the encoded JSON is left
// empty on purpose, error handling is hand-wavy, and a real consumer
// would persist things instead of holding them in memory) — it's just
// here so a dev reading it understands the shape of the calls and what
// to do with each returned value.

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
)

// gameSession is a stand-in for whatever a real consumer would build:
// something that owns one running game instance, persists its Snapshot
// between calls, and translates between "stuff that happens in the
// outside world" (a player answers a question, a timer really fires,
// ...) and engine.Signal values fed into engineservice.Step.
//
// A real implementation would NOT hold engine.Program/engine.Snapshot
// directly in memory like this across requests — Program would be
// compiled once and cached/shared, and Snapshot would be loaded from
// persistence (see engineservice.DecodeSnapshot) at the start of every
// call and saved again (engineservice.EncodeSnapshot) after every
// successful Step. Keeping them as plain fields here is just to keep
// this draft simple to read top to bottom.
type gameSession struct {
	program  engine.Program
	snapshot engine.Snapshot

	// pendingQuestions is how WE keep track of "what did we ask, and to
	// whom", so that when an answer comes back from the outside world we
	// know which engine.Signal to build. The engine itself already knows
	// this internally (see Snapshot.Root.QuestionSlots), but it doesn't
	// hand us a nice "waiting on this" map — that's session-layer
	// bookkeeping we own, driven by the OpenQuestionOutput values Step
	// gives us. Keyed here by slot name for simplicity; a real consumer
	// would probably also need the workflow instance Path once there are
	// child workflows involved.
	pendingQuestions map[string]engine.OpenQuestionOutput
}

// newGameSession is the "Definition -> compiled Program -> initial
// Snapshot" pipeline: everything that happens once, when a game instance
// is created.
func newGameSession() (*gameSession, error) {
	// Step 1: get a program.Definition. In real life this bytes slice
	// would come from wherever game definitions are stored/authored (see
	// program/DEFINITION.md for how one gets generated in the first
	// place) — left empty here since this is just a draft.
	var encodedDefinition []byte

	def, err := gameservice.DecodeJSON(encodedDefinition)
	if err != nil {
		// A *gameservice.DecodeError here means the JSON itself is
		// structurally broken (bad kind, missing field, wrong type) —
		// nothing about game *rules* yet, just "this isn't a valid
		// Definition at all".
		return nil, fmt.Errorf("decoding definition: %w", err)
	}

	// Optional but cheap: gameservice.Validate catches obvious
	// language-level mistakes (duplicate names, bad operator/operand
	// types) before we even bother handing this to the engine. Doesn't
	// guarantee the definition compiles - see DEFINITION.md's note on
	// this - just narrows feedback earlier when it does find something.
	if errs := gameservice.Validate(*def); len(errs) > 0 {
		return nil, fmt.Errorf("definition failed validation: %v", errs)
	}

	// Step 2: compile it. This is the real, authoritative check.
	compiledProgram, diags := engineservice.Compile(*def)
	if diags.HasErrors() {
		// diags is a Diagnostics (ordered []Diagnostic), collected, not
		// stop-at-first-problem. Any SeverityError entry means
		// compiledProgram must NOT be used to run a game - treat the
		// whole thing as unusable, log/report every diagnostic, don't
		// try to "run it anyway".
		return nil, fmt.Errorf("compile errors: %v", diags)
	}
	// diags might still be non-empty here even without errors (warnings,
	// info) - those are fine to just log and move on.

	// Step 3: create the initial Snapshot for this one game instance.
	snap, startSignal, err := engineservice.NewSnapshot(compiledProgram, engine.InitializationInput{
		RootParameters: map[string]engine.Value{
			// whatever the root workflow's declared Parameters need, by name.
		},
		// Seed should come from a real entropy source, drawn once, here -
		// never a hardcoded/predictable value in real code. Left as the
		// zero value in this draft.
		Seed: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("initializing snapshot: %w", err)
	}

	session := &gameSession{
		program:          compiledProgram,
		snapshot:         snap,
		pendingQuestions: map[string]engine.OpenQuestionOutput{},
	}

	// startSignal is mandatory - it's what actually gets the root
	// workflow instance to run its first transition (typically a
	// "WorkflowStarted" reaction). Don't discard it - run it through
	// Step exactly like any other signal, using our own generic
	// applyStep helper below, which is also what every other kind of
	// incoming signal will go through.
	if err := session.applyStep(startSignal); err != nil {
		return nil, fmt.Errorf("applying start signal: %w", err)
	}

	return session, nil
}

// applyStep is the one place that actually calls engineservice.Step and
// deals with everything a Commit can contain. Every other method below
// (HandleUserIntent, QuestionAnswered, ...) just builds the right
// engine.Signal for its situation and calls this.
func (s *gameSession) applyStep(signal engine.Signal) error {
	commit, err := engineservice.Step(s.program, s.snapshot, signal, engine.DefaultLimits())
	if err != nil {
		// Two of these are "expected, nothing happened" outcomes, not
		// bugs - see README.md. A real consumer should check for these
		// specifically (errors.Is) before treating something as a real
		// failure to log/alert on.
		//
		//   if errors.Is(err, engineservice.ErrSignalRejected) { ... }
		//   if errors.Is(err, engineservice.ErrInputRejected) { ... }
		//
		// Either way: s.snapshot is guaranteed untouched here, nothing
		// was published - it's safe to just return and let the caller
		// decide what to tell the player (e.g. "that's not valid right
		// now").
		return fmt.Errorf("step rejected/failed: %w", err)
	}

	// Success: commit.Snapshot is the new authoritative state. This is
	// where a real consumer would persist it (engineservice.EncodeSnapshot
	// -> save to storage) instead of just reassigning a field.
	s.snapshot = commit.Snapshot

	// commit.Outputs: declarative things WE need to actually go do.
	// The engine never does any of this itself - see README.md's Outputs
	// table. Type-switch over every variant.
	for _, output := range commit.Outputs {
		switch o := output.(type) {
		case engine.OpenQuestionOutput:
			// Remember this so that when the answer comes back (from a
			// websocket message, an HTTP request, whatever), we know
			// what it's an answer TO. This is exactly the "map between
			// what we send to the user and what we receive" bookkeeping
			// mentioned above.
			s.pendingQuestions[o.Slot] = o
			// ... also actually deliver the question to o.Recipient
			// through whatever real transport this consumer uses.

		case engine.CloseQuestionOutput:
			delete(s.pendingQuestions, o.Slot)

		case engine.ActivatePresentationOutput, engine.UpdatePresentationOutput, engine.RemovePresentationOutput:
			// ... push o.Model / removal to o.Recipient's client.

		case engine.ScheduleTimerOutput:
			// ... this is a REQUEST to schedule a real timer. The engine
			// never does this itself - a consumer needs a real scheduler
			// (a job queue, time.AfterFunc, whatever) that, when it
			// fires, builds a SignalKindTimerExpired signal and runs it
			// through applyStep, the same way QuestionAnswered does
			// below for questions.

		case engine.CancelTimerOutput:
			// ... cancel whatever real timer was scheduled for this slot.

		case engine.EmitEffectOutput:
			// ... purely cosmetic, deliver-or-don't, never affects state.

		case engine.WorkflowCompletedOutput:
			// o.Path empty means the ROOT workflow (the whole game
			// instance) just ended - o.Outcome tells us how (Completed /
			// Failed / Cancelled). This is the one place a session layer
			// finds out the game is over and can start wrapping up
			// (show results, archive the session, etc).
			if len(o.Path) == 0 {
				fmt.Println("game instance ended:", o.Outcome.Kind)
			}
		}
	}

	// commit.InternalSignals: signals the engine itself still needs
	// applied, in SEPARATE Step calls (Step never chains these for us).
	// The most common example is a freshly spawned child workflow's own
	// "WorkflowStarted" signal. Just feed each one back through
	// applyStep, same as anything else.
	for _, internal := range commit.InternalSignals {
		if err := s.applyStep(internal); err != nil {
			return fmt.Errorf("applying internal signal: %w", err)
		}
	}

	// commit.Trace: informational only (logging/debugging/replay
	// verification) - nothing downstream needs to consume it, so nothing
	// to do with it here.

	return nil
}

// HandleUserIntent is called whenever a player submits an action from
// the outside world (a button press, a command, ...) - the ordinary,
// unprompted kind of input (see program.UserIntentDeclaration).
func (s *gameSession) HandleUserIntent(actor engine.UserID, intent string, fields map[string]engine.Value) error {
	return s.applyStep(engine.Signal{
		Kind:   engine.SignalKindIntent,
		Intent: intent,
		Actor:  actor,
		Fields: fields,
		// Path would be set here too, if this intent targets a child
		// workflow instance rather than the root - left as the zero
		// value (root) in this draft.
	})
}

// QuestionAnswered is called whenever a player responds to a question we
// previously opened (see the OpenQuestionOutput handling above).
func (s *gameSession) QuestionAnswered(respondent engine.UserID, slot string, answer engine.Value) error {
	// Here we're supposed to match which question we received the
	// answer for: slot alone is enough to look it up in the bookkeeping
	// we built when we handled OpenQuestionOutput above, but a real
	// implementation should also confirm respondent actually matches
	// pending.Recipient, and probably reject/no-op if there's nothing
	// pending for that slot at all (the player answered something we no
	// longer care about, e.g. after a timeout already closed it).
	pending, ok := s.pendingQuestions[slot]
	if !ok {
		return fmt.Errorf("no pending question for slot %q", slot)
	}
	if pending.Recipient != respondent {
		return fmt.Errorf("slot %q is not awaiting an answer from %q", slot, respondent)
	}

	// Building the actual signal: Step itself re-validates all of this
	// (authorized respondent, response type, any Validation expression)
	// before ever accepting it - see ErrInputRejected in README.md - so
	// this session-layer check is just to fail fast / give a clearer
	// error, not the real authority.
	err := s.applyStep(engine.Signal{
		Kind:       engine.SignalKindQuestionAnswered,
		Slot:       slot,
		Respondent: respondent,
		Answer:     answer,
	})
	if err != nil {
		return err
	}

	// Only clear our own bookkeeping once Step actually accepted it -
	// applyStep's own handling of CloseQuestionOutput above also does
	// this whenever the engine closes the slot on its own, so this is
	// belt-and-suspenders for the case where the slot stays open for a
	// repeatable question (draft-quality reasoning here, not verified
	// against actual multi-answer semantics).
	delete(s.pendingQuestions, slot)
	return nil
}

// TimerExpired would be the analogous method for a real timer actually
// firing (see ScheduleTimerOutput above) - sketched, not filled in, same
// idea as QuestionAnswered: look up whatever we remembered about this
// timer slot when we scheduled it, then build a
// SignalKindTimerExpired signal and applyStep it.
func (s *gameSession) TimerExpired(slot string) error {
	return s.applyStep(engine.Signal{
		Kind: engine.SignalKindTimerExpired,
		Slot: slot,
	})
}
