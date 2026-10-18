package liveconnection

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_start_session_mock_test.go . sessionStarter,frontendSigner,gameSandbox

// sessionStarter is the durable Session's side of starting a game.
type sessionStarter interface {
	CheckStartEligibility(ctx context.Context, sessionUUID string, userUUID string, playerCount int) (sessionpkg.StartEligibility, error)
	CommitStart(ctx context.Context, sessionUUID string, userUUID string, commit sessionpkg.StartCommit) (sessionpkg.CommitStartOutcome, error)
	AbortStart(ctx context.Context, sessionUUID string) error
}

// frontendSigner grants clients access to a game version's frontend package.
type frontendSigner interface {
	FrontendAccess(ctx context.Context, gameVersionUUID string) (sessionpkg.FrontendAccess, error)
}

// gameSandbox runs a game's scripts in execution contexts, one per running
// Session. An error is an infrastructure failure, never the script's.
type gameSandbox interface {
	CreateContext(ctx context.Context, spec sandbox.ContextSpec) (string, error)
	Start(ctx context.Context, contextID string, input sandbox.StartInput) (sandbox.StartResult, error)
	Project(ctx context.Context, contextID string, participantID string) (sandbox.ProjectResult, error)
	DiscardContext(ctx context.Context, contextID string) error
}

// maxStartRuns is how many times a game's start runs before a script failure
// aborts the Session: a failure is retried once in a fresh context.
const maxStartRuns = 2

const startOutcomeError = "error"

var (
	startOutcomeLabel = metrics.NewLabel("outcome",
		string(runtime.SessionStarted),
		string(runtime.StartAlreadyStarted),
		string(runtime.StartInProgress),
		string(runtime.StartNotInLobby),
		string(runtime.StartInvalidPlayerCount),
		string(runtime.StartGameUnavailable),
		string(runtime.StartFailed),
		startOutcomeError,
	)

	sessionStarts = metrics.NewCounter(
		"play_runtime_session_start_total",
		"Attempts to start a Session's game, by outcome.",
		startOutcomeLabel,
	)
	sessionStartDuration = metrics.NewHistogram(
		"play_runtime_session_start_duration_seconds",
		"How long starting a Session's game took, by outcome.",
		metrics.LatencyBuckets(),
		startOutcomeLabel,
	)
	startScriptFailures = metrics.NewCounter(
		"play_runtime_start_script_failures_total",
		"Runs of a game's start or first projections that failed in the script.",
	)
)

// startSession starts sessionUUID's game with its lobby, as userUUID asked.
// An error means the Session stays in its lobby.
func (m *Manager) startSession(ctx context.Context, sessionController *controller.Controller, sessionUUID string, userUUID string) (runtime.StartSessionResult, error) {
	began := time.Now()
	outcome, err := m.runStart(ctx, sessionController, sessionUUID, userUUID)
	label := string(outcome)
	if err != nil {
		label = startOutcomeError
	}
	sessionStarts.Inc(ctx, startOutcomeLabel.Value(label))
	sessionStartDuration.ObserveDuration(ctx, time.Since(began), startOutcomeLabel.Value(label))
	logging.LogFields(ctx, logging.Field("start_outcome", label))
	if err != nil {
		return runtime.StartSessionResult{}, err
	}
	return runtime.StartSessionResult{Outcome: outcome}, nil
}

func (m *Manager) runStart(ctx context.Context, sessionController *controller.Controller, sessionUUID string, userUUID string) (runtime.StartSessionOutcome, error) {
	participants, outcome, ok := sessionController.BeginStart()
	if !ok {
		return outcome, nil
	}
	outcome, err := m.startFrozenLobby(ctx, sessionController, sessionUUID, userUUID, participants)
	if err != nil || (outcome != runtime.SessionStarted && outcome != runtime.StartFailed) {
		sessionController.CancelStart()
	}
	return outcome, err
}

