package liveconnection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/gamemanifest"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// previewStore is the durable side of previews
// (play/session/workflows/previewlifecycle).
type previewStore interface {
	OpenPreview(ctx context.Context, versionUUID string, ownerUUID string, revision int64, players int) (sessionpkg.PreviewOpening, error)
	EndPreview(ctx context.Context, previewUUID string, reason sessionpkg.PreviewEndReason) (bool, error)
	FindPreview(ctx context.Context, previewUUID string) (sessionpkg.Preview, error)
}

// DefaultMaxLivePreviews is how many previews one instance runs at once.
const DefaultMaxLivePreviews = 50

// previewEpoch is every preview controller's epoch: previews are never
// owned by another instance.
const previewEpoch = 1

// previewEndedStatus is the status a preview's connections are told it
// ended with; its EndCause is the PreviewEndReason.
const previewEndedStatus = "ENDED"

var (
	previewStartOutcomeLabel = metrics.NewLabel("outcome",
		string(runtime.PreviewStarted), string(runtime.PreviewBuildNotReady), string(runtime.PreviewInvalidPlayers),
		string(runtime.PreviewUnavailable), string(runtime.PreviewScriptFailure),
	)

	previewStarts = metrics.NewCounter(
		"play_runtime_preview_starts_total",
		"Requests to start a preview of a draft build, by outcome.",
		previewStartOutcomeLabel,
	)
	previewSwitches = metrics.NewCounter(
		"play_runtime_preview_switches_total",
		"Times a preview's owner switched the participant they play as.",
	)
	previewPauses = metrics.NewCounter(
		"play_runtime_preview_pauses_total",
		"Times a preview was paused.",
	)
)

// Previews runs previews on the live controller: a draft's owner playing a
// SUCCEEDED build with synthetic participants, one at a time, from one
// connection. Nothing of a preview is checkpointed, replayed or recovered.
type Previews struct {
	resolver *ControllerResolver
	store    previewStore
}

// NewPreviews constructs the previews of this instance, at most
// maxLivePreviews at once (zero takes DefaultMaxLivePreviews).
func NewPreviews(maxLivePreviews int, sandbox gameSandbox, store previewStore) *Previews {
	if maxLivePreviews <= 0 {
		maxLivePreviews = DefaultMaxLivePreviews
	}
	root, cancel := context.WithCancel(context.Background())
	unjoinable := previewsAreNotSessions{}
	deps := &controllerDependencies{
		adminJoins:         unjoinable,
		playerJoins:        unjoinable,
		manifests:          unjoinable,
		starts:             unjoinable,
		frontends:          unjoinable,
		limits:             previewLimits{store: store},
		ends:               previewEnds{store: store},
		checkpoints:        previewsKeepNothing{},
		heartbeats:         previewsKeepNothing{},
		replay:             previewsKeepNothing{},
		owners:             previewsKeepNothing{},
		faults:             previewsKeepNothing{},
		sandbox:            sandbox,
		now:                time.Now,
		seeds:              randomSeed,
		tickInterval:       defaultTickInterval,
		transportGrace:     defaultTransportGrace,
		recoveryWindow:     defaultRecoveryWindow,
		contextIdleTimeout: defaultContextIdleTimeout,
		heartbeatInterval:  sessionpkg.OwnerHeartbeatInterval,
		checkpointInterval: defaultCheckpointInterval,
		limitsInterval:     defaultLimitsInterval,
		inactivityTimeout:  defaultInactivityTimeout,
		liveLimits:         defaultLiveLimits(),
		logger:             slog.Default(),
		previews:           true,
	}
	return &Previews{
		resolver: &ControllerResolver{
			deps:            deps,
			maxLiveSessions: maxLivePreviews,
			controllers:     make(map[string]*sessionController),
			root:            root,
			cancel:          cancel,
		},
		store: store,
	}
}

// Close ends every preview of this instance and waits for their controllers.
func (p *Previews) Close() {
	p.resolver.Close()
}

