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
// something that owns one running game instance and translates between
// "stuff that happens in the outside world" (a player answers a question,
// a timer really fires, ...) and engine.Signal values fed into
// engineservice.AdvanceTurn.
//
// Notice what this does NOT hold: an engine.Snapshot. engineservice owns
// reconstructing whatever internal runtime state it needs, entirely on its
// own, by internally replaying signals every call — a consumer only ever
// supplies the durable input log itself (start + signals below), exactly
// what it would persist instead of any Snapshot. A real implementation
// would not hold that log in memory across requests either — it would
// append each signal to durable storage as it happens
// and load the ordered log back at the start of every call — but keeping
// it as a plain slice here is just to keep this draft simple to read top
// to bottom.
type gameSession struct {
	program engine.Program
	start   engine.InitializationInput
	signals []engine.Signal

	// pendingQuestions is how WE keep track of "what did we ask, and to
	// whom", so that when an answer comes back from the outside world we
	// know which engine.Signal to build. The engine itself already knows
	// this internally, but it doesn't hand us a nice "waiting on this" map
	// — that's session-layer bookkeeping we own, driven by the
	// OpenQuestionOutput values AdvanceTurn gives us. Keyed here by
	// InteractionID, the engine-assigned address a caller answers/
	// correlates against — never Slot, which is purely informational now
	// (see OpenQuestionOutput's own doc comment).
	pendingQuestions map[engine.InteractionID]engine.OpenQuestionOutput
}

// newGameSession is the "Definition -> compiled Program -> first turn"
// pipeline: everything that happens once, when a game instance is
// created.
func newGameSession() (*gameSession, error) {
	// In real life this bytes slice would come from wherever game
	// definitions are stored/authored (see program/DEFINITION.md) —
	// left empty here since this is just a draft.
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
	// language-level mistakes before bothering the engine with them.
	// It doesn't guarantee the definition compiles (see DEFINITION.md),
	// it just narrows feedback earlier when it does find something.
	if errs := gameservice.Validate(*def); len(errs) > 0 {
		return nil, fmt.Errorf("definition failed validation: %v", errs)
	}

	// Compile is the real, authoritative check. Any SeverityError
	// diagnostic means compiledProgram must not be used to run a game;
	// a warning or info diagnostic is fine to log and move on from.
	compiledProgram, diags := engineservice.Compile(*def)
	if diags.HasErrors() {
		return nil, fmt.Errorf("compile errors: %v", diags)
	}

	start := engine.InitializationInput{
		RootParameters: map[string]engine.Value{
			// whatever the root workflow's declared Parameters need, by name.
		},
		// A real Seed must come from a real entropy source, drawn once,
		// here — never a hardcoded/predictable value. Left as the zero
		// value in this draft.
		Seed: 0,
	}

	session := &gameSession{
		program:          compiledProgram,
		start:            start,
		pendingQuestions: map[engine.InteractionID]engine.OpenQuestionOutput{},
	}

	// StartTurn performs the mandatory first turn entirely internally —
	// there is no signal for this consumer to construct or record: the
	// engine's own synthesized first lifecycle signal is never a caller
	// concern. s.signals stays empty until the first turn after this one.
	outputs, err := engineservice.StartTurn(compiledProgram, start, engine.DefaultLimits())
	if err != nil {
		return nil, fmt.Errorf("starting session: %w", err)
	}
	if err := session.handleOutputs(outputs); err != nil {
		return nil, err
	}

	return session, nil
}

// applyTurn is the one place that actually calls engineservice.AdvanceTurn.
// Every other method below (HandleUserIntent, QuestionAnswered, ...) just
// builds the right engine.Signal for its situation and calls this.
func (s *gameSession) applyTurn(signal engine.Signal) error {
	outputs, err := engineservice.AdvanceTurn(s.program, s.start, s.signals, signal, engine.DefaultLimits())
	if err != nil {
		// ErrSignalRejected and ErrInputRejected are "expected, nothing
		// happened" outcomes for signal itself, not bugs (see README.md)
		// — a real consumer should check for these specifically with
		// errors.Is before treating something as a real failure to
		// log/alert on. ErrReplayDivergence is different: it means
		// s.signals (or the pinned Definition) no longer reproduces what
		// actually happened before — a data-integrity condition a real
		// consumer must alert loudly on, never treat as an ordinary
		// decline.
		return fmt.Errorf("turn rejected/failed: %w", err)
	}

	// A real consumer would durably append signal here — this is exactly
	// what it would persist instead of any Snapshot — before or alongside
	// acting on outputs.
	s.signals = append(s.signals, signal)

	return s.handleOutputs(outputs)
}

