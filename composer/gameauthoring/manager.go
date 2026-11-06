// Package gameauthoring groups the cross-domain reads of a game being
// authored: Game Management decides who may read it, Play holds the version
// being built.
package gameauthoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameauthoring -destination=manager_mock_test.go . editableVersionFinder,draftReader,resourceLinkReader,gameSimulator,cardReader

// editableVersionFinder is Game Management's decision of whether a user may
// edit, and so read, a game's version being built.
type editableVersionFinder interface {
	FindEditableVersion(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersion, error)
}

// draftReader is Play's reads of a version being built.
type draftReader interface {
	ReadDraft(ctx context.Context, versionUUID string, revision int64, withContent bool) (sessionpkg.DraftSnapshot, error)
	ListRevisions(ctx context.Context, versionUUID string, before int64, limit int) ([]sessionpkg.DraftRevision, error)
	RevisionChanges(ctx context.Context, versionUUID string, since int64, upTo int64) (sessionpkg.DraftRevisionChanges, error)
	DiffRevisions(ctx context.Context, versionUUID string, from int64, to int64, onlyPath string) (sessionpkg.DraftRevisionDiff, error)
	ReadBuild(ctx context.Context, versionUUID string, revision int64, wait time.Duration) (sessionpkg.DraftBuild, error)
	LoadBuiltRevision(ctx context.Context, versionUUID string, revision int64) (sessionpkg.BuiltRevision, error)
}

// gameSimulator is the Runtime's simulation of a scenario on a game's
// backend script.
type gameSimulator interface {
	Simulate(ctx context.Context, script []byte, profile string, manifest sessionpkg.RuntimeManifest, scenario runtime.Scenario) (runtime.SimulationResult, error)
}

// resourceLinkReader is Identity's view of the links to one resource.
type resourceLinkReader interface {
	ResourceLinkStatus(ctx context.Context, userUUID string, kind string, resourceID string) (identity.ResourceLinkStatus, error)
}

// cardReader is Game Management's card of a game, whose name a draft read
// shows.
type cardReader interface {
	ReadCard(ctx context.Context, ownerUUID string, gameUUID string) (game.CardRead, error)
}

// Manager composes the game-authoring reads. It owns no business entity.
type Manager struct {
	editions  editableVersionFinder
	drafts    draftReader
	links     resourceLinkReader
	simulator gameSimulator
	cards     cardReader
}

// New constructs a Manager from Game Management's edit authorization, Play's
// version being built, Identity's links an AI uses for a game and the
// Runtime's simulation.
func New(editions editableVersionFinder, drafts draftReader, links resourceLinkReader, simulator gameSimulator, cards cardReader) *Manager {
	return &Manager{editions: editions, drafts: drafts, links: links, simulator: simulator, cards: cards}
}

// AILinkStatusRead is whether a game has a live link for an AI, set only
// when Access says the user owns the game (found or not_editable).
type AILinkStatusRead struct {
	Access game.EditableVersionOutcome
	Status identity.ResourceLinkStatus
}

// AILinkStatus says whether userUUID's game gameUUID has a live AI link.
func (m *Manager) AILinkStatus(ctx context.Context, userUUID string, gameUUID string) (AILinkStatusRead, error) {
	defer logging.Step(ctx, "composer.ai_link_status").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome == game.EditableVersionNotFound {
		return AILinkStatusRead{Access: editable.Outcome}, err
	}
	status, err := m.links.ResourceLinkStatus(ctx, userUUID, game.ResourceKind, gameUUID)
	if err != nil {
		return AILinkStatusRead{}, fmt.Errorf("reading the AI link: %s", err)
	}
	return AILinkStatusRead{Access: editable.Outcome, Status: status}, nil
}

// DraftRead is a read of a game's version being built: whether Game
// Management let the user read it (Access) and, only when it did, the
// revision read, or Missing when Play does not have the version yet (the
// orchestrator's EnsureDraft creates it).
type DraftRead struct {
	Access   game.EditableVersionOutcome
	Missing  bool
	Snapshot sessionpkg.DraftSnapshot
	// GameName is the name on the game's card.
	GameName string
}