// StartPreview opens and starts a preview of revision (0: the current one)
// of the DRAFT versionUUID for ownerUUID with players synthetic
// participants, ending the version's running preview as replaced.
func (p *Previews) StartPreview(ctx context.Context, versionUUID string, ownerUUID string, revision int64, players int) (runtime.StartPreviewResult, error) {
	defer logging.Step(ctx, "liveconnection.start_preview").Close()

	opening, err := p.store.OpenPreview(ctx, versionUUID, ownerUUID, revision, players)
	if err != nil {
		return runtime.StartPreviewResult{}, fmt.Errorf("opening preview: %w", err)
	}
	switch opening.Outcome {
	case sessionpkg.PreviewOpened:
	case sessionpkg.PreviewBuildNotReady:
		return p.started(ctx, runtime.StartPreviewResult{Outcome: runtime.PreviewBuildNotReady, BuildStatus: string(opening.BuildStatus)}), nil
	case sessionpkg.PreviewInvalidPlayers:
		return p.started(ctx, runtime.StartPreviewResult{Outcome: runtime.PreviewInvalidPlayers, MinPlayers: opening.MinPlayers, MaxPlayers: opening.MaxPlayers}), nil
	default:
		return runtime.StartPreviewResult{}, fmt.Errorf("unknown preview open outcome %q", opening.Outcome)
	}
	if opening.Replaced != "" {
		p.endHere(ctx, opening.Replaced, sessionpkg.EndPlatformTerminated)
	}

	controller, rejection := p.resolver.controllerFor(ctx, opening.PreviewUUID, previewEpoch)
	if rejection != "" {
		return p.failedStart(ctx, opening.PreviewUUID, runtime.PreviewUnavailable, sessionpkg.PreviewEndUnavailable)
	}
	var outcome runtime.StartPreviewOutcome
	if !controller.do(ctx, func() { outcome, err = controller.startPreview(ctx, ownerUUID, opening) }) {
		return runtime.StartPreviewResult{}, errStopped
	}
	if err != nil {
		if _, ended := p.store.EndPreview(context.WithoutCancel(ctx), opening.PreviewUUID, sessionpkg.PreviewEndUnavailable); ended != nil {
			logging.LogError(ctx, ended)
		}
		return runtime.StartPreviewResult{}, err
	}
	switch outcome {
	case runtime.PreviewStarted:
		return p.started(ctx, runtime.StartPreviewResult{Outcome: outcome, PreviewUUID: opening.PreviewUUID, Revision: opening.Revision, Participants: slices.Clone(opening.Participants)}), nil
	case runtime.PreviewScriptFailure:
		return p.failedStart(ctx, opening.PreviewUUID, outcome, sessionpkg.PreviewEndScriptFailure)
	default:
		return p.failedStart(ctx, opening.PreviewUUID, outcome, sessionpkg.PreviewEndUnavailable)
	}
}

// failedStart records that the preview could not start and answers outcome.
func (p *Previews) failedStart(ctx context.Context, previewUUID string, outcome runtime.StartPreviewOutcome, reason sessionpkg.PreviewEndReason) (runtime.StartPreviewResult, error) {
	if _, err := p.store.EndPreview(context.WithoutCancel(ctx), previewUUID, reason); err != nil {
		return runtime.StartPreviewResult{}, err
	}
	return p.started(ctx, runtime.StartPreviewResult{Outcome: outcome}), nil
}

func (p *Previews) started(ctx context.Context, result runtime.StartPreviewResult) runtime.StartPreviewResult {
	previewStarts.Inc(ctx, previewStartOutcomeLabel.Value(string(result.Outcome)))
	logging.LogFields(ctx, logging.Field("preview_start_outcome", string(result.Outcome)), logging.Field("preview_uuid", result.PreviewUUID))
	return result
}

// endHere ends the preview previewUUID runs on this instance, if it runs
// here, with cause.
func (p *Previews) endHere(ctx context.Context, previewUUID string, cause sessionpkg.EndCause) {
	controller := p.resolver.live(previewUUID)
	if controller == nil {
		return
	}
	controller.do(ctx, func() {
		if _, err := controller.endSession(ctx, sessionpkg.SessionEnd{Cause: cause}, nil); err != nil {
			logging.LogError(ctx, err)
		}
	})
}

