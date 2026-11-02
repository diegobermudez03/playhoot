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
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameauthoring -destination=manager_mock_test.go . editableVersionFinder,draftReader

// editableVersionFinder is Game Management's decision of whether a user may
// edit, and so read, a game's version being built.
type editableVersionFinder interface {
	FindEditableVersion(ctx context.Context, gameUUID string, userUUID string) (game.EditableVersion, error)
}

// draftReader is Play's reads of a version being built.
type draftReader interface {
	ReadDraft(ctx context.Context, versionUUID string, revision int64, withContent bool) (sessionpkg.DraftSnapshot, error)
	ListRevisions(ctx context.Context, versionUUID string) ([]sessionpkg.DraftRevision, error)
	ReadBuild(ctx context.Context, versionUUID string, revision int64, wait time.Duration) (sessionpkg.DraftBuild, error)
}

// Manager composes the game-authoring reads. It owns no business entity.
type Manager struct {
	editions editableVersionFinder
	drafts   draftReader
}

// New constructs a Manager from Game Management's edit authorization and
// Play's version being built.
func New(editions editableVersionFinder, drafts draftReader) *Manager {
	return &Manager{editions: editions, drafts: drafts}
}

// DraftRead is a read of a game's version being built: whether Game
// Management let the user read it (Access) and, only when it did, the
// revision read, or Missing when Play does not have the version yet (the
// orchestrator's EnsureDraft creates it).
type DraftRead struct {
	Access   game.EditableVersionOutcome
	Missing  bool
	Snapshot sessionpkg.DraftSnapshot
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
	return DraftRead{Access: editable.Outcome, Snapshot: snapshot}, nil
}

// ListDraftRevisions lists the revisions of the version of gameUUID being
// built.
func (m *Manager) ListDraftRevisions(ctx context.Context, userUUID string, gameUUID string) (DraftRevisions, error) {
	defer logging.Step(ctx, "composer.list_draft_revisions").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil || editable.Outcome != game.EditableVersionFound {
		return DraftRevisions{Access: editable.Outcome}, err
	}
	revisions, err := m.drafts.ListRevisions(ctx, editable.VersionUUID)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		logging.LogFields(ctx, logging.Field("draft_missing", true))
		return DraftRevisions{Access: editable.Outcome, Missing: true}, nil
	}
	if err != nil {
		return DraftRevisions{}, fmt.Errorf("listing the revisions of the version being built: %s", err)
	}
	return DraftRevisions{Access: editable.Outcome, Revisions: revisions}, nil
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