// startFrozenLobby starts the game with participants. It completes or aborts
// the Start attempt; any other outcome leaves cancelling it to the caller.
func (m *Manager) startFrozenLobby(ctx context.Context, sessionController *controller.Controller, sessionUUID string, userUUID string, participants []runtime.Participant) (runtime.StartSessionOutcome, error) {
	eligibility, err := m.starts.CheckStartEligibility(ctx, sessionUUID, userUUID, len(participants))
	if err != nil {
		return "", fmt.Errorf("checking start eligibility: %w", err)
	}
	switch eligibility.Outcome {
	case sessionpkg.StartEligible:
	case sessionpkg.StartAlreadyStarted:
		return runtime.StartAlreadyStarted, nil
	case sessionpkg.StartSessionNotFound, sessionpkg.StartNotInLobby:
		return runtime.StartNotInLobby, nil
	case sessionpkg.StartInvalidPlayerCount:
		return runtime.StartInvalidPlayerCount, nil
	case sessionpkg.StartGameUnavailable:
		return runtime.StartGameUnavailable, nil
	default:
		// not_creator included: admin connections are only the creator's.
		return "", fmt.Errorf("unexpected start eligibility %q", eligibility.Outcome)
	}
	// Signed before running the game, so failing to sign wastes no context.
	access, err := m.frontends.FrontendAccess(ctx, eligibility.GameVersionUUID)
	if err != nil {
		return "", fmt.Errorf("signing frontend access: %w", err)
	}

	participantIDs := make([]string, len(participants))
	for i, participant := range participants {
		participantIDs[i] = participant.ParticipantID
	}
	spec := sandbox.ContextSpec{GameVersionUUID: eligibility.GameVersionUUID, Script: eligibility.Script}
	game, views, started, err := m.startGame(ctx, spec, sandbox.StartInput{Participants: participantIDs, RandomSeed: m.seeds()})
	if err != nil {
		return "", err
	}
	if !started {
		if err := m.starts.AbortStart(ctx, sessionUUID); err != nil {
			return "", fmt.Errorf("aborting session: %w", err)
		}
		sessionController.AbortStart()
		return runtime.StartFailed, nil
	}

	outcome, err := m.commitStart(ctx, sessionUUID, userUUID, eligibility.GameVersionUUID, participants, game)
	if err != nil || outcome != runtime.SessionStarted {
		m.discardContext(ctx, game.ContextID)
		return outcome, err
	}
	frontend := runtime.GameFrontend{URL: access.URL, SHA256: access.SHA256, ExpiresAt: access.ExpiresAt}
	sessionController.CompleteStart(game, frontend, views)
	return runtime.SessionStarted, nil
}

// commitStart makes the Session RUNNING with its roster and initial
// checkpoint, together, so a RUNNING Session is always recoverable.
func (m *Manager) commitStart(ctx context.Context, sessionUUID string, userUUID string, gameVersionUUID string, participants []runtime.Participant, game controller.Game) (runtime.StartSessionOutcome, error) {
	participantIDs := make([]string, len(participants))
	roster := make([]sessionpkg.StartParticipant, len(participants))
	for i, participant := range participants {
		participantIDs[i] = participant.ParticipantID
		roster[i] = sessionpkg.StartParticipant{UserUUID: participant.UserUUID, ParticipantID: participant.ParticipantID}
	}
	encoded, err := checkpoint.Encode(checkpoint.Checkpoint{
		GameVersionUUID: gameVersionUUID,
		ContextRevision: game.ContextRevision,
		State:           game.State,
		PRNGState:       game.PRNGState,
		Participants:    participantIDs,
	})
	if err != nil {
		return "", err
	}
	committed, err := m.starts.CommitStart(ctx, sessionUUID, userUUID, sessionpkg.StartCommit{
		Participants: roster,
		Checkpoint:   sessionpkg.Checkpoint{ContextRevision: game.ContextRevision, Content: encoded},
	})
	if err != nil {
		return "", fmt.Errorf("committing start: %w", err)
	}
	switch committed {
	case sessionpkg.CommitStartCommitted:
		return runtime.SessionStarted, nil
	case sessionpkg.CommitStartNotInLobby:
		return runtime.StartNotInLobby, nil
	default:
		return "", fmt.Errorf("unknown commit start outcome %q", committed)
	}
}