// AttachPreview attaches ownerUUID's connection to previewUUID, replacing
// the connection already attached. A preview that is not running here is
// told as ended: previews are never recovered.
func (p *Previews) AttachPreview(ctx context.Context, previewUUID string, ownerUUID string) (runtime.AttachPreviewResult, error) {
	defer logging.Step(ctx, "liveconnection.attach_preview").Close()

	if controller := p.resolver.live(previewUUID); controller != nil {
		if result, attached := controller.attachPreview(ctx, previewUUID, ownerUUID); attached {
			return result, nil
		}
	}
	preview, err := p.store.FindPreview(ctx, previewUUID)
	if errors.Is(err, sessionpkg.ErrPreviewNotFound) {
		return runtime.AttachPreviewResult{Outcome: runtime.PreviewNotFound}, nil
	}
	if err != nil {
		return runtime.AttachPreviewResult{}, err
	}
	if preview.OwnerUserUUID != ownerUUID {
		return runtime.AttachPreviewResult{Outcome: runtime.PreviewNotFound}, nil
	}
	if !preview.Ended {
		// Nothing runs it: this instance restarted since it started.
		if _, err := p.store.EndPreview(ctx, previewUUID, sessionpkg.PreviewEndUnavailable); err != nil {
			return runtime.AttachPreviewResult{}, err
		}
		return runtime.AttachPreviewResult{Outcome: runtime.PreviewAlreadyEnded, EndReason: string(sessionpkg.PreviewEndUnavailable)}, nil
	}
	return runtime.AttachPreviewResult{Outcome: runtime.PreviewAlreadyEnded, EndReason: string(preview.EndReason)}, nil
}

// live is the controller of sessionUUID on this instance that has not
// ended, or nil.
func (r *ControllerResolver) live(sessionUUID string) *sessionController {
	r.mu.Lock()
	controller, found := r.controllers[sessionUUID]
	r.mu.Unlock()
	if !found {
		return nil
	}
	select {
	case <-controller.ended:
		return nil
	default:
		return controller
	}
}

// startPreview starts opening's game with its synthetic participants, all
// played by ownerUUID. Runs on the main goroutine.
func (c *sessionController) startPreview(ctx context.Context, ownerUUID string, opening sessionpkg.PreviewOpening) (runtime.StartPreviewOutcome, error) {
	rules, err := gamemanifest.Compile(opening.Manifest)
	if err != nil {
		c.endPreviewStart()
		return "", err
	}
	participants := make([]runtime.Participant, len(opening.Participants))
	for i, participantID := range opening.Participants {
		participants[i] = runtime.Participant{ParticipantID: participantID, UserUUID: ownerUUID}
	}
	s := c.lock()
	s.manifest = Manifest{MinPlayers: rules.MinPlayers, MaxPlayers: rules.MaxPlayers, Inputs: rules.Inputs, Commands: rules.Commands, Update: rules.Update, Rejoin: rules.Rejoin}
	s.phase = phaseStarting
	s.startSettled = make(chan struct{})
	s.previewRevision = opening.Revision
	s.actingAs = opening.Participants[0]
	c.unlock()
	c.manifestLoaded = true

	spec := sandbox.ContextSpec{GameVersionUUID: opening.GameVersionUUID, Script: opening.Script, Profile: scriptcheck.Profile(opening.ScriptProfile)}
	game, views, started, err := c.startGame(ctx, spec, sandbox.StartInput{Participants: slices.Clone(opening.Participants), RandomSeed: c.deps.seeds()})
	if errors.Is(err, sandbox.ErrNoCapacity) {
		c.endPreviewStart()
		return runtime.PreviewUnavailable, nil
	}
	if err != nil {
		c.endPreviewStart()
		return "", err
	}
	if !started {
		c.endPreviewStart()
		return runtime.PreviewScriptFailure, nil
	}
	c.completeStart(participants, game, frontendOf(opening.Frontend), views, nil)
	return runtime.PreviewStarted, nil
}

// endPreviewStart ends a preview whose start failed here.
func (c *sessionController) endPreviewStart() {
	s := c.lock()
	defer c.unlock()
	c.end(s, runtime.ConnectionEnd{Reason: runtime.EndSessionEnded})
}

