package liveconnection

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_start_session_mock_test.go . sessionStarter,frontendSigner

// sessionStarter is the durable Session's side of starting a game.
type sessionStarter interface {
	CheckStartEligibility(ctx context.Context, sessionUUID string, userUUID string, playerCount int) (sessionpkg.StartEligibility, error)
	CommitStart(ctx context.Context, sessionUUID string, userUUID string, commit sessionpkg.StartCommit) (sessionpkg.CommitStartOutcome, error)
	LoadRunningSession(ctx context.Context, sessionUUID string) (sessionpkg.RunningSession, error)
}

// frontendSigner grants clients access to a game version's frontend package.
type frontendSigner interface {
	FrontendAccess(ctx context.Context, gameVersionUUID string) (sessionpkg.FrontendAccess, error)
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
		string(runtime.StartSandboxUnavailable),
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
	startedSessionsAdopted = metrics.NewCounter(
		"play_runtime_started_sessions_adopted_total",
		"Start requests that found the Session already RUNNING in the database while its controller was not, and caught the controller up.",
	)
	startScriptFailures = metrics.NewCounter(
		"play_runtime_start_script_failures_total",
		"Runs of a game's start or first projections that failed in the script.",
	)
)

// requestStart starts the game on the main goroutine, as userUUID asked, and
// waits for the outcome.
func (c *sessionController) requestStart(ctx context.Context, userUUID string) (runtime.StartSessionResult, error) {
	var result runtime.StartSessionResult
	var err error
	if c.do(ctx, func() { result, err = c.startSession(ctx, userUUID) }) {
		return result, err
	}
	if ctx.Err() != nil {
		return runtime.StartSessionResult{}, ctx.Err()
	}
	select {
	case <-c.ended:
		return runtime.StartSessionResult{Outcome: runtime.StartNotInLobby}, nil
	default:
		return runtime.StartSessionResult{}, errStopped
	}
}

