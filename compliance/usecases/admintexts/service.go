// Package admintexts is how the texts people accept are published: the
// administrator writes a new version and it is served from then on. Versions
// are never edited, so an acceptance always points at what was accepted.
package admintexts

import (
	"context"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance"
	internalrepo "github.com/diegobermudez03/playhoot/compliance/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// MaxContentBytes is the longest a text may be.
const MaxContentBytes = 256 << 10

var (
	kindLabel    = metrics.NewLabel("kind", kindNames()...)
	outcomeLabel = metrics.NewLabel("outcome", string(compliance.TextPublished), string(compliance.TextInvalid))

	publicationsTotal = metrics.NewCounter(
		"compliance_text_publications_total",
		"Acceptance text versions the administrator published, by kind and what was decided.",
		kindLabel, outcomeLabel,
	)
)

func kindNames() []string {
	names := make([]string, len(compliance.TextKinds))
	for i, kind := range compliance.TextKinds {
		names[i] = string(kind)
	}
	return names
}

// Service is the acceptance texts' administration.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// AdminListTexts returns every kind in every language with the versions
// published for it, newest first. A pair with no versions is listed too:
// until it has one, nothing is served for it.
func (s *Service) AdminListTexts(ctx context.Context) ([]compliance.TextCatalogEntry, error) {
	defer logging.Step(ctx, "compliance.admin_list_texts").Close()
	rows, err := s.repo.AllTextVersions(ctx)
	if err != nil {
		return nil, err
	}
	published := make(map[string][]compliance.TextVersion, len(rows))
	for _, row := range rows {
		key := row.Kind + "/" + row.Language
		published[key] = append(published[key], compliance.TextVersion{Version: row.Version, PublishedAt: row.PublishedAt})
	}
	catalog := make([]compliance.TextCatalogEntry, 0, len(compliance.TextKinds)*len(compliance.Languages))
	for _, kind := range compliance.TextKinds {
		for _, language := range compliance.Languages {
			versions := published[string(kind)+"/"+language]
			if versions == nil {
				versions = []compliance.TextVersion{}
			}
			catalog = append(catalog, compliance.TextCatalogEntry{Kind: kind, Language: language, Versions: versions})
		}
	}
	return catalog, nil
}

// AdminPublishText publishes content as the newest version of kind in
// language. Versions count per kind and language: publishing one language
// neither hides nor changes the others.
func (s *Service) AdminPublishText(ctx context.Context, kind compliance.TextKind, language string, content string) (compliance.PublishedText, error) {
	defer logging.Step(ctx, "compliance.admin_publish_text").Close()
	content = strings.TrimSpace(content)
	if !slices.Contains(compliance.TextKinds, kind) || !slices.Contains(compliance.Languages, language) ||
		content == "" || len(content) > MaxContentBytes {
		publicationsTotal.Inc(ctx, kindLabel.Value(knownKind(kind)), outcomeLabel.Value(string(compliance.TextInvalid)))
		return compliance.PublishedText{Outcome: compliance.TextInvalid}, nil
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	published, err := utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (compliance.PublishedText, error) {
		version, err := s.repo.NextTextVersion(ctx, tx, string(kind), language)
		if err != nil {
			return compliance.PublishedText{}, err
		}
		row := internalrepo.AcceptanceText{
			Kind: string(kind), Version: version, Language: language, Content: content,
			PublishedAt: now, CreatedAt: now,
		}
		if err := s.repo.InsertText(ctx, tx, row); err != nil {
			return compliance.PublishedText{}, err
		}
		return compliance.PublishedText{Outcome: compliance.TextPublished, Version: version, PublishedAt: now}, nil
	})
	if err != nil {
		return compliance.PublishedText{}, err
	}
	publicationsTotal.Inc(ctx, kindLabel.Value(string(kind)), outcomeLabel.Value(string(compliance.TextPublished)))
	logging.LogFields(ctx, logging.Field("published_text_version", published.Version))
	return published, nil
}

// knownKind is kind when it is one, so a refusal of an unknown kind is still
// measurable.
func knownKind(kind compliance.TextKind) string {
	if slices.Contains(compliance.TextKinds, kind) {
		return string(kind)
	}
	return string(compliance.TextTerms)
}
