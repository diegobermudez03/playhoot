// Package templatecatalog lists and searches the templates a game can start
// from. Step 1 (the human, 2026-11-05) answers nothing yet: featuring,
// categories and the search itself come in step 2.
package templatecatalog

import (
	"context"

	"github.com/diegobermudez03/playhoot/discovery"
	"github.com/diegobermudez03/playhoot/logging"
)

// Service is the template catalog.
type Service struct{}

// New constructs the catalog.
func New() *Service {
	return &Service{}
}

// Featured lists the templates Playhoot features for AIs, featured first by
// category.
func (s *Service) Featured(ctx context.Context) ([]discovery.TemplateEntry, error) {
	defer logging.Step(ctx, "templatecatalog.featured").Close()
	return []discovery.TemplateEntry{}, nil
}

// Categories lists the categories templates are filed under.
func (s *Service) Categories(ctx context.Context) ([]discovery.TemplateCategory, error) {
	defer logging.Step(ctx, "templatecatalog.categories").Close()
	return []discovery.TemplateCategory{}, nil
}

// Search answers one page of the templates query matches.
func (s *Service) Search(ctx context.Context, query discovery.TemplateQuery) (discovery.TemplatePage, error) {
	defer logging.Step(ctx, "templatecatalog.search").Close()
	logging.LogFields(ctx, logging.Field("template_query", query.Text), logging.Field("template_category", query.Category), logging.Field("template_players", query.Players))
	return discovery.TemplatePage{Templates: []discovery.TemplateEntry{}}, nil
}