// DraftRevisions is the list of revisions of a game's version being built,
// set only when Access is found and the version is not Missing in Play.
type DraftRevisions struct {
	Access    game.EditableVersionOutcome
	Missing   bool
	Revisions []sessionpkg.DraftRevision
}

// ReadDraft reads revision (0: the current one) of the version of gameUUID
// being built, with file contents when withContent. A revision past the
// current one returns sessionpkg.ErrDraftRevisionNotFound.
func (m *Manager) ReadDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, withContent bool) (DraftRead, error) {
	defer logging.Step(ctx, "composer.read_draft").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftRead{Access: editable.Outcome}, err
	}
	snapshot, err := m.drafts.ReadDraft(ctx, editable.VersionUUID, revision, withContent)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftRead{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			return DraftRead{}, err
		}
		return DraftRead{}, fmt.Errorf("reading the version being built: %s", err)
	}
	read, err := m.cards.ReadCard(ctx, userUUID, gameUUID)
	if err != nil {
		return DraftRead{}, fmt.Errorf("reading the game's card: %s", err)
	}
	return DraftRead{Access: editable.Outcome, Snapshot: snapshot, GameName: read.Card.Name}, nil
}

// ListDraftRevisions lists the revisions of the version of gameUUID being
// built numbered below before (from the current one when 0), newest first,
// at most limit.
func (m *Manager) ListDraftRevisions(ctx context.Context, userUUID string, gameUUID string, before int64, limit int) (DraftRevisions, error) {
	defer logging.Step(ctx, "composer.list_draft_revisions").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftRevisions{Access: editable.Outcome}, err
	}
	revisions, err := m.drafts.ListRevisions(ctx, editable.VersionUUID, before, limit)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftRevisions{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		return DraftRevisions{}, fmt.Errorf("listing the revisions of the version being built: %s", err)
	}
	return DraftRevisions{Access: editable.Outcome, Revisions: revisions}, nil
}

// DraftChanges is what changed in a game's version being built between two
// revisions, set only when Access is found and the version is not Missing.
type DraftChanges struct {
	Access  game.EditableVersionOutcome
	Missing bool
	Changes sessionpkg.DraftRevisionChanges
}

// ReadDraftChanges reads the files of revision upTo (0: the current one) of
// the version of gameUUID being built and what changed since revision since
// (0: since the beginning). A revision it lacks returns
// sessionpkg.ErrDraftRevisionNotFound.
func (m *Manager) ReadDraftChanges(ctx context.Context, userUUID string, gameUUID string, since int64, upTo int64) (DraftChanges, error) {
	defer logging.Step(ctx, "composer.read_draft_changes").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftChanges{Access: editable.Outcome}, err
	}
	changes, err := m.drafts.RevisionChanges(ctx, editable.VersionUUID, since, upTo)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftChanges{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			return DraftChanges{}, err
		}
		return DraftChanges{}, fmt.Errorf("reading the changes of the version being built: %s", err)
	}
	return DraftChanges{Access: editable.Outcome, Changes: changes}, nil
}

// DraftDiff compares two revisions of a game's version being built, set only
// when Access is found and the version is not Missing.
type DraftDiff struct {
	Access  game.EditableVersionOutcome
	Missing bool
	Diff    sessionpkg.DraftRevisionDiff
}

// ReadDraftDiff compares revision from (0: the empty project) with revision
// to (0: the current one) of the version of gameUUID being built, only
// onlyPath when set. A revision it lacks returns
// sessionpkg.ErrDraftRevisionNotFound.
func (m *Manager) ReadDraftDiff(ctx context.Context, userUUID string, gameUUID string, from int64, to int64, onlyPath string) (DraftDiff, error) {
	defer logging.Step(ctx, "composer.read_draft_diff").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftDiff{Access: editable.Outcome}, err
	}
	diff, err := m.drafts.DiffRevisions(ctx, editable.VersionUUID, from, to, onlyPath)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftDiff{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			return DraftDiff{}, err
		}
		return DraftDiff{}, fmt.Errorf("comparing revisions of the version being built: %s", err)
	}
	return DraftDiff{Access: editable.Outcome, Diff: diff}, nil
}