// startGame runs the game's start and first projections, once more in a
// fresh context if the script fails. started is false when it failed twice.
func (m *Manager) startGame(ctx context.Context, spec sandbox.ContextSpec, input sandbox.StartInput) (game controller.Game, views map[string][]byte, started bool, err error) {
	for run := 1; run <= maxStartRuns; run++ {
		game, views, scriptErr, err := m.runGameStart(ctx, spec, input)
		if err != nil {
			return controller.Game{}, nil, false, err
		}
		if scriptErr == "" {
			if run > 1 {
				monitoring.Alert(ctx, "game start succeeded only after its script failed")
			}
			return game, views, true, nil
		}
		startScriptFailures.Inc(ctx)
		logging.LogFields(ctx, logging.Field("start_script_error", scriptErr))
	}
	return controller.Game{}, nil, false, nil
}

// runGameStart runs start and every participant's projection in a new
// context, discarded unless it succeeds; scriptErr is set when the script
// failed.
func (m *Manager) runGameStart(ctx context.Context, spec sandbox.ContextSpec, input sandbox.StartInput) (game controller.Game, views map[string][]byte, scriptErr string, err error) {
	contextID, err := m.sandbox.CreateContext(ctx, spec)
	if err != nil {
		return controller.Game{}, nil, "", fmt.Errorf("creating sandbox context: %w", err)
	}
	game, views, scriptErr, err = m.startInContext(ctx, contextID, input)
	if err != nil || scriptErr != "" {
		m.discardContext(ctx, contextID)
	}
	return game, views, scriptErr, err
}

func (m *Manager) startInContext(ctx context.Context, contextID string, input sandbox.StartInput) (game controller.Game, views map[string][]byte, scriptErr string, err error) {
	result, err := m.sandbox.Start(ctx, contextID, input)
	if err != nil {
		return controller.Game{}, nil, "", fmt.Errorf("starting game: %w", err)
	}
	switch result.Outcome {
	case sandbox.ScriptSucceeded:
	case sandbox.ScriptFailed:
		return controller.Game{}, nil, result.ScriptError, nil
	default:
		return controller.Game{}, nil, "", fmt.Errorf("unknown start outcome %q", result.Outcome)
	}

	// Projected before committing, so a projection the script fails also
	// fails the Start instead of a RUNNING Session.
	views = make(map[string][]byte, len(input.Participants))
	for _, participantID := range input.Participants {
		projection, err := m.sandbox.Project(ctx, contextID, participantID)
		if err != nil {
			return controller.Game{}, nil, "", fmt.Errorf("projecting for %s: %w", participantID, err)
		}
		switch projection.Outcome {
		case sandbox.ScriptSucceeded:
			views[participantID] = projection.View
		case sandbox.ScriptFailed:
			return controller.Game{}, nil, projection.ScriptError, nil
		default:
			return controller.Game{}, nil, "", fmt.Errorf("unknown project outcome %q", projection.Outcome)
		}
	}
	return controller.Game{
		ContextID:       contextID,
		ContextRevision: result.ContextRevision,
		State:           result.State,
		PRNGState:       result.PRNGState,
	}, views, "", nil
}

// discardContext releases a context no game will use. Failing only leaks it
// in the sandbox, so it is logged, not returned.
func (m *Manager) discardContext(ctx context.Context, contextID string) {
	if err := m.sandbox.DiscardContext(ctx, contextID); err != nil {
		logging.LogError(ctx, fmt.Errorf("discarding sandbox context: %w", err))
	}
}