// handleOutputs deals with everything one turn's Outputs can contain,
// shared by newGameSession's first turn and every applyTurn call after it.
func (s *gameSession) handleOutputs(outputs []engine.Output) error {
	for _, output := range outputs {
		switch o := output.(type) {
		case engine.OpenQuestionOutput:
			// Remembered so a later answer (arriving as a websocket
			// message, an HTTP request, whatever) can be matched back to
			// what it's an answer to.
			s.pendingQuestions[o.InteractionID] = o
			// ... also actually deliver the question to o.Recipient
			// through whatever real transport this consumer uses.

		case engine.CloseQuestionOutput:
			delete(s.pendingQuestions, o.InteractionID)

		case engine.ActivatePresentationOutput, engine.UpdatePresentationOutput, engine.RemovePresentationOutput:
			// ... push o.Model / removal to o.Recipient's client.

		case engine.ScheduleTimerOutput:
			// ... a request to schedule a real timer: a consumer needs a
			// real scheduler (a job queue, time.AfterFunc, whatever)
			// that, when it fires, builds a SignalKindTimerExpired
			// signal and runs it through applyTurn.

		case engine.CancelTimerOutput:
			// ... cancel whatever real timer was scheduled for this slot.

		case engine.EmitEffectOutput:
			// ... purely cosmetic, deliver-or-don't, never affects state.

		case engine.RunCompletedOutput:
			// The one instance just ended; this is the one place a
			// session layer finds out the game is over.
			fmt.Println("game instance ended:", o.Outcome.Kind)
		}
	}
	return nil
}

// HandleUserIntent is called whenever a player submits an action from
// the outside world (a button press, a command, ...) - the ordinary,
// unprompted kind of input (see program.UserIntentDeclaration).
func (s *gameSession) HandleUserIntent(actor engine.UserID, intent string, fields map[string]engine.Value) error {
	return s.applyTurn(engine.Signal{
		Kind:   engine.SignalKindIntent,
		Intent: intent,
		Actor:  actor,
		Fields: fields,
	})
}

// QuestionAnswered is called whenever a player responds to a question we
// previously opened (see the OpenQuestionOutput handling above).
// interactionID is the InteractionID that opened it (see
// OpenQuestionOutput.InteractionID) — a real consumer would have
// resolved this from whatever the client's answer message referenced
// (a stored, previously-delivered handle), never a slot name.
func (s *gameSession) QuestionAnswered(respondent engine.UserID, interactionID engine.InteractionID, answer engine.Value) error {
	// A real implementation should confirm respondent actually matches
	// pending.Recipient, and reject/no-op if nothing is pending for this
	// interaction (the player answered something we no longer care
	// about, e.g. after a timeout already closed it).
	pending, ok := s.pendingQuestions[interactionID]
	if !ok {
		return fmt.Errorf("no pending question for interaction %v", interactionID)
	}
	if pending.Recipient != respondent {
		return fmt.Errorf("interaction %v is not awaiting an answer from %q", interactionID, respondent)
	}

	// The engine itself re-validates all of this (authorized respondent,
	// response type, any Validation expression) before ever accepting
	// it — see ErrInputRejected in README.md — so this session-layer
	// check is just to fail fast, not the real authority. The engine
	// resolves interactionID to the underlying slot/key itself; this
	// consumer never needs to know or supply one.
	err := s.applyTurn(engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: interactionID,
		Respondent:    respondent,
		Answer:        answer,
	})
	if err != nil {
		return err
	}

	// Cleared only once the turn actually accepted the answer;
	// handleOutputs's own CloseQuestionOutput handling also clears this
	// whenever the engine closes the slot on its own.
	delete(s.pendingQuestions, interactionID)
	return nil
}

// TimerExpired would be the analogous method for a real timer actually
// firing (see ScheduleTimerOutput above) — sketched, not filled in.
func (s *gameSession) TimerExpired(slot string) error {
	return s.applyTurn(engine.Signal{
		Kind: engine.SignalKindTimerExpired,
		Slot: slot,
	})
}
