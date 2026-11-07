package gameauthoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=gameauthoring -destination=template_mock_test.go . templateFinder

// templateFinder is Game Management's decision of whether a game may be
// started from: published and allowed as a template by its owner.
type templateFinder interface {
	FindTemplate(ctx context.Context, gameUUID string) (game.Template, error)
}

// ErrTemplateNotFound is returned for a game that is not a template anyone
// may start from.
var ErrTemplateNotFound = errors.New("template not found")

// template asks Game Management whether templateGameUUID may be started
// from, answering the published version to copy.
func (m *Manager) template(ctx context.Context, templateGameUUID string) (game.TemplateSource, error) {
	found, err := m.templates.FindTemplate(ctx, templateGameUUID)
	if err != nil {
		return game.TemplateSource{}, fmt.Errorf("finding the template: %s", err)
	}
	logging.LogFields(ctx, logging.Field("template_game_uuid", templateGameUUID), logging.Field("template_outcome", string(found.Outcome)))
	if found.Outcome != game.TemplateFound {
		return game.TemplateSource{}, ErrTemplateNotFound
	}
	return game.TemplateSource{GameUUID: found.GameUUID, VersionUUID: found.VersionUUID}, nil
}

// UseTemplate makes the version of gameUUID being built start over from the
// template templateGameUUID as its next revision, over baseRevision: the
// template's code and assets, keeping the draft's notes.md, brief.json and
// the uploads the template still declares. The template is never changed.
func (m *Manager) UseTemplate(ctx context.Context, userUUID string, gameUUID string, templateGameUUID string, baseRevision int64, idempotencyKey string, origin string) (DraftEdit, error) {
	defer logging.Step(ctx, "orchestrator.use_template").Close()

	source, err := m.template(ctx, templateGameUUID)
	if err != nil {
		return DraftEdit{}, err
	}
	var uncopiable bool
	edit, err := m.editDraft(ctx, userUUID, gameUUID, func(versionUUID string) (sessionpkg.DraftChangeResult, error) {
		result, err := m.drafts.ApplyTemplate(ctx, versionUUID, sessionpkg.DraftTemplate{GameUUID: source.GameUUID, VersionUUID: source.VersionUUID}, baseRevision, idempotencyKey, origin)
		uncopiable = errors.Is(err, sessionpkg.ErrDraftTemplateNotFound)
		return result, err
	})
	if uncopiable {
		// Game Management and Play disagree on the template's version.
		monitoring.Alert(ctx, "play cannot copy the published version game management names for a template")
		return DraftEdit{}, fmt.Errorf("%w: %s", ErrTemplateNotFound, err)
	}
	return edit, err
}

// draftStart is what the version Game Management names for editable starts
// from in Play: the template its first edition started from, or the
// published version an edition continues.
func draftStart(editable game.EditableVersion) sessionpkg.DraftStart {
	if editable.Template != nil {
		return sessionpkg.DraftStart{FromVersionUUID: editable.Template.VersionUUID, TemplateGameUUID: editable.Template.GameUUID}
	}
	return sessionpkg.DraftStart{FromVersionUUID: editable.BaseVersionUUID}
}