// DraftBuildRead is a read of one revision's build of a game's version being
// built, set only when Access is found and the version is not Missing.
type DraftBuildRead struct {
	Access  game.EditableVersionOutcome
	Missing bool
	Build   sessionpkg.DraftBuild
}

// ReadDraftBuild reads the build of revision (0: the current one) of the
// version of gameUUID being built, waiting up to wait for it to end. A
// revision without a build returns sessionpkg.ErrDraftBuildNotFound.
func (m *Manager) ReadDraftBuild(ctx context.Context, userUUID string, gameUUID string, revision int64, wait time.Duration) (DraftBuildRead, error) {
	defer logging.Step(ctx, "composer.read_draft_build").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftBuildRead{Access: editable.Outcome}, err
	}
	read, err := m.drafts.ReadBuild(ctx, editable.VersionUUID, revision, wait)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftBuildRead{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		if errors.Is(err, sessionpkg.ErrDraftBuildNotFound) {
			return DraftBuildRead{}, err
		}
		return DraftBuildRead{}, fmt.Errorf("reading the build of the version being built: %s", err)
	}
	return DraftBuildRead{Access: editable.Outcome, Build: read}, nil
}

// DraftSimulation is a simulation of a scenario on a revision of a game's
// version being built, set only when Access is found and the version is not
// Missing: the revision's build status, and the result only when Ready.
type DraftSimulation struct {
	Access      game.EditableVersionOutcome
	Missing     bool
	Revision    int64
	BuildStatus sessionpkg.DraftBuildStatus
	Ready       bool
	Result      runtime.SimulationResult
}

// SimulateDraft plays scenario on the build of revision (0: the current one)
// of the version of gameUUID being built. A revision without a build returns
// sessionpkg.ErrDraftBuildNotFound; one whose build did not succeed is not
// Ready.
func (m *Manager) SimulateDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, scenario runtime.Scenario) (DraftSimulation, error) {
	defer logging.Step(ctx, "composer.simulate_draft").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftSimulation{Access: editable.Outcome}, err
	}
	built, err := m.drafts.LoadBuiltRevision(ctx, editable.VersionUUID, revision)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftSimulation{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		if errors.Is(err, sessionpkg.ErrDraftBuildNotFound) {
			return DraftSimulation{}, err
		}
		return DraftSimulation{}, fmt.Errorf("loading the build to simulate: %s", err)
	}
	read := DraftSimulation{Access: editable.Outcome, Revision: built.Revision, BuildStatus: built.BuildStatus, Ready: built.Ready}
	if !built.Ready {
		return read, nil
	}
	read.Result, err = m.simulator.Simulate(ctx, built.Script, built.ScriptProfile, built.Manifest, scenario)
	if err != nil {
		if errors.Is(err, runtime.ErrSimulationsBusy) {
			return DraftSimulation{}, err
		}
		return DraftSimulation{}, fmt.Errorf("simulating: %s", err)
	}
	return read, nil
}

// editableVersion asks Game Management for the version userUUID may read. A
// found outcome without a version, or an unknown outcome, is broken data.
func (m *Manager) editableVersion(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersion, error) {
	editable, err := m.editions.FindEditableVersion(ctx, gameUUID, userUUID)
	if err != nil {
		return game.EditableVersion{}, fmt.Errorf("finding the editable version: %s", err)
	}
	logging.LogFields(ctx, logging.Field("edit_access", string(editable.Outcome)))
	switch editable.Outcome {
	case game.EditableVersionFound:
		if editable.VersionUUID == "" {
			monitoring.Alert(ctx, "game management found an editable version without its uuid")
			return game.EditableVersion{}, fmt.Errorf("game %s has an editable version without uuid", gameUUID)
		}
	case game.EditableVersionNotFound, game.EditableVersionNotEditable:
	default:
		monitoring.Alert(ctx, "game management answered an unknown editable version outcome")
		return game.EditableVersion{}, fmt.Errorf("unknown editable version outcome %q", editable.Outcome)
	}
	return editable, nil
}