// attachPreview attaches ownerUUID's connection, as the participant last
// acted as; false when the preview no longer runs here.
func (c *sessionController) attachPreview(ctx context.Context, previewUUID string, ownerUUID string) (runtime.AttachPreviewResult, bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || len(s.participants) == 0 {
		return runtime.AttachPreviewResult{}, false
	}
	if s.participants[0].UserUUID != ownerUUID {
		return runtime.AttachPreviewResult{Outcome: runtime.PreviewNotFound}, true
	}
	if older := s.players[ownerUUID]; older != nil && !older.isEnded() {
		older.end(runtime.EndReplaced)
		playersReplaced.Inc(ctx)
	}
	attached := newConnection(uuid.NewString(), runtime.Player{UserUUID: ownerUUID})
	attached.participantID = s.actingAs
	s.players[ownerUUID] = attached
	s.inputs[s.actingAs].follow(attached.id)
	s.sendFull(ctx, attached, s.actingAs)
	s.recordActivity()

	participantIDs := make([]string, len(s.participants))
	for i, participant := range s.participants {
		participantIDs[i] = participant.ParticipantID
	}
	start := runtime.PreviewStart{
		PreviewUUID: previewUUID, Revision: s.previewRevision, Participants: participantIDs,
		ActingAs: s.actingAs, Paused: s.paused, Frontend: s.start.Frontend,
	}
	return runtime.AttachPreviewResult{Outcome: runtime.PreviewAttached, Start: start, Connection: c.previewConnection(attached)}, true
}

// previewConnection is what the owner's attached connection can do.
func (c *sessionController) previewConnection(owner *connection) runtime.PreviewConnection {
	return runtime.PreviewConnection{
		ActAs: func(ctx context.Context, participantID string) error {
			return c.actAs(ctx, owner, participantID)
		},
		Sample: func(ctx context.Context, sample runtime.Sample) error {
			return c.acceptSample(ctx, owner, sample)
		},
		Command: func(ctx context.Context, command runtime.Command) (runtime.CommandResult, error) {
			return c.acceptCommand(ctx, owner, command)
		},
		Resync: func(ctx context.Context) error {
			c.resync(ctx, owner)
			return nil
		},
		Pause: func(ctx context.Context) error {
			c.pausePreview(owner, true)
			return nil
		},
		Resume: func(ctx context.Context) error {
			c.pausePreview(owner, false)
			return nil
		},
		End: func(ctx context.Context) error {
			if !c.do(ctx, func() {
				if _, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled}, nil); err != nil {
					logging.LogError(ctx, err)
				}
			}) && ctx.Err() != nil {
				return ctx.Err()
			}
			return nil
		},
		NextView: func(ctx context.Context) (runtime.OwnView, bool) {
			return owner.views.next(ctx, owner.ended)
		},
		Ended: owner.wait,
		Disconnect: func() {
			s := c.lock()
			// The synthetic participants stay: only what the owner held is
			// let go.
			if s.players[owner.userUUID] == owner {
				if s.phase == phaseRunning {
					s.resetInputs(owner.participantID)
				}
				delete(s.players, owner.userUUID)
			}
			c.unlock()
			owner.end(runtime.EndDisconnected)
		},
	}
}

// actAs makes owner play as participantID from now on, letting go of the
// inputs of the participant it played as, with a FULL view of its new one.
func (c *sessionController) actAs(ctx context.Context, owner *connection, participantID string) error {
	s := c.lock()
	defer c.unlock()
	if !slices.ContainsFunc(s.participants, func(participant runtime.Participant) bool { return participant.ParticipantID == participantID }) {
		return fmt.Errorf("%w: %s", runtime.ErrUnknownParticipant, participantID)
	}
	if s.phase != phaseRunning || s.players[owner.userUUID] != owner {
		return nil
	}
	if owner.participantID != participantID {
		s.resetInputs(owner.participantID)
	}
	owner.participantID = participantID
	s.actingAs = participantID
	s.inputs[participantID].follow(owner.id)
	s.sendFull(ctx, owner, participantID)
	s.recordActivity()
	previewSwitches.Inc(ctx)
	return nil
}

// pausePreview pauses or resumes the preview owner plays. Resuming moves the
// start of Session time by the pause, so the game never sees it.
func (c *sessionController) pausePreview(owner *connection, pause bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || s.players[owner.userUUID] != owner || s.paused == pause {
		return
	}
	now := s.now()
	if pause {
		s.paused, s.pausedAt = true, now
		previewPauses.Inc(c.root)
	} else {
		s.runningSince = s.runningSince.Add(now.Sub(s.pausedAt))
		s.paused, s.pausedAt = false, time.Time{}
	}
	s.recordActivity()
}

// previewLimits are a preview's deadlines: it lives at most until its
// recorded expiry.
type previewLimits struct {
	store previewStore
}

func (l previewLimits) LoadSessionLimits(ctx context.Context, previewUUID string) (sessionpkg.SessionLimits, error) {
	preview, err := l.store.FindPreview(ctx, previewUUID)
	if err != nil {
		return sessionpkg.SessionLimits{}, err
	}
	return sessionpkg.SessionLimits{LobbyExpiresAt: preview.ExpiresAt, ExpiresAt: preview.ExpiresAt}, nil
}

