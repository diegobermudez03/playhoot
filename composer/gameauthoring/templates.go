package gameauthoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/discovery"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// templateFinder is Game Management's decision of whether a game may be
// started from: published and allowed as a template by its owner.
type templateFinder interface {
	FindTemplate(ctx context.Context, gameUUID string) (game.Template, error)
}

// templateCatalog is Discovery's catalog of templates.
type templateCatalog interface {
	Featured(ctx context.Context) ([]discovery.TemplateEntry, error)
	Categories(ctx context.Context) ([]discovery.TemplateCategory, error)
	Search(ctx context.Context, query discovery.TemplateQuery) (discovery.TemplatePage, error)
}

// TemplateBrowse is where finding a template starts: the featured ones and
// the categories.
type TemplateBrowse struct {
	Featured   []discovery.TemplateEntry
	Categories []discovery.TemplateCategory
}

// BrowseTemplates answers the featured templates and the categories.
func (m *Manager) BrowseTemplates(ctx context.Context) (TemplateBrowse, error) {
	defer logging.Step(ctx, "composer.browse_templates").Close()

	featured, err := m.catalog.Featured(ctx)
	if err != nil {
		return TemplateBrowse{}, fmt.Errorf("listing featured templates: %s", err)
	}
	categories, err := m.catalog.Categories(ctx)
	if err != nil {
		return TemplateBrowse{}, fmt.Errorf("listing template categories: %s", err)
	}
	return TemplateBrowse{Featured: featured, Categories: categories}, nil
}

// SearchTemplates answers one page of the templates query matches.
func (m *Manager) SearchTemplates(ctx context.Context, query discovery.TemplateQuery) (discovery.TemplatePage, error) {
	defer logging.Step(ctx, "composer.search_templates").Close()

	page, err := m.catalog.Search(ctx, query)
	if err != nil {
		return discovery.TemplatePage{}, fmt.Errorf("searching templates: %s", err)
	}
	return page, nil
}

// TemplateRead is a template as an AI inspects it before choosing it, set
// only when Found: its card, the revision its published version was frozen
// at, its files without contents, its brief.json, and the manifest its build
// declares (players, commands, inputs). Never its code.
type TemplateRead struct {
	Found    bool
	Template game.Template
	Revision int64
	Files    []sessionpkg.DraftFile
	Brief    *string
	Manifest []byte
}

// InspectTemplate reads templateGameUUID as a template: anyone may.
func (m *Manager) InspectTemplate(ctx context.Context, templateGameUUID string) (TemplateRead, error) {
	defer logging.Step(ctx, "composer.inspect_template").Close()

	template, snapshot, found, err := m.templateSnapshot(ctx, templateGameUUID)
	if err != nil || !found {
		return TemplateRead{}, err
	}
	read := TemplateRead{Found: true, Template: template, Revision: snapshot.Revision, Files: make([]sessionpkg.DraftFile, len(snapshot.Files))}
	for i, file := range snapshot.Files {
		if file.Path == sessionpkg.BriefPath {
			read.Brief = file.Content
		}
		file.Content = nil
		read.Files[i] = file
	}
	build, err := m.drafts.ReadBuild(ctx, template.VersionUUID, snapshot.Revision, 0)
	if err != nil && !errors.Is(err, sessionpkg.ErrDraftBuildNotFound) {
		return TemplateRead{}, fmt.Errorf("reading the template's build: %s", err)
	}
	read.Manifest = build.Manifest
	return read, nil
}

// TemplateFiles are files of a template's published revision, set only when
// Found; Missing names the paths it does not have.
type TemplateFiles struct {
	Found    bool
	Revision int64
	Files    []sessionpkg.DraftFile
	Missing  []string
}

// ReadTemplateFiles reads paths of templateGameUUID's published revision.
func (m *Manager) ReadTemplateFiles(ctx context.Context, templateGameUUID string, paths []string) (TemplateFiles, error) {
	defer logging.Step(ctx, "composer.read_template_files").Close()

	_, snapshot, found, err := m.templateSnapshot(ctx, templateGameUUID)
	if err != nil || !found {
		return TemplateFiles{}, err
	}
	byPath := make(map[string]sessionpkg.DraftFile, len(snapshot.Files))
	for _, file := range snapshot.Files {
		byPath[file.Path] = file
	}
	read := TemplateFiles{Found: true, Revision: snapshot.Revision}
	for _, path := range paths {
		file, has := byPath[path]
		if !has {
			read.Missing = append(read.Missing, path)
			continue
		}
		read.Files = append(read.Files, file)
	}
	return read, nil
}

// templateSnapshot is templateGameUUID as a template and its published
// revision with contents, found only when Game Management lets anyone start
// from it.
func (m *Manager) templateSnapshot(ctx context.Context, templateGameUUID string) (game.Template, sessionpkg.DraftSnapshot, bool, error) {
	template, err := m.templates.FindTemplate(ctx, templateGameUUID)
	if err != nil {
		return game.Template{}, sessionpkg.DraftSnapshot{}, false, fmt.Errorf("finding the template: %s", err)
	}
	logging.LogFields(ctx, logging.Field("template_outcome", string(template.Outcome)))
	if template.Outcome != game.TemplateFound {
		return game.Template{}, sessionpkg.DraftSnapshot{}, false, nil
	}
	snapshot, err := m.drafts.ReadDraft(ctx, template.VersionUUID, 0, true)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		monitoring.Alert(ctx, "game management names a template version play does not have")
		return game.Template{}, sessionpkg.DraftSnapshot{}, false, nil
	}
	if err != nil {
		return game.Template{}, sessionpkg.DraftSnapshot{}, false, fmt.Errorf("reading the template's version: %s", err)
	}
	return template, snapshot, true, nil
}