// startSession starts the game with the lobby, as userUUID asked. An error
// means the Session stays in its lobby.
func (c *sessionController) startSession(ctx context.Context, userUUID string) (runtime.StartSessionResult, error) {
	began := time.Now()
	outcome, err := c.runStart(ctx, userUUID)
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

func (c *sessionController) runStart(ctx context.Context, userUUID string) (runtime.StartSessionOutcome, error) {
	participants, outcome, ok := c.beginStart()
	if !ok {
		return outcome, nil
	}
	outcome, err := c.startFrozenLobby(ctx, userUUID, participants)
	if err != nil || (outcome != runtime.SessionStarted && outcome != runtime.StartFailed) {
		c.cancelStart()
	}
	return outcome, err
}

// startFrozenLobby starts the game with participants. It completes or aborts
// the Start attempt; any other outcome leaves cancelling it to the caller.
func (c *sessionController) startFrozenLobby(ctx context.Context, userUUID string, participants []runtime.Participant) (runtime.StartSessionOutcome, error) {
	eligibility, err := c.deps.starts.CheckStartEligibility(ctx, c.sessionUUID, userUUID, len(participants))
	if err != nil {
		return "", fmt.Errorf("checking start eligibility: %w", err)
	}
	switch eligibility.Outcome {
	case sessionpkg.StartEligible:
	case sessionpkg.StartAlreadyStarted:
		return c.adoptRunningSession(ctx)
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
	access, err := c.deps.frontends.FrontendAccess(ctx, eligibility.GameVersionUUID)
	if err != nil {
		return "", fmt.Errorf("signing frontend access: %w", err)
	}

	participantIDs := make([]string, len(participants))
	for i, participant := range participants {
		participantIDs[i] = participant.ParticipantID
	}
	spec := sandbox.ContextSpec{GameVersionUUID: eligibility.GameVersionUUID, Script: eligibility.Script}
	game, views, started, err := c.startGame(ctx, spec, sandbox.StartInput{Participants: participantIDs, RandomSeed: c.deps.seeds()})
	if errors.Is(err, sandbox.ErrNoCapacity) {
		return runtime.StartSandboxUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if !started {
		return c.abortStart(ctx)
	}

	outcome, err := c.commitStart(ctx, userUUID, participants, game)
	if err != nil || outcome != runtime.SessionStarted {
		c.discardContext(ctx)
		return outcome, err
	}
	c.completeStart(participants, game, frontendOf(access), views, nil)
	c.recordStart(ctx)
	return runtime.SessionStarted, nil
}

// resumedGame is what a game resumed from a checkpoint keeps: each
// participant's processed COMMAND tokens and who was disconnected.
type resumedGame struct {
	processed    map[string][]string
	disconnected []string
}

// adoptRunningSession catches the controller up with a start the database
// already committed (its commit answer was lost, or this instance restarted):
// the game runs from the committed roster and latest checkpoint, in a restored
// context, since the database is the truth.
func (c *sessionController) adoptRunningSession(ctx context.Context) (runtime.StartSessionOutcome, error) {
	running, err := c.deps.starts.LoadRunningSession(ctx, c.sessionUUID)
	if errors.Is(err, sessionpkg.ErrSessionNotRunning) {
		// It ended meanwhile.
		return runtime.StartNotInLobby, nil
	}
	if err != nil {
		return "", fmt.Errorf("loading running session: %w", err)
	}
	saved, err := checkpoint.Decode(running.Checkpoint.Content)
	if err != nil {
		return "", err
	}
	access, err := c.deps.frontends.FrontendAccess(ctx, running.GameVersionUUID)
	if err != nil {
		return "", fmt.Errorf("signing frontend access: %w", err)
	}

	participants := make([]runtime.Participant, len(running.Participants))
	participantIDs := make([]string, len(running.Participants))
	for i, participant := range running.Participants {
		participants[i] = runtime.Participant{ParticipantID: participant.ParticipantID, UserUUID: participant.UserUUID, DisplayName: participant.DisplayName}
		participantIDs[i] = participant.ParticipantID
	}
	game := Game{
		GameVersionUUID: running.GameVersionUUID,
		Script:          running.Script,
		ContextRevision: saved.ContextRevision,
		State:           saved.State,
		PRNGState:       saved.PRNGState,

		ExecutedThroughSessionTimeMs: saved.ExecutedThroughSessionTimeMs,
	}
	// The committed game already ran this script, so any script failure here
	// is an error, not a start failure.
	err = c.restoreContext(ctx, game)
	if errors.Is(err, sandbox.ErrNoCapacity) {
		return runtime.StartSandboxUnavailable, nil
	}
	if err != nil {
		return "", fmt.Errorf("restoring the running game: %w", err)
	}
	views, err := c.projectAll(ctx, participantIDs)
	if err != nil {
		c.discardContext(ctx)
		return "", fmt.Errorf("projecting the restored game: %w", err)
	}
	c.completeStart(participants, game, frontendOf(access), views, &resumedGame{processed: saved.ProcessedCommandTokens, disconnected: saved.Disconnected})
	c.recordStart(ctx)
	startedSessionsAdopted.Inc(ctx)
	return runtime.StartAlreadyStarted, nil
}

func frontendOf(access sessionpkg.FrontendAccess) runtime.GameFrontend {
	return runtime.GameFrontend{
		URL:       access.URL,
		SHA256:    access.SHA256,
		ExpiresAt: access.ExpiresAt,
		Viewport:  runtime.GameViewport{AspectRatio: access.Viewport.AspectRatio, Width: access.Viewport.Width, Height: access.Viewport.Height},
	}
}

// commitStart makes the Session RUNNING with its roster and initial
// checkpoint, together, so a RUNNING Session is always recoverable.
func (c *sessionController) commitStart(ctx context.Context, userUUID string, participants []runtime.Participant, game Game) (runtime.StartSessionOutcome, error) {
	participantIDs := make([]string, len(participants))
	roster := make([]sessionpkg.StartParticipant, len(participants))
	for i, participant := range participants {
		participantIDs[i] = participant.ParticipantID
		roster[i] = sessionpkg.StartParticipant{UserUUID: participant.UserUUID, ParticipantID: participant.ParticipantID, DisplayName: participant.DisplayName}
	}
	encoded, err := checkpoint.Encode(checkpoint.Checkpoint{
		GameVersionUUID: game.GameVersionUUID,
		ContextRevision: game.ContextRevision,
		State:           game.State,
		PRNGState:       game.PRNGState,
		Participants:    participantIDs,
	})
	if err != nil {
		return "", err
	}
	committed, err := c.deps.starts.CommitStart(ctx, c.sessionUUID, userUUID, sessionpkg.StartCommit{
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
// fresh context if the script fails. started is false when it failed twice,
// and the game keeps its context only when it started.
func (c *sessionController) startGame(ctx context.Context, spec sandbox.ContextSpec, input sandbox.StartInput) (game Game, views map[string][]byte, started bool, err error) {
	for run := 1; run <= maxStartRuns; run++ {
		game, views, err := c.runGameStart(ctx, spec, input)
		var scriptFailure errScript
		if errors.As(err, &scriptFailure) {
			startScriptFailures.Inc(ctx)
			logging.LogFields(ctx, logging.Field("start_script_error", scriptFailure.detail))
			continue
		}
		if err != nil {
			return Game{}, nil, false, err
		}
		if run > 1 {
			monitoring.Alert(ctx, "game start succeeded only after its script failed")
		}
		return game, views, true, nil
	}
	return Game{}, nil, false, nil
}

// runGameStart runs start and every participant's projection in a new
// context, discarded unless it succeeds.
func (c *sessionController) runGameStart(ctx context.Context, spec sandbox.ContextSpec, input sandbox.StartInput) (Game, map[string][]byte, error) {
	if err := c.createContext(ctx, spec); err != nil {
		return Game{}, nil, err
	}
	game, views, err := c.startInContext(ctx, spec, input)
	if err != nil {
		c.discardContext(ctx)
	}
	return game, views, err
}

func (c *sessionController) startInContext(ctx context.Context, spec sandbox.ContextSpec, input sandbox.StartInput) (Game, map[string][]byte, error) {
	result, err := c.deps.sandbox.Start(ctx, c.contextID, input)
	if err != nil {
		return Game{}, nil, fmt.Errorf("starting game: %w", err)
	}
	switch result.Outcome {
	case sandbox.ScriptSucceeded:
	case sandbox.ScriptFailed:
		return Game{}, nil, errScript{detail: result.ScriptError}
	default:
		return Game{}, nil, fmt.Errorf("unknown start outcome %q", result.Outcome)
	}

	// Projected before committing, so a projection the script fails also
	// fails the Start instead of a RUNNING Session.
	views, err := c.projectAll(ctx, input.Participants)
	if err != nil {
		return Game{}, nil, err
	}
	return Game{
		GameVersionUUID: spec.GameVersionUUID,
		Script:          spec.Script,
		ContextRevision: result.ContextRevision,
		State:           result.State,
		PRNGState:       result.PRNGState,
	}, views, nil
}

// beginStart freezes the lobby for one Start attempt and returns its
// participants, numbered P1..Pn in the order they joined. ok is false, with
// the outcome, when the Session cannot start now. Whether the game allows that
// many players is the durable Session's decision.
func (c *sessionController) beginStart() (participants []runtime.Participant, outcome runtime.StartSessionOutcome, ok bool) {
	s := c.lock()
	defer c.unlock()
	switch s.phase {
	case phaseLobby:
	case phaseStarting:
		return nil, runtime.StartInProgress, false
	case phaseRunning:
		return nil, runtime.StartAlreadyStarted, false
	default:
		return nil, runtime.StartNotInLobby, false
	}
	s.phase = phaseStarting
	s.startSettled = make(chan struct{})
	s.participants = make([]runtime.Participant, len(s.lobby))
	for i, userUUID := range s.lobby {
		s.participants[i] = runtime.Participant{ParticipantID: fmt.Sprintf("P%d", i+1), UserUUID: userUUID, DisplayName: s.players[userUUID].displayName}
	}
	return slices.Clone(s.participants), "", true
}

// cancelStart returns the Session to its lobby after a Start attempt that
// did not start it. Players who disconnected during the attempt leave it now.
func (c *sessionController) cancelStart() {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseStarting {
		return
	}
	s.phase = phaseLobby
	s.settleStart()
	s.participants = nil
	before := len(s.lobby)
	s.lobby = slices.DeleteFunc(s.lobby, func(userUUID string) bool {
		if s.players[userUUID].isEnded() {
			delete(s.players, userUUID)
			return true
		}
		return false
	})
	if len(s.lobby) != before {
		s.broadcastLobby()
	}
}

// completeStart runs the started game with the committed roster: the lobby
// ends, every connection learns who plays, each participant gets a FULL first
// view; a lobby player left out of participants is ended. A resumed game
// (nil for a new one) keeps what its checkpoint knew, and the participants
// who were playing get the recovery window instead of the transport grace.
func (c *sessionController) completeStart(participants []runtime.Participant, game Game, frontend runtime.GameFrontend, views map[string][]byte, resumed *resumedGame) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseStarting {
		return
	}
	s.phase = phaseRunning
	s.settleStart()
	s.participants = slices.Clone(participants)
	s.game = game
	s.start = runtime.SessionStart{Participants: slices.Clone(participants), Frontend: frontend}
	s.lobby = nil
	s.runningSince = s.now()
	s.baseSessionTimeMs = game.ExecutedThroughSessionTimeMs
	s.lastActivity = s.runningSince
	s.lastCheckpoint = s.runningSince
	awaitUntil, cause := s.runningSince.Add(s.transportGrace), disconnectGraceExpired
	if resumed == nil {
		resumed = &resumedGame{}
	} else {
		awaitUntil, cause = s.runningSince.Add(c.deps.recoveryWindow), disconnectRecoveryEnded
	}
	defaults := s.manifest.Inputs.Defaults()
	for _, participant := range participants {
		s.inputs[participant.ParticipantID] = newParticipantInputs(defaults, s.limits)
		s.commands[participant.ParticipantID] = newParticipantCommands(resumed.processed[participant.ParticipantID], s.limits)
		// Whoever has no connection now has until awaitUntil to come.
		presence := &participantPresence{disconnected: slices.Contains(resumed.disconnected, participant.ParticipantID)}
		if !presence.disconnected {
			presence.await(awaitUntil, cause)
		}
		s.presence[participant.ParticipantID] = presence
		s.views[participant.ParticipantID] = views[participant.ParticipantID]
	}
	inRoster := make(map[string]bool, len(participants))
	for _, participant := range participants {
		inRoster[participant.UserUUID] = true
	}
	for userUUID, player := range s.players {
		if !inRoster[userUUID] {
			player.end(runtime.EndSessionEnded)
			delete(s.players, userUUID)
		}
	}
	for _, admin := range s.admins {
		admin.markStarted(s.start)
	}
	for _, participant := range s.participants {
		player, found := s.players[participant.UserUUID]
		if !found || player.isEnded() {
			continue
		}
		if s.presence[participant.ParticipantID].disconnected {
			// Only rejoining brings them back.
			player.end(runtime.EndDisconnected)
			delete(s.players, participant.UserUUID)
			continue
		}
		player.participantID = participant.ParticipantID
		s.presence[participant.ParticipantID].attach()
		s.inputs[participant.ParticipantID].stream = player.id
		s.sendFull(c.root, player, participant.ParticipantID)
		// Last, so everything above is visible to whoever sees the start.
		player.markStarted(s.start)
	}
	s.broadcastRoster()
}

// abortStart ends the Session after its game failed to start, durably and
// then live: every connection learns it was aborted. A Session that did not
// end is an error, so the attempt is cancelled.
func (c *sessionController) abortStart(ctx context.Context) (runtime.StartSessionOutcome, error) {
	result, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: sessionpkg.EndGameStartFailure}, nil)
	if err != nil {
		return "", fmt.Errorf("aborting session: %w", err)
	}
	if result.EndCause == "" {
		return "", fmt.Errorf("aborting session: it is %s and did not end", result.Status)
	}
	return runtime.StartFailed, nil
}