// previewEnds records a preview's end; its connections are told the reason
// as the EndCause.
type previewEnds struct {
	store previewStore
}

func (e previewEnds) EndSession(ctx context.Context, previewUUID string, end sessionpkg.SessionEnd) (sessionpkg.EndSessionResult, error) {
	reason := previewEndReason(end.Cause)
	if _, err := e.store.EndPreview(ctx, previewUUID, reason); err != nil {
		return sessionpkg.EndSessionResult{}, err
	}
	return sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: previewEndedStatus, EndCause: string(reason)}, nil
}

// previewEndReason is why a preview the controller ends with cause ended.
func previewEndReason(cause sessionpkg.EndCause) sessionpkg.PreviewEndReason {
	switch cause {
	case sessionpkg.EndGameFinished:
		return sessionpkg.PreviewEndFinished
	case sessionpkg.EndInactivityTimeout, sessionpkg.EndMaxLifetime:
		return sessionpkg.PreviewEndIdle
	case sessionpkg.EndHostCancelled:
		return sessionpkg.PreviewEndEndedByOwner
	case sessionpkg.EndPlatformTerminated:
		return sessionpkg.PreviewEndReplaced
	case sessionpkg.EndScriptFailure, sessionpkg.EndGameStartFailure:
		return sessionpkg.PreviewEndScriptFailure
	default:
		return sessionpkg.PreviewEndUnavailable
	}
}

// previewsKeepNothing is what a preview has instead of a Session's durable
// record: no checkpoints, heartbeats, replay, ownership or fault reports.
type previewsKeepNothing struct{}

func (previewsKeepNothing) SaveCheckpoint(context.Context, string, sessionpkg.Checkpoint) (sessionpkg.SaveCheckpointOutcome, error) {
	return sessionpkg.CheckpointSaved, nil
}

func (previewsKeepNothing) RecordOwnerHeartbeat(context.Context, string) (sessionpkg.OwnerHeartbeatResult, error) {
	return sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatRecorded}, nil
}

func (previewsKeepNothing) StartRecording(string) string                         { return "" }
func (previewsKeepNothing) Offer(context.Context, string, replay.Capture)        {}
func (previewsKeepNothing) Finish(context.Context, string, replay.Capture) error { return nil }
func (previewsKeepNothing) StopRecording(string)                                 {}

func (previewsKeepNothing) ReleaseOwner(context.Context, string, int64) error { return nil }

func (previewsKeepNothing) RecordGameFault(context.Context, string, sessionpkg.GameFault) (sessionpkg.RecordGameFaultOutcome, error) {
	return sessionpkg.GameFaultNotRecorded, nil
}

// errNotASession is what the Session-only operations answer for a preview;
// previews never reach them.
var errNotASession = errors.New("previews are not Sessions")

// previewsAreNotSessions stands for the Session operations a preview never
// uses: joining a lobby, starting it, recovering it.
type previewsAreNotSessions struct{}

func (previewsAreNotSessions) EligibleToJoinAsAdmin(context.Context, string, string) (sessionpkg.AdminJoinEligibility, error) {
	return "", errNotASession
}

func (previewsAreNotSessions) EligibleToJoinAsPlayer(context.Context, string) (sessionpkg.PlayerJoinEligibility, error) {
	return "", errNotASession
}

func (previewsAreNotSessions) LoadRuntimeManifest(context.Context, string) (sessionpkg.RuntimeManifest, error) {
	return sessionpkg.RuntimeManifest{}, errNotASession
}

func (previewsAreNotSessions) CheckStartEligibility(context.Context, string, string, int) (sessionpkg.StartEligibility, error) {
	return sessionpkg.StartEligibility{}, errNotASession
}

func (previewsAreNotSessions) CommitStart(context.Context, string, string, sessionpkg.StartCommit) (sessionpkg.CommitStartOutcome, error) {
	return "", errNotASession
}

func (previewsAreNotSessions) LoadRunningSession(context.Context, string) (sessionpkg.RunningSession, error) {
	return sessionpkg.RunningSession{}, errNotASession
}

func (previewsAreNotSessions) FrontendAccess(context.Context, string) (sessionpkg.FrontendAccess, error) {
	return sessionpkg.FrontendAccess{}, errNotASession
}
